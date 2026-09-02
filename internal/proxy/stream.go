package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"switchblade/internal/billing"
)

// usageScraper pulls token counts out of a response without buffering it.
//
// It understands both dialects the pool speaks. OpenAI reports the whole usage
// block once, in the final chunk. Anthropic splits it: input_tokens arrives in
// message_start and output_tokens accumulates across message_delta frames. Each
// field is taken from the last frame that carried a non-zero value, which lands
// on the right answer for either shape.
type usageScraper struct {
	u billing.Usage
}

type usageProbe struct {
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
	} `json:"usage"`
	Message *struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (s *usageScraper) apply(p usageProbe) {
	if u := p.Usage; u != nil {
		if u.PromptTokens > 0 {
			s.u.PromptTokens = u.PromptTokens
		}
		if u.CompletionTokens > 0 {
			s.u.CompletionTokens = u.CompletionTokens
		}
		if u.InputTokens > 0 {
			s.u.PromptTokens = u.InputTokens
		}
		if u.OutputTokens > 0 {
			s.u.CompletionTokens = u.OutputTokens
		}
		if u.TotalTokens > 0 {
			s.u.TotalTokens = u.TotalTokens
		}
	}
	if p.Message != nil && p.Message.Usage != nil {
		if p.Message.Usage.InputTokens > 0 {
			s.u.PromptTokens = p.Message.Usage.InputTokens
		}
		if p.Message.Usage.OutputTokens > 0 {
			s.u.CompletionTokens = p.Message.Usage.OutputTokens
		}
	}
}

var (
	sseData  = []byte("data:")
	sseDone  = []byte("[DONE]")
	jsonNull = []byte("null")
)

// consumeLine feeds one raw SSE line to the scraper.
func (s *usageScraper) consumeLine(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if !bytes.HasPrefix(trimmed, sseData) {
		return
	}
	payload := bytes.TrimSpace(trimmed[len(sseData):])
	if len(payload) == 0 || bytes.Equal(payload, sseDone) {
		return
	}
	var p usageProbe
	if json.Unmarshal(payload, &p) != nil {
		return
	}
	s.apply(p)
}

// isUsageOnlyChunk reports whether an SSE line carries OpenAI's terminal
// usage-only chunk — a populated usage block over an empty choices array.
// Chunks that attach usage to a content delta are not matched; those carry
// payload the client must see.
func isUsageOnlyChunk(trimmed []byte) bool {
	if !bytes.HasPrefix(trimmed, sseData) {
		return false
	}
	payload := bytes.TrimSpace(trimmed[len(sseData):])
	if len(payload) == 0 || bytes.Equal(payload, sseDone) {
		return false
	}
	var p struct {
		Usage   json.RawMessage   `json:"usage"`
		Choices []json.RawMessage `json:"choices"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return false
	}
	return len(p.Choices) == 0 && len(p.Usage) > 0 && !bytes.Equal(p.Usage, jsonNull)
}

// result returns the scraped usage with total filled in when upstream omitted it.
func (s *usageScraper) result() billing.Usage {
	u := s.u
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// extractUsage reads token counts from a complete, non-streamed response body.
func extractUsage(body []byte) billing.Usage {
	var s usageScraper
	var p usageProbe
	if json.Unmarshal(body, &p) == nil {
		s.apply(p)
	}
	return s.result()
}

// streamOpts adjusts pipeStream for one request.
type streamOpts struct {
	// dropUsageFrame suppresses the terminal usage-only chunk. Set when the
	// proxy injected stream_options.include_usage itself: the client never
	// opted in, and a chunk whose choices array is empty breaks clients that
	// index choices[0] on every frame.
	dropUsageFrame bool
	// idle bounds the silence between frames; kill runs when it elapses so a
	// stalled provider cannot pin the goroutine forever. Zero disables.
	idle time.Duration
	kill func()
}

// pipeStream copies an upstream stream to the client as it arrives, flushing
// after every line so tokens surface in real time instead of landing in one
// batch at the end, and scrapes usage on the way past.
//
// Reading line-wise via bufio.Reader rather than a Scanner avoids the token
// size limit — a single SSE frame carrying a large tool-call argument would
// otherwise abort the stream.
func pipeStream(w http.ResponseWriter, src io.Reader, opts streamOpts) (billing.Usage, int64, error) {
	flusher, _ := w.(http.Flusher)
	br := bufio.NewReaderSize(src, 32<<10)

	var watchdog *time.Timer
	if opts.idle > 0 && opts.kill != nil {
		watchdog = time.AfterFunc(opts.idle, opts.kill)
		defer watchdog.Stop()
	}

	var scraper usageScraper
	var written int64
	skipBlank := false // a dropped data line takes its frame terminator with it

	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			if watchdog != nil {
				watchdog.Reset(opts.idle)
			}
			scraper.consumeLine(line)

			drop := false
			if opts.dropUsageFrame {
				if trimmed := bytes.TrimSpace(line); len(trimmed) == 0 {
					drop, skipBlank = skipBlank, false
				} else if isUsageOnlyChunk(trimmed) {
					drop, skipBlank = true, true
				} else {
					skipBlank = false
				}
			}

			if !drop {
				n, writeErr := w.Write(line)
				written += int64(n)
				if flusher != nil {
					flusher.Flush()
				}
				if writeErr != nil {
					// Client hung up. Stop, but still report what was metered.
					return scraper.result(), written, writeErr
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				readErr = nil
			}
			return scraper.result(), written, readErr
		}
	}
}

// estimateUsage approximates a stream that carried bytes but no usage frame at
// four bytes per token. It over-counts SSE framing slightly, but the flag
// keeps the guess distinguishable from real metering — the alternative is
// billing such streams at zero, which makes them free.
func estimateUsage(promptBytes int, streamedBytes int64) billing.Usage {
	u := billing.Usage{
		PromptTokens:     promptBytes / 4,
		CompletionTokens: int(streamedBytes / 4),
		Estimated:        true,
	}
	u.TotalTokens = u.PromptTokens + u.CompletionTokens
	return u
}

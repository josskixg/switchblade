package ws

import (
	"fmt"
	"net/http"
)

type client struct {
	ch chan string
}

// Hub fans out SSE events to connected browser clients.
type Hub struct {
	register   chan *client
	unregister chan *client
	broadcast  chan [2]string // [event, data]
	clients    map[*client]struct{}
}

// NewHub returns an uninitialised Hub. Call Run() in a goroutine before use.
func NewHub() *Hub {
	return &Hub{
		register:   make(chan *client, 8),
		unregister: make(chan *client, 8),
		broadcast:  make(chan [2]string, 64),
		clients:    make(map[*client]struct{}),
	}
}

// Run processes register/unregister/broadcast in a single goroutine — no mutex needed.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			delete(h.clients, c)
			close(c.ch)
		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", msg[0], msg[1]):
				default:
					// ponytail: slow client dropped rather than blocked
				}
			}
		}
	}
}

// ServeSSE is an HTTP handler that streams SSE to a single client.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	c := &client{ch: make(chan string, 16)}
	h.register <- c
	defer func() { h.unregister <- c }()

	for {
		select {
		case msg, open := <-c.ch:
			if !open {
				return
			}
			fmt.Fprint(w, msg)
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Broadcast sends event+data to all connected SSE clients.
func (h *Hub) Broadcast(event, data string) {
	select {
	case h.broadcast <- [2]string{event, data}:
	default:
		// ponytail: drop if broadcast channel full, no blocking callers
	}
}

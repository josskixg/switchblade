package proxy

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
)

// Decompress attempts gzip decompression; returns body as-is if not gzip.
func Decompress(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		// not gzip — return as-is
		return body, nil
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompress gzip: %w", err)
	}
	return out, nil
}

// DecompressResponse decompresses based on Content-Encoding header value.
// "br" is returned as-is (no brotli in stdlib).
func DecompressResponse(body []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer r.Close()
		return io.ReadAll(r)
	case "deflate":
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("deflate reader: %w", err)
		}
		defer r.Close()
		return io.ReadAll(r)
	case "br":
		// ponytail: brotli not in stdlib; return as-is, add golang.org/x/net if needed
		return body, nil
	default:
		return body, nil
	}
}

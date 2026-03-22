package httputil

import (
	"errors"
	"fmt"
	"io"
)

// ErrBodyTooLarge is returned when the response body exceeds the configured limit.
var ErrBodyTooLarge = errors.New("body too large")

// ReadBody reads the full body up to maxBytes. Returns ErrBodyTooLarge if the
// body exceeds the limit, preventing silent truncation and OOM from oversized responses.
func ReadBody(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrBodyTooLarge, maxBytes)
	}
	return data, nil
}

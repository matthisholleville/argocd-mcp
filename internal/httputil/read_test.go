package httputil

import (
	"errors"
	"strings"
	"testing"
)

func TestReadBody_UnderLimit(t *testing.T) {
	data, err := ReadBody(strings.NewReader("hello"), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", data)
	}
}

func TestReadBody_ExactLimit(t *testing.T) {
	input := strings.Repeat("a", 100)
	data, err := ReadBody(strings.NewReader(input), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(data))
	}
}

func TestReadBody_ExceedsLimit(t *testing.T) {
	input := strings.Repeat("a", 101)
	_, err := ReadBody(strings.NewReader(input), 100)
	if err == nil {
		t.Fatal("expected error for body exceeding limit")
	}
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("expected ErrBodyTooLarge, got: %v", err)
	}
}

func TestReadBody_EmptyBody(t *testing.T) {
	data, err := ReadBody(strings.NewReader(""), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected 0 bytes, got %d", len(data))
	}
}

type errReader struct{ err error }

func (e errReader) Read(_ []byte) (int, error) { return 0, e.err }

func TestReadBody_ReaderError(t *testing.T) {
	sentinel := errors.New("network reset")
	_, err := ReadBody(errReader{sentinel}, 100)
	if err == nil {
		t.Fatal("expected error from reader")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error in chain, got: %v", err)
	}
}

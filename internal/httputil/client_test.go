package httputil

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Default options must produce a client that uses the system trust store and
// rejects unknown self-signed certs.
func TestNewClient_Default_RejectsUnknownCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewClient(ClientOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected TLS verification failure against unknown CA, got nil")
	}
}

// TLSInsecure=true must bypass verification (the escape hatch for local dev
// or emergencies; prefer CABundlePath in production).
func TestNewClient_TLSInsecure_AcceptsAnyCert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewClient(ClientOptions{Timeout: 5 * time.Second, TLSInsecure: true})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("insecure client failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// CABundlePath must load the PEM, append it to the system pool, and trust a
// server whose leaf is signed by it. This is the primary production path for
// private PKI deployments.
func TestNewClient_CABundlePath_TrustsCustomCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The test server's self-signed cert can be extracted via its Certificate().
	leaf := srv.Certificate()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	bundlePath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(bundlePath, pemBytes, 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	client, err := NewClient(ClientOptions{
		Timeout:      5 * time.Second,
		CABundlePath: bundlePath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("client failed against custom CA: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// The supplied bundle must actually end up in RootCAs, not silently ignored.
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("transport is not *http.Transport")
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated from CABundlePath")
	}
}

// A missing bundle file must fail at construction time, not surface as a
// runtime 502 on the first request.
func TestNewClient_CABundlePath_MissingFile_Fails(t *testing.T) {
	_, err := NewClient(ClientOptions{CABundlePath: "/definitely/not/a/real/path.pem"})
	if err == nil {
		t.Fatal("expected error for missing CA bundle file")
	}
	if !strings.Contains(err.Error(), "read CA bundle") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// A PEM file that parses but contains no certificates is a config error and
// must fail at construction time.
func TestNewClient_CABundlePath_EmptyPEM_Fails(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(bundlePath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	_, err := NewClient(ClientOptions{CABundlePath: bundlePath})
	if err == nil {
		t.Fatal("expected error for empty PEM")
	}
	if !strings.Contains(err.Error(), "no valid certificates") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// Make sure we're not leaking a nil transport on the happy path. The fetcher
// and gateway both assume Transport is non-nil and safe to reuse.
func TestNewClient_TransportIsAlwaysSet(t *testing.T) {
	client, err := NewClient(ClientOptions{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.Transport == nil {
		t.Fatal("expected non-nil Transport")
	}
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Errorf("expected *http.Transport, got %T", client.Transport)
	}
}

// Guard against accidentally returning a client whose TLS config leaks
// between calls (e.g. someone mutating RootCAs on the shared pool).
func TestNewClient_TLSConfigIsIsolated(t *testing.T) {
	a, err := NewClient(ClientOptions{TLSInsecure: true})
	if err != nil {
		t.Fatalf("NewClient a: %v", err)
	}
	b, err := NewClient(ClientOptions{TLSInsecure: false})
	if err != nil {
		t.Fatalf("NewClient b: %v", err)
	}

	trA := a.Transport.(*http.Transport)
	trB := b.Transport.(*http.Transport)
	if trA.TLSClientConfig == trB.TLSClientConfig {
		t.Fatal("two clients share the same TLSClientConfig pointer")
	}
	if !trA.TLSClientConfig.InsecureSkipVerify {
		t.Error("client a should be insecure")
	}
	if trB.TLSClientConfig.InsecureSkipVerify {
		t.Error("client b should be secure")
	}
}


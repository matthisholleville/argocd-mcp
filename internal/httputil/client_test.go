package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_UsesProxyFromEnvironment(t *testing.T) {
	var proxied string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = r.URL.String()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)

	resp, err := NewClient(5*time.Second, false).Get("http://argocd.example.invalid/api/version")
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	resp.Body.Close()

	if proxied != "http://argocd.example.invalid/api/version" {
		t.Fatalf("request did not go through HTTP_PROXY, proxy saw %q", proxied)
	}
}

func TestNewClient_TLSInsecure(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if _, err := NewClient(5*time.Second, false).Get(srv.URL); err == nil {
		t.Fatal("self-signed certificate accepted without TLS insecure")
	}

	resp, err := NewClient(5*time.Second, true).Get(srv.URL)
	if err != nil {
		t.Fatalf("TLS insecure client rejected self-signed certificate: %v", err)
	}
	resp.Body.Close()
}

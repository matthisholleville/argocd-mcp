package httputil

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	resp, err := NewClient(5*time.Second, nil).Get("http://argocd.example.invalid/api/version")
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

	if _, err := NewClient(5*time.Second, nil).Get(srv.URL); err == nil {
		t.Fatal("self-signed certificate accepted without TLS insecure")
	}

	insecure, err := TLSConfig(true, "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := NewClient(5*time.Second, insecure).Get(srv.URL)
	if err != nil {
		t.Fatalf("TLS insecure client rejected self-signed certificate: %v", err)
	}
	resp.Body.Close()
}

func TestTLSConfig_CABundle(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	bundle := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(empty, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("trusts the bundle", func(t *testing.T) {
		cfg, err := TLSConfig(false, bundle)
		if err != nil {
			t.Fatalf("TLSConfig: %v", err)
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 5 * time.Second}
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("certificate signed by the bundle was rejected: %v", err)
		}
		resp.Body.Close()
	})

	for name, path := range map[string]string{"missing file": filepath.Join(dir, "nope.pem"), "no certificate": empty} {
		t.Run(name+" fails fast", func(t *testing.T) {
			if _, err := TLSConfig(false, path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

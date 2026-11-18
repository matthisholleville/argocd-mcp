package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// TLSConfig builds the TLS settings shared by every outbound client from
// ARGOCD_TLS_INSECURE and ARGOCD_CA_BUNDLE. The bundle is added to the system
// pool and validated here, so a bad path fails at startup, not on first call.
func TLSConfig(insecure bool, caBundlePath string) (*tls.Config, error) {
	cfg := &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // operator opt-in via ARGOCD_TLS_INSECURE
	if caBundlePath == "" {
		return cfg, nil
	}

	pem, err := os.ReadFile(caBundlePath)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle %s contains no PEM certificate", caBundlePath)
	}
	cfg.RootCAs = pool
	return cfg, nil
}

// NewClient returns a client built on a clone of http.DefaultTransport, so
// HTTP(S)_PROXY, dial timeouts and HTTP/2 keep working; a zero http.Transport
// silently drops them.
func NewClient(timeout time.Duration, tlsConfig *tls.Config) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig.Clone()
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

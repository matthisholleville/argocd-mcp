package httputil

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewClient returns a client built on a clone of http.DefaultTransport, so
// HTTP(S)_PROXY, dial timeouts and HTTP/2 keep working; a zero http.Transport
// silently drops them.
func NewClient(timeout time.Duration, tlsInsecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: tlsInsecure} //nolint:gosec // operator opt-in via ARGOCD_TLS_INSECURE
	return &http.Client{Timeout: timeout, Transport: transport}
}

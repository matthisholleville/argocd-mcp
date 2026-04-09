package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// ClientOptions configures the shared HTTP client used to reach ArgoCD
// (OpenAPI spec, API gateway, Dex token proxy).
//
// All three clients in the server must share these options so that a single
// configuration point covers every outbound TLS handshake. Introducing a new
// client path should route through NewClient rather than constructing
// http.Client directly.
type ClientOptions struct {
	// Timeout is the request-level timeout applied to the client. A zero value
	// leaves the client without a timeout; callers that want a bounded timeout
	// must set it explicitly.
	Timeout time.Duration

	// TLSInsecure disables TLS certificate verification when set to true.
	// Prefer CABundlePath for a real trust chain; only use this for local
	// development against self-signed certs.
	TLSInsecure bool

	// CABundlePath points at a PEM-encoded CA bundle on disk. When set, the
	// file is loaded at client construction time and appended to the system
	// trust pool. This is the preferred way to trust ArgoCD behind a private
	// PKI (Vault, internal CA, etc.) without disabling verification entirely.
	// Ignored when empty.
	CABundlePath string
}

// NewClient returns an *http.Client whose transport honors the supplied
// options. A non-empty CABundlePath is loaded eagerly so that misconfiguration
// fails fast at startup instead of surfacing as a runtime 502 on the first
// outbound request.
func NewClient(opts ClientOptions) (*http.Client, error) {
	tlsCfg, err := newTLSConfig(opts)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

func newTLSConfig(opts ClientOptions) (*tls.Config, error) {
	cfg := &tls.Config{
		InsecureSkipVerify: opts.TLSInsecure, //nolint:gosec // Configurable via ARGOCD_TLS_INSECURE
	}

	if opts.CABundlePath == "" {
		return cfg, nil
	}

	pem, err := os.ReadFile(opts.CABundlePath)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle %q: %w", opts.CABundlePath, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		// SystemCertPool is not available on some platforms (notably Windows
		// before Go 1.18 and scratch images without any system store). Fall
		// back to an empty pool so the user-supplied bundle is still honored.
		pool = x509.NewCertPool()
	}

	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle %q contains no valid certificates", opts.CABundlePath)
	}

	cfg.RootCAs = pool
	return cfg, nil
}

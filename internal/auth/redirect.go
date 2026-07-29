package auth

import (
	"fmt"
	"net"
	"net/url"
)

// ValidateClientRedirect decides whether an authorization code may be delivered
// to raw.
//
// In proxy-callback mode this server substitutes its own redirect URI before the
// provider sees the request, so the provider's registration check no longer
// constrains where the code ends up: this function is the only thing that does.
// Without it, /authorize signs whatever target a caller asks for and
// /oauth/callback hands that caller someone else's authorization code.
//
// Accepted: RFC 8252 loopback URIs (the ephemeral ports MCP clients bind), and
// exact matches against operator-supplied allowed values for clients that call
// back to a fixed public URL.
func ValidateClientRedirect(raw string, allowed []string) error {
	for _, a := range allowed {
		if raw == a {
			return nil
		}
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("unparseable redirect_uri")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect_uri scheme %q is not allowed", u.Scheme)
	}
	if !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("redirect_uri host %q is neither loopback nor allowlisted", u.Hostname())
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

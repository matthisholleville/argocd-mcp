package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/matthisholleville/argocd-mcp/internal/httputil"
)

const maxDiscoveryResponseSize = 1 << 20 // 1 MB

// ProviderEndpoints holds the OAuth endpoints of an OIDC provider.
type ProviderEndpoints struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// DiscoverProvider fetches {issuer}/.well-known/openid-configuration and
// returns the endpoints the OAuth proxy needs. Used in oidc mode, where the
// provider is an arbitrary IdP rather than ArgoCD's bundled Dex (whose
// endpoints are known statically).
func DiscoverProvider(ctx context.Context, client *http.Client, issuer string) (ProviderEndpoints, error) {
	var endpoints ProviderEndpoints

	discoveryURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return endpoints, fmt.Errorf("build discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return endpoints, fmt.Errorf("fetch %s: %w", discoveryURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return endpoints, fmt.Errorf("fetch %s: unexpected status %d", discoveryURL, resp.StatusCode)
	}

	body, err := httputil.ReadBody(resp.Body, maxDiscoveryResponseSize)
	if err != nil {
		return endpoints, fmt.Errorf("read discovery document: %w", err)
	}
	if err := json.Unmarshal(body, &endpoints); err != nil {
		return endpoints, fmt.Errorf("parse discovery document: %w", err)
	}

	if endpoints.AuthorizationEndpoint == "" || endpoints.TokenEndpoint == "" {
		return endpoints, fmt.Errorf("discovery document at %s advertises no authorization_endpoint or token_endpoint", discoveryURL)
	}

	// OpenID Connect Discovery 1.0 §4.3: the document's issuer must equal the
	// issuer that was asked for. Without this check, a redirect or a tampered
	// issuer silently swaps in another origin's authorize and token endpoints.
	if endpoints.Issuer != strings.TrimRight(issuer, "/") && endpoints.Issuer != issuer {
		return endpoints, fmt.Errorf("discovery document at %s declares issuer %q, expected %q",
			discoveryURL, endpoints.Issuer, issuer)
	}

	return endpoints, nil
}

// ArgoCDOIDCSettings is the subset of ArgoCD's /api/v1/settings we read.
type ArgoCDOIDCSettings struct {
	Issuer string
	// ClientID is cliClientID when ArgoCD declares one, since that client is
	// the one meant for non-browser callers, and clientID otherwise.
	ClientID string
	// Scopes is ArgoCD's requestedScopes. Providers differ on what exists at all
	// (neither Entra ID nor Google Workspace has a `groups` scope), so asking for
	// anything other than what ArgoCD asks for risks invalid_scope on every login.
	Scopes []string
}

type argocdSettings struct {
	OIDCConfig *struct {
		Issuer          string   `json:"issuer"`
		ClientID        string   `json:"clientID"`
		CLIClientID     string   `json:"cliClientID"`
		RequestedScopes []string `json:"requestedScopes"`
	} `json:"oidcConfig"`
	DexConfig *struct {
		Connectors []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"connectors"`
	} `json:"dexConfig"`
}

// DiscoverArgoCDOIDC reads ArgoCD's own OIDC configuration from its public
// settings endpoint, so oidc mode needs no issuer or client id of its own:
// pointing at an ArgoCD is enough to learn which provider it trusts.
//
// It fails when ArgoCD advertises a Dex configuration instead, because tokens
// from an external provider would not validate against a Dex-backed ArgoCD.
func DiscoverArgoCDOIDC(ctx context.Context, client *http.Client, argoCDBaseURL string) (ArgoCDOIDCSettings, error) {
	var out ArgoCDOIDCSettings

	settingsURL := strings.TrimRight(argoCDBaseURL, "/") + "/api/v1/settings"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, settingsURL, nil)
	if err != nil {
		return out, fmt.Errorf("build settings request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("fetch %s: %w", settingsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("fetch %s: unexpected status %d", settingsURL, resp.StatusCode)
	}

	body, err := httputil.ReadBody(resp.Body, maxDiscoveryResponseSize)
	if err != nil {
		return out, fmt.Errorf("read settings: %w", err)
	}

	var settings argocdSettings
	if err := json.Unmarshal(body, &settings); err != nil {
		return out, fmt.Errorf("parse settings: %w", err)
	}

	if settings.OIDCConfig == nil || settings.OIDCConfig.Issuer == "" {
		if settings.DexConfig != nil {
			return out, fmt.Errorf("%s advertises a Dex configuration, not oidc.config: use AUTH_MODE=oauth", settingsURL)
		}
		return out, fmt.Errorf("%s advertises no oidcConfig: set OIDC_ISSUER and OIDC_CLIENT_ID explicitly", settingsURL)
	}

	out.Issuer = settings.OIDCConfig.Issuer
	out.Scopes = settings.OIDCConfig.RequestedScopes
	out.ClientID = settings.OIDCConfig.CLIClientID
	if out.ClientID == "" {
		out.ClientID = settings.OIDCConfig.ClientID
	}
	if out.ClientID == "" {
		return out, fmt.Errorf("%s advertises an oidcConfig without a client id: set OIDC_CLIENT_ID explicitly", settingsURL)
	}

	return out, nil
}

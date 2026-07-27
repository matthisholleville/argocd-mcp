package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testCodec(t *testing.T) *StateCodec {
	t.Helper()
	codec, err := NewStateCodec()
	if err != nil {
		t.Fatalf("could not build codec: %v", err)
	}
	return codec
}

// --- StateCodec tests ---

func TestStateCodec_RoundTrip(t *testing.T) {
	codec := testCodec(t)

	state, err := codec.Encode("http://localhost:53421/callback", "client-state")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	redirect, clientState, err := codec.Decode(state)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if redirect != "http://localhost:53421/callback" {
		t.Errorf("unexpected redirect: %q", redirect)
	}
	if clientState != "client-state" {
		t.Errorf("unexpected client state: %q", clientState)
	}
}

// The same input must not produce the same state twice: the upstream state
// doubles as the client's CSRF token.
func TestStateCodec_EncodeIsNotDeterministic(t *testing.T) {
	codec := testCodec(t)

	first, err := codec.Encode("http://localhost:1/callback", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	second, err := codec.Encode("http://localhost:1/callback", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if first == second {
		t.Error("expected a fresh nonce per authorization request")
	}
}

// The whole point of signing: a caller must not be able to hand us a redirect
// target of their choosing and have the authorization code delivered there.
func TestStateCodec_RejectsTamperedState(t *testing.T) {
	codec := testCodec(t)

	state, err := codec.Encode("http://localhost:53421/callback", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	body, _, _ := strings.Cut(state, ".")

	forged, err := codec.Encode("https://evil.example.com/steal", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, forgedSig, _ := strings.Cut(forged, ".")

	cases := map[string]string{
		"signature from another payload": body + "." + forgedSig,
		"no signature":                   body,
		"empty":                          "",
		"garbage":                        "not.valid",
	}
	for name, tampered := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := codec.Decode(tampered); err == nil {
				t.Error("expected the state to be rejected")
			}
		})
	}
}

// A state signed by a different process must not verify here.
func TestStateCodec_KeysAreNotShared(t *testing.T) {
	state, err := testCodec(t).Encode("http://localhost:1/callback", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, _, err := testCodec(t).Decode(state); err == nil {
		t.Error("expected a state from another codec to be rejected")
	}
}

// --- HandleCallback tests ---

func TestHandleCallback_RelaysCodeToClient(t *testing.T) {
	codec := testCodec(t)
	state, err := codec.Encode("http://localhost:53421/callback", "client-state")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	handler := HandleCallback(codec, silentLogger())
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=authcode&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("could not parse redirect: %v", err)
	}
	if location.Host != "localhost:53421" || location.Path != "/callback" {
		t.Errorf("expected the client's loopback, got %q", location)
	}
	if location.Query().Get("code") != "authcode" {
		t.Errorf("expected the code forwarded, got %q", location.Query().Get("code"))
	}
	// The client checks its own state, not ours.
	if got := location.Query().Get("state"); got != "client-state" {
		t.Errorf("expected the client's state restored, got %q", got)
	}
}

// A provider-side failure must reach the client, which is what surfaces it to
// the user, rather than dead-ending on this server.
func TestHandleCallback_RelaysProviderError(t *testing.T) {
	codec := testCodec(t)
	state, err := codec.Encode("http://localhost:53421/callback", "s")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	handler := HandleCallback(codec, silentLogger())
	req := httptest.NewRequest(http.MethodGet,
		"/oauth/callback?error=access_denied&error_description=nope&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	q, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("could not parse redirect: %v", err)
	}
	if q.Query().Get("error") != "access_denied" {
		t.Errorf("expected the error forwarded, got %q", q.Query().Get("error"))
	}
	if q.Query().Get("error_description") != "nope" {
		t.Errorf("expected the description forwarded, got %q", q.Query().Get("error_description"))
	}
	if q.Query().Get("code") != "" {
		t.Error("expected no code on an error callback")
	}
}

// An unverifiable state leaves no trustworthy redirect target, so this must
// terminate here instead of honouring the query parameters.
func TestHandleCallback_RejectsUnverifiableState(t *testing.T) {
	handler := HandleCallback(testCodec(t), silentLogger())
	req := httptest.NewRequest(http.MethodGet,
		"/oauth/callback?code=authcode&state=forged.signature&redirect_uri=https://evil.example.com", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "" {
		t.Errorf("expected no redirect, got %q", rec.Header().Get("Location"))
	}
}

// --- HandleAuthorize proxy-callback tests ---

// Providers that require exact redirect registration only ever see this
// server's callback; the client's loopback rides along in the state.
func TestHandleAuthorize_SubstitutesCallbackURL(t *testing.T) {
	codec := testCodec(t)
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/oauth2/v1/authorize",
		ClientID:              "okta-client",
		Scopes:                "openid",
		CallbackURL:           "https://mcp.example.com/oauth/callback",
		StateCodec:            codec,
	})
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&redirect_uri=http%3A%2F%2Flocalhost%3A53421%2Fcallback&state=client-state", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("could not parse redirect: %v", err)
	}
	if got := location.Query().Get("redirect_uri"); got != "https://mcp.example.com/oauth/callback" {
		t.Errorf("expected the server callback, got %q", got)
	}

	redirect, clientState, err := codec.Decode(location.Query().Get("state"))
	if err != nil {
		t.Fatalf("state does not decode: %v", err)
	}
	if redirect != "http://localhost:53421/callback" {
		t.Errorf("expected the client redirect carried, got %q", redirect)
	}
	if clientState != "client-state" {
		t.Errorf("expected the client state carried, got %q", clientState)
	}
}

// Without a client redirect there is nowhere to send the code back to, and
// silently substituting one would strand the login.
func TestHandleAuthorize_RequiresClientRedirectWhenProxying(t *testing.T) {
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/oauth2/v1/authorize",
		ClientID:              "okta-client",
		Scopes:                "openid",
		CallbackURL:           "https://mcp.example.com/oauth/callback",
		StateCodec:            testCodec(t),
	})
	req := httptest.NewRequest(http.MethodGet, "/authorize?response_type=code&state=s", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// Dex mode keeps passing the client's own loopback through untouched.
func TestHandleAuthorize_KeepsClientRedirectWithoutCallbackURL(t *testing.T) {
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://argocd.example.com/api/dex/auth",
		ClientID:              "argo-cd-cli",
		Scopes:                DexScopes,
	})
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A53421%2Fcallback&state=client-state", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("could not parse redirect: %v", err)
	}
	if got := location.Query().Get("redirect_uri"); got != "http://localhost:53421/callback" {
		t.Errorf("expected the client redirect untouched, got %q", got)
	}
	if got := location.Query().Get("state"); got != "client-state" {
		t.Errorf("expected the client state untouched, got %q", got)
	}
}

// --- HandleToken redirect override ---

// The provider checks the code against the redirect it was issued for, so the
// exchange has to repeat this server's callback, not the client's.
func TestHandleToken_OverridesRedirectURI(t *testing.T) {
	var received string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		received = r.FormValue("redirect_uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"jwt"}`))
	}))
	defer idp.Close()

	handler := HandleToken(TokenOptions{
		TokenEndpoint: idp.URL,
		ClientID:      "okta-client",
		RedirectURI:   "https://mcp.example.com/oauth/callback",
	})
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("grant_type=authorization_code&code=abc&redirect_uri=http%3A%2F%2Flocalhost%3A53421%2Fcallback"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if received != "https://mcp.example.com/oauth/callback" {
		t.Errorf("expected the server callback on the exchange, got %q", received)
	}
}

// Dex mode must keep forwarding whatever redirect the client used.
func TestHandleToken_KeepsClientRedirectURIByDefault(t *testing.T) {
	var received string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		received = r.FormValue("redirect_uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"jwt"}`))
	}))
	defer idp.Close()

	handler := HandleToken(TokenOptions{TokenEndpoint: idp.URL, ClientID: "argo-cd-cli"})
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("grant_type=authorization_code&code=abc&redirect_uri=http%3A%2F%2Flocalhost%3A53421%2Fcallback"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if received != "http://localhost:53421/callback" {
		t.Errorf("expected the client redirect preserved, got %q", received)
	}
}

package auth

import (
	"log/slog"
	"net/http"
	"net/url"
)

// CallbackPath is the path this server exposes as the provider's redirect URI.
const CallbackPath = "/oauth/callback"

// HandleCallback serves GET /oauth/callback, the single redirect URI registered
// at the provider. It hands the authorization code back to whichever loopback
// the MCP client bound, restoring the state the client originally sent.
func HandleCallback(codec *StateCodec, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		clientRedirect, clientState, err := codec.Decode(q.Get("state"))
		if err != nil {
			// Without a verified state there is no redirect target to report an
			// error to, so this terminates here rather than trusting the query.
			logger.Warn("callback with unusable state", slog.String("error", err.Error()))
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}

		target, err := url.Parse(clientRedirect)
		if err != nil {
			logger.Warn("callback state carries an unparseable redirect_uri",
				slog.String("error", err.Error()))
			http.Error(w, "invalid redirect_uri in state", http.StatusBadRequest)
			return
		}

		// Forward the provider's verdict as-is, success or failure: the client
		// is the one that knows how to surface it.
		forwarded := url.Values{}
		for _, key := range []string{"code", "error", "error_description", "error_uri", "iss"} {
			if v := q.Get(key); v != "" {
				forwarded.Set(key, v)
			}
		}
		if clientState != "" {
			forwarded.Set("state", clientState)
		}
		target.RawQuery = forwarded.Encode()

		http.Redirect(w, r, target.String(), http.StatusFound)
	}
}

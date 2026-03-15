package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type contextKey int

const bearerTokenKey contextKey = iota

// WithBearerToken stores the raw bearer token in the context.
func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenKey, token)
}

// GetBearerToken retrieves the bearer token from the context.
func GetBearerToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(bearerTokenKey).(string)
	return token, ok && token != ""
}

// NewPassthroughMiddleware extracts the Bearer token from the Authorization
// header and stores it in the context. On 401, it includes the WWW-Authenticate
// header with the resource_metadata URL so MCP clients can discover the OAuth flow.
func NewPassthroughMiddleware(serverBaseURL string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := extractBearer(r)
			if err != nil {
				logger.Warn("auth: missing token",
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
				)
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(
					`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`,
					serverBaseURL,
				))
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":             "unauthorized",
					"error_description": err.Error(),
				})
				return
			}

			ctx := WithBearerToken(r.Context(), token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", authErr("missing Authorization header")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", authErr("Authorization header must be 'Bearer <token>'")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", authErr("empty Bearer token")
	}
	return token, nil
}

type authErr string

func (e authErr) Error() string { return string(e) }

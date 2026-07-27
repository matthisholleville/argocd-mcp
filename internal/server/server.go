package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/matthisholleville/argocd-mcp/internal/audit"
	"github.com/matthisholleville/argocd-mcp/internal/ratelimit"
	"github.com/matthisholleville/argocd-mcp/internal/auth"
	"github.com/matthisholleville/argocd-mcp/internal/config"
	"github.com/matthisholleville/argocd-mcp/internal/gateway"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
	"github.com/matthisholleville/argocd-mcp/internal/toolgen"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverName = "argocd-mcp"

// Run is the single entry point.
func Run(cfg *config.Config, version string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Server-wide context tied to SIGTERM/SIGINT for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Fetch and parse ArgoCD OpenAPI spec.
	if cfg.TLSInsecure {
		logger.Warn("TLS certificate verification is DISABLED (ARGOCD_TLS_INSECURE=true)")
	}

	const fetchSpecTimeout = 30 * time.Second
	fetchCtx, fetchCancel := context.WithTimeout(ctx, fetchSpecTimeout)
	defer fetchCancel()

	endpoints, err := openapi.FetchAndParse(fetchCtx, cfg.SpecURL, cfg.ArgoCDToken, cfg.TLSInsecure, logger)
	if err != nil {
		logger.Error("failed to load ArgoCD spec", slog.String("url", cfg.SpecURL), slog.String("error", err.Error()))
		return fmt.Errorf("load ArgoCD spec: %w", err)
	}
	if len(endpoints) == 0 {
		logger.Error("no endpoints found in ArgoCD spec", slog.String("url", cfg.SpecURL))
		return fmt.Errorf("no endpoints found in ArgoCD spec at %s", cfg.SpecURL)
	}

	if cfg.DisableWrite {
		before := len(endpoints)
		endpoints = openapi.FilterReadOnly(endpoints)
		logger.Info("write operations disabled",
			slog.Int("total", before),
			slog.Int("read_only", len(endpoints)),
			slog.Int("filtered_out", before-len(endpoints)),
		)
	}

	if len(cfg.AllowedResources) > 0 {
		before := len(endpoints)
		endpoints = openapi.FilterByTags(endpoints, cfg.AllowedResources)
		logger.Info("resource scope filtering applied",
			slog.Any("allowed_resources", cfg.AllowedResources),
			slog.Int("before", before),
			slog.Int("after", len(endpoints)),
			slog.Int("filtered_out", before-len(endpoints)),
		)
	}

	// 2. Build the search backend.
	searcher, err := buildSearcher(cfg, endpoints, logger)
	if err != nil {
		logger.Error("failed to build searcher", slog.String("error", err.Error()))
		return fmt.Errorf("build searcher: %w", err)
	}

	// 3. Build the gateway.
	gw := gateway.NewGateway(cfg.ArgoCDBaseURL, cfg.ArgoCDToken, cfg.TLSInsecure, logger)

	// 4. Create MCP server.
	mcpServer := server.NewMCPServer(
		serverName,
		version,
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(true),
		server.WithRecovery(),
		server.WithLogging(),
		server.WithHooks(buildHooks(logger)),
	)

	// Build the allowed endpoints set for execute-time enforcement.
	// nil means all endpoints are allowed (no ALLOWED_RESOURCES restriction).
	var allowed *openapi.AllowedEndpoints
	if len(cfg.AllowedResources) > 0 {
		allowed = openapi.NewAllowedEndpoints(endpoints)
	}

	limiter := ratelimit.New(ctx, cfg.RateLimit, cfg.RateLimitBurst)
	if cfg.RateLimit > 0 {
		logger.Info("rate limiting enabled",
			slog.Float64("rate_per_sec", cfg.RateLimit),
			slog.Int("burst", cfg.RateLimitBurst),
		)
	}

	var auditor *audit.Logger
	if cfg.AuditLog {
		auditor = audit.New(logger)
		logger.Info("audit logging enabled")
	}

	gateway.RegisterMCPPrompts(mcpServer)

	switch cfg.ToolMode {
	case "generated":
		exec := toolgen.NewGatewayAdapter(gw)
		generated := toolgen.GenerateAll(endpoints, exec, limiter, auditor, cfg.DisableWrite)
		for _, gt := range generated {
			mcpServer.AddTool(gt.Tool, gt.Handler)
		}
		logger.Info("generated tools mode",
			slog.Int("tools_registered", len(generated)),
			slog.Int("endpoints_total", len(endpoints)),
		)
	default: // "search"
		gateway.RegisterMCPTools(mcpServer, gateway.ToolOptions{
			EndpointCount:    len(endpoints),
			DisableWrite:     cfg.DisableWrite,
			AllowedResources: cfg.AllowedResources,
		}, searcher, gw, allowed, limiter, auditor)
	}

	// 5. Resolve the OAuth proxy before announcing readiness: oidc mode reaches
	// out to ArgoCD and to the provider, and a server that logged "ready" and
	// then died on discovery is a confusing way to report a misconfiguration.
	proxy, err := resolveOAuthProxy(ctx, cfg, logger)
	if err != nil {
		return err
	}

	logger.Info("argocd-mcp ready",
		slog.String("transport", cfg.Transport),
		slog.String("auth_mode", cfg.AuthMode),
		slog.String("argocd", cfg.ArgoCDBaseURL),
		slog.String("tool_mode", cfg.ToolMode),
		slog.Int("endpoints", len(endpoints)),
		slog.Bool("embeddings", cfg.EmbeddingsEnabled),
		slog.Bool("disable_write", cfg.DisableWrite),
		slog.Bool("tls_insecure", cfg.TLSInsecure),
	)

	// 6. Start.
	switch cfg.Transport {
	case "http":
		return runHTTP(ctx, mcpServer, cfg, proxy, logger)
	default:
		return runStdio(mcpServer, logger)
	}
}

func buildSearcher(cfg *config.Config, endpoints []openapi.Endpoint, logger *slog.Logger) (gateway.Searcher, error) {
	if !cfg.EmbeddingsEnabled {
		logger.Info("using keyword search")
		return openapi.NewKeywordSearcher(endpoints), nil
	}

	logger.Info("using vector search (Ollama)",
		slog.String("ollama_url", cfg.OllamaURL),
		slog.String("model", cfg.EmbeddingsModel),
	)

	vi, err := openapi.NewVectorIndex(openapi.VectorConfig{
		OllamaURL: cfg.OllamaURL,
		Model:     cfg.EmbeddingsModel,
		TopK:      20,
	})
	if err != nil {
		return nil, err
	}

	logger.Info("indexing endpoints into vector store...")
	if err := vi.Index(context.Background(), endpoints); err != nil {
		logger.Error("failed to index endpoints into vector store", slog.String("error", err.Error()))
		return nil, fmt.Errorf("vector index: %w", err)
	}
	logger.Info("vector index ready")

	return vi, nil
}

func runStdio(s *server.MCPServer, logger *slog.Logger) error {
	logger.Info("starting stdio transport")
	return server.ServeStdio(s)
}

func runHTTP(ctx context.Context, s *server.MCPServer, cfg *config.Config, proxy *oauthProxy, logger *slog.Logger) error {
	httpSrv := server.NewStreamableHTTPServer(s,
		server.WithStateLess(true),
		server.WithEndpointPath("/mcp"),
	)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if proxy != nil {
		mountOAuthProxy(mux, httpSrv, cfg, logger, proxy)
	} else {
		mux.Handle("/mcp", httpSrv)
		logger.Info("token mode — /mcp is unauthenticated, using static ARGOCD_TOKEN")
	}

	httpServer := &http.Server{Addr: cfg.Addr, Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting HTTP transport", slog.String("addr", cfg.Addr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections...")
		// ctx is already cancelled; use a fresh context so the shutdown timeout is not immediately expired.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := httpServer.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		logger.Error("http server failed", slog.String("error", err.Error()))
		return fmt.Errorf("http server: %w", err)
	}
}

// oauthProxy holds the resolved provider endpoints the /authorize and /token
// proxies use. nil means token mode: no OAuth surface at all.
type oauthProxy struct {
	authorize auth.AuthorizeOptions
	token     auth.TokenOptions
	// stateCodec is set when this server terminates the provider's callback
	// itself; it is what /oauth/callback needs to find the client again.
	stateCodec *auth.StateCodec
}

// resolveOAuthProxy works out which provider to proxy to, reaching out to
// ArgoCD and the provider in oidc mode. Called before the server reports ready
// so a misconfiguration surfaces as a startup failure.
func resolveOAuthProxy(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*oauthProxy, error) {
	switch cfg.AuthMode {
	case "oauth":
		dexBase := strings.TrimRight(cfg.ArgoCDBaseURL, "/") + "/api/dex"
		logger.Info("OAuth mode enabled",
			slog.String("dex_issuer", dexBase),
			slog.String("client_id", cfg.DexClientID),
		)
		return &oauthProxy{
			authorize: auth.AuthorizeOptions{
				AuthorizationEndpoint: dexBase + "/auth",
				ClientID:              cfg.DexClientID,
				Scopes:                auth.DexScopes,
			},
			token: auth.TokenOptions{
				TokenEndpoint: dexBase + "/token",
				ClientID:      cfg.DexClientID,
			},
		}, nil

	case "oidc":
		client := &http.Client{Timeout: 15 * time.Second}

		var callbackURL string
		var stateCodec *auth.StateCodec
		if cfg.OIDCProxyCallback {
			var err error
			if stateCodec, err = auth.NewStateCodec(); err != nil {
				return nil, fmt.Errorf("oidc setup: %w", err)
			}
			callbackURL = strings.TrimRight(cfg.ServerBaseURL, "/") + auth.CallbackPath
		}

		issuer, clientID := cfg.OIDCIssuer, cfg.OIDCClientID
		if issuer == "" {
			settings, err := auth.DiscoverArgoCDOIDC(ctx, client, cfg.ArgoCDBaseURL)
			if err != nil {
				return nil, fmt.Errorf("oidc setup: read OIDC settings from ArgoCD: %w", err)
			}
			issuer, clientID = settings.Issuer, settings.ClientID
			logger.Info("discovered OIDC provider from ArgoCD",
				slog.String("issuer", issuer),
				slog.String("client_id", clientID),
			)
		}

		endpoints, err := auth.DiscoverProvider(ctx, client, issuer)
		if err != nil {
			return nil, fmt.Errorf("oidc setup: discover provider %s: %w", issuer, err)
		}

		logger.Info("OIDC mode enabled",
			slog.String("issuer", issuer),
			slog.String("client_id", clientID),
			slog.Bool("confidential_client", cfg.OIDCClientSecret != ""),
			slog.String("registered_redirect_uri", callbackURL),
		)
		return &oauthProxy{
			authorize: auth.AuthorizeOptions{
				AuthorizationEndpoint:  endpoints.AuthorizationEndpoint,
				ClientID:               clientID,
				Scopes:                 cfg.OIDCScopes,
				RequestedIDTokenClaims: cfg.OIDCRequestedIDTokenClaims,
				CallbackURL:            callbackURL,
				StateCodec:             stateCodec,
			},
			token: auth.TokenOptions{
				TokenEndpoint: endpoints.TokenEndpoint,
				ClientID:      clientID,
				ClientSecret:  cfg.OIDCClientSecret,
				RedirectURI:   callbackURL,
			},
			stateCodec: stateCodec,
		}, nil

	default:
		return nil, nil
	}
}

func mountOAuthProxy(
	mux *http.ServeMux,
	httpSrv http.Handler,
	cfg *config.Config,
	logger *slog.Logger,
	proxy *oauthProxy,
) {
	authorize, token := proxy.authorize, proxy.token
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", auth.HandleAuthServerMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", auth.HandleProtectedResourceMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", auth.HandleProtectedResourceMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("POST /register", auth.HandleRegister(authorize.ClientID, logger))
	mux.HandleFunc("GET /authorize", auth.HandleAuthorize(authorize))
	mux.HandleFunc("POST /token", auth.HandleToken(token))
	if proxy.stateCodec != nil {
		mux.HandleFunc("GET "+auth.CallbackPath, auth.HandleCallback(proxy.stateCodec, logger))
	}

	authMiddleware := auth.NewPassthroughMiddleware(cfg.ServerBaseURL, logger)
	mux.Handle("/mcp", authMiddleware(httpSrv))
}

func buildHooks(logger *slog.Logger) *server.Hooks {
	hooks := &server.Hooks{}
	hooks.AddBeforeCallTool(func(ctx context.Context, id any, req *mcp.CallToolRequest) {
		logger.Info("tool called", slog.String("tool", req.Params.Name), slog.Any("request_id", id))
	})
	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		logger.Error("request error", slog.String("method", string(method)), slog.String("error", err.Error()))
	})
	return hooks
}

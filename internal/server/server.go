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

	// 5. Start.
	switch cfg.Transport {
	case "http":
		return runHTTP(ctx, mcpServer, cfg, logger)
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

func runHTTP(ctx context.Context, s *server.MCPServer, cfg *config.Config, logger *slog.Logger) error {
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

	if cfg.AuthMode == "oauth" {
		mountOAuth(mux, httpSrv, cfg, logger)
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

func mountOAuth(mux *http.ServeMux, httpSrv http.Handler, cfg *config.Config, logger *slog.Logger) {
	dexBase := strings.TrimRight(cfg.ArgoCDBaseURL, "/") + "/api/dex"

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", auth.HandleAuthServerMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", auth.HandleProtectedResourceMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", auth.HandleProtectedResourceMetadata(cfg.ServerBaseURL))
	mux.HandleFunc("POST /register", auth.HandleRegister(cfg.DexClientID, logger))
	mux.HandleFunc("GET /authorize", auth.HandleAuthorize(dexBase+"/auth", cfg.DexClientID))
	mux.HandleFunc("POST /token", auth.HandleToken(dexBase+"/token", cfg.DexClientID, cfg.TLSInsecure))

	authMiddleware := auth.NewPassthroughMiddleware(cfg.ServerBaseURL, logger)
	mux.Handle("/mcp", authMiddleware(httpSrv))

	logger.Info("OAuth mode enabled",
		slog.String("dex_issuer", dexBase),
		slog.String("client_id", cfg.DexClientID),
	)
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

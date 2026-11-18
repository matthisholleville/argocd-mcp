# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

argocd-mcp is an MCP (Model Context Protocol) server that exposes the ArgoCD API to AI agents. It fetches ArgoCD's OpenAPI spec at startup and turns it into MCP tools, either as two meta-tools (`search_operations` + `execute_operation`) or as one generated tool per endpoint.

**Project priorities** (in order):
1. **Safety**: never let an agent do more than the operator allowed (`DISABLE_WRITE`, `ALLOWED_RESOURCES`, user-scoped tokens, rate limits)
2. **Correctness**: tool calls must map exactly to the ArgoCD API, and tool annotations must tell the truth
3. **Simplicity**: minimum code, no speculative features or configurability

## Development Commands

Use `make` targets. They are the source of truth for builds and tests.

```bash
make build         # Build binary to ./bin/argocd-mcp
make test          # go test -race -count=1 ./... (unit tests, no Docker)
make e2e           # E2E against a real ArgoCD in k3s (requires Docker, ~1-2 min)
make chart-test    # Helm template tests (requires helm): renders the chart and asserts on the Deployment
make run           # Build + run with ARGOCD_BASE_URL / ARGOCD_TOKEN from env
make docker-build  # Build the container image
./hack/helm-diff.sh origin/main   # Diff of the rendered chart vs a base ref (default + embeddings profiles)
go vet ./... && go vet -tags e2e ./e2e/...   # What CI's lint job runs
```

## Architecture

```text
startup: fetch {ARGOCD_BASE_URL}/swagger.json → parse endpoints → filter (DISABLE_WRITE, ALLOWED_RESOURCES) → build searcher → register tools
request: MCP client → transport (stdio | Streamable HTTP /mcp) → [auth middleware] → tool handler → rate limit → Gateway → ArgoCD API
```

### Directory Structure

| Directory | Purpose |
|---|---|
| `cmd/argocd-mcp/` | Entrypoint: loads config, calls `server.Run` |
| `internal/server/` | Wiring: spec loading, tool mode switch, MCP server, HTTP mux (`/mcp`, `/healthz`, `/readyz`, `/metrics`), OAuth proxy mounting, graceful shutdown |
| `internal/config/` | All configuration, from environment variables only (`config.Load`) |
| `internal/openapi/` | Spec fetch/parse, read-only and tag filters, keyword search and Ollama vector search |
| `internal/gateway/` | `search` tool mode: the two meta-tools, MCP prompts, and `Gateway`, the HTTP client to ArgoCD |
| `internal/toolgen/` | `generated` tool mode: one MCP tool per endpoint (naming, params, annotations) |
| `internal/auth/` | OAuth modes: Dex (`oauth`) and external OIDC (`oidc`) proxy, metadata, callback state, bearer passthrough |
| `internal/audit/` | Structured audit log of every tool call (`AUDIT_LOG`) |
| `internal/metrics/` | Prometheus collectors for tool calls |
| `internal/ratelimit/` | Per-user token-bucket limiter for executions |
| `e2e/` | E2E tests (build tag `e2e`); `e2e/fixtures` starts k3s + ArgoCD |
| `charts/argocd-mcp/` | Helm chart (version bumped by release-please) |

### Tool modes (`TOOL_MODE`)

- `search` (default): `search_operations` (searches the local endpoint index, never calls ArgoCD) and `execute_operation` (calls ArgoCD). Code in `internal/gateway/tools.go`.
- `generated`: one tool per endpoint, named `argocd_<service>_<action>` (e.g. `ApplicationService_List` → `argocd_application_list`). Code in `internal/toolgen/`.

### Auth modes (`AUTH_MODE`)

- `token` (default): static `ARGOCD_TOKEN` for every call; `/mcp` is unauthenticated.
- `oauth`: proxies ArgoCD's bundled Dex; the user's id_token is forwarded to ArgoCD.
- `oidc`: proxies the external provider ArgoCD trusts (read from ArgoCD's `oidc.config` unless overridden).

In OAuth modes, `Gateway.Execute` forwards the caller's bearer token (`auth.GetBearerToken(ctx)`), so ArgoCD RBAC applies per user. The static token is only the fallback.

## Code Conventions

### Safety invariants

- **Guards are enforced twice**: `DISABLE_WRITE` and `ALLOWED_RESOURCES` filter the endpoint list at startup (`internal/server/server.go`), **and** `handleExecute` re-checks both on every `execute_operation` call, since the agent controls `method` and `path`. In `generated` mode, out-of-scope tools are never created and `GenerateAll` skips write methods again as defence-in-depth. Never drop one of the two layers.
- **Tool annotations must be explicit**: `mcp.NewTool` defaults to `destructiveHint=true, readOnlyHint=false`. Every tool must set its annotations (see `annotationsForMethod` in `internal/toolgen/generator.go` and `searchTool`/`executeTool` in `internal/gateway/tools.go`).
- **Outbound HTTP goes through `httputil.NewClient`**: it clones `http.DefaultTransport` (keeps `HTTP(S)_PROXY`, timeouts, HTTP/2) and applies `ARGOCD_TLS_INSECURE`. Never build an `http.Client` or a zero `http.Transport` by hand: that is how the OAuth token exchange ended up ignoring both.
- **Never log tokens**. Audit logs identify the user by the email claim of their token (`static-token` in token mode), never by the token itself.
- **Metrics labels stay bounded**: tool name and status only, no user or path labels.

### Style

- Errors: wrap with context, `fmt.Errorf("load ArgoCD spec: %w", err)`.
- Logging: `log/slog` JSON to stderr (stdout is the MCP stdio channel: never print to it).
- Constructors: `New<Type>()`; dependencies are injected via constructors, not globals.
- **No superfluous comments.** Comment only the non-obvious *why*. Do not narrate the code.
- Keep changes surgical: don't refactor or reformat code unrelated to the task.

## Testing

### Test-Driven Development (mandatory, red/green)

**Every change** (feature, bug fix, refactor, dependency bump that changes behaviour) goes through red/green:

1. **Red**: write the test first, run it, and **watch it fail for the right reason**: the assertion about the missing behaviour, not a compile error or a typo.
2. **Green**: write the minimum code that makes it pass. Run the test again.
3. **Refactor** only while everything stays green.

- A bug fix starts with a test that reproduces the bug.
- A refactor without a behaviour change must keep the existing tests green before and after.
- Never write the implementation first and the test afterwards, and never weaken an assertion to get to green.
- In the PR, state which tests were red before the change and green after.

Unit tests are table-driven where it fits, use `httptest` fakes for ArgoCD, and run with `-race` (`make test`).

### E2E coverage (as much as possible)

Any behaviour an MCP client can observe **must** be covered by an e2e test against the real ArgoCD, in addition to unit tests: tools, tool modes, annotations, guards (`DISABLE_WRITE`, `ALLOWED_RESOURCES`, rate limit), transports, protocol versions, and calls to the ArgoCD API. The e2e test also goes red first, then green (`make e2e`). If something genuinely cannot be covered end to end (e.g. a third-party OIDC provider), say so in the PR and explain why.

How the harness works:

- `fixtures.RunArgoCD(t)` starts **one shared** k3s container per package (testcontainers-go), installs a pinned ArgoCD version, scales it down to `argocd-server` + redis, seeds the `guestbook` Application (as a CR, never synced) and returns a URL + admin token. `fixtures.CleanupArgoCD()` runs from `TestMain`.
- `fixtures.RunOllama(t)` starts **one shared** Ollama container (image pinned to the ollama subchart's `appVersion`) and pulls `nomic-embed-text`, the production embeddings model. Use it for anything touching `EMBEDDINGS_ENABLED` / vector search. `fixtures.CleanupOllama()` runs from `TestMain`.
- `TestMain` builds the real `argocd-mcp` binary; tests start it with their own env (`startHTTPServer`, `connectStdio`) and drive it with the mcp-go client, pinning the protocol version (`client.WithProtocolVersion`).
- Cover both protocol generations: `2025-11-25` (initialize handshake) and `2026-07-28` (stateless).
- `e2e` is a **required status check on `main`**.

## Files You Must Not Edit

- **`CHANGELOG.md`, `.release-please-manifest.json`**, and the chart `version`/`appVersion` in `charts/argocd-mcp/Chart.yaml`: owned by release-please.
- **`go.sum`**: only via `go get` / `go mod tidy`.

## Common Gotchas

- **Go version is pinned in four places**: `go.mod` (`go` directive), `Dockerfile` (`golang:<ver>-alpine@sha256:…`), and `go-version` in `.github/workflows/ci.yml` and `release.yml`. Renovate updates them together in its "Go toolchain" group. If you change one by hand, change all of them.
- **The builder image sets `GOTOOLCHAIN=local`**: its Go must be ≥ the `go.mod` directive, or `docker build` fails.
- **testcontainers-go and Go versions**: a testcontainers release can raise the minimum Go version for the whole module. Check `go.mod` after bumping it.
- **Ollama version lives in two places**: the subchart version in `charts/argocd-mcp/Chart.yaml` (which pins the Ollama image) and `ollamaImage` in `e2e/fixtures/ollama.go`. When the chart is bumped, set `ollamaImage` to the new chart `appVersion` (the helm-diff PR comment shows it) so e2e tests what ships.
- **Small embedding models don't fit the ArgoCD spec**: some endpoint descriptions exceed 256 tokens, so models like `all-minilm` fail at indexing (Ollama returns 500). Keep `nomic-embed-text` in tests.
- **e2e code only compiles with `-tags e2e`**: plain `go build ./...` / `go vet ./...` skip it, and so do gopls diagnostics unless the tag is configured.
- **Stdio transport and contexts**: the mcp-go stdio transport ties the subprocess to the context passed to `Start`. Pass a long-lived context (`t.Context()` in tests), not a short timeout.
- **mcp-go protocol eras**: since v1.x the server serves both the legacy handshake and the stateless 2026-07-28 core on the same endpoint. Don't add logic that depends on `initialize` having happened (hooks, session state).

## Helm chart

- Chart changes follow red/green too: add a case to `hack/charttest/chart_test.go` (build tag `chart`, renders with `helm template` and decodes the Deployment) before touching `charts/argocd-mcp/templates/`.
- CI lints the chart with the default values **and** with `embeddings.enabled=true` (otherwise the ollama subchart is never rendered).
- On every PR, the `helm-diff` job renders the chart on the base branch and on the PR (`hack/helm-diff.sh`) and posts the manifest diff as a sticky PR comment. The comment is removed when the rendered chart is unchanged. Review it for any chart or subchart change.

## Commits and Releases

- PRs are **squash-merged only** (repo setting): the **PR title becomes the commit message**, with an empty body. The PR title must therefore be a Conventional Commit; intermediate commits on the branch don't reach `main`.
- **Conventional Commits** are required: release-please derives versions and the changelog from them. `feat:` → minor, `fix:` → patch, `chore:`/`test:`/`docs:` → no release. Use `fix(deps):` when a dependency or toolchain update changes the shipped binary or image.
- PRs target `main`; dependency updates come from Renovate (`renovate.json`): minor/patch/digest are automerged when CI is green, while the Go toolchain and Helm chart dependencies need review.

## Common Development Tasks

- **New configuration option**: failing test in `config_test.go` (+ e2e case for its effect) → field + doc comment in `internal/config/config.go` → parse/validate in `Load` → wire in `internal/server/server.go` → document in `README.md` → expose in the chart (`charts/argocd-mcp/values.yaml` + env in `templates/deployment.yaml`).
- **New tool or tool behaviour (search mode)**: failing unit test in `internal/gateway/tools_test.go` + failing e2e case in `e2e/argocd_test.go` → implement in `internal/gateway/tools.go` (definition + annotations + handler) → both green.
- **Generated-mode change**: failing tests in `internal/toolgen/generator_test.go` + e2e case → implement in `internal/toolgen/` → green.
- **Auth change**: failing regression test first (see `proxy_regression_test.go`) → change `internal/auth/` → green; redirect and state validation are security-sensitive.

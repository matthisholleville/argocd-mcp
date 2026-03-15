# argocd-mcp

MCP server that exposes the **entire** ArgoCD API to LLMs — all 103+ endpoints, not just a handful.

Most ArgoCD MCP servers hardcode a few operations: list apps, sync, get status. When ArgoCD adds a new feature, you wait for the maintainer to add it.

**argocd-mcp** takes a different approach. It reads ArgoCD's OpenAPI spec at startup and exposes every endpoint through just 2 tools: `search` and `execute`. New ArgoCD version? Restart the server. Done.

- **103+ endpoints**, 2 tools, ~200 tokens of system prompt
- Works with **Claude Desktop, Cursor, Claude Code**, or any MCP client
- **No code per endpoint** — the OpenAPI spec is the source of truth
- **Two auth modes**: static token (simple) or OAuth via ArgoCD Dex (per-user RBAC)
- **Optional semantic search** via Ollama embeddings

## Quick Start

### Option 1: Static Token (simple)

Best for local dev, CI/CD, or single-user setups.

**Claude Desktop / Cursor (stdio):**

```json
{
  "mcpServers": {
    "argocd": {
      "command": "./bin/argocd-mcp",
      "env": {
        "ARGOCD_BASE_URL": "https://argocd.example.com",
        "ARGOCD_TOKEN": "your-token"
      }
    }
  }
}
```

**HTTP mode:**

```bash
docker run -p 8080:8080 \
  -e ARGOCD_BASE_URL=https://argocd.example.com \
  -e ARGOCD_TOKEN=your-token \
  -e MCP_TRANSPORT=http \
  ghcr.io/matthisholleville/argocd-mcp:latest
```

### Option 2: OAuth via ArgoCD Dex (per-user RBAC)

Best for multi-user, production setups. Each user authenticates with their own identity via ArgoCD's built-in Dex. **No static token needed** — the user's Dex `id_token` is forwarded to ArgoCD, which applies its RBAC policies per user.

```bash
docker run -p 8080:8080 \
  -e ARGOCD_BASE_URL=https://argocd.example.com \
  -e MCP_TRANSPORT=http \
  -e AUTH_MODE=oauth \
  -e DEX_CLIENT_ID=argo-cd-cli \
  -e SERVER_BASE_URL=https://mcp.example.com \
  ghcr.io/matthisholleville/argocd-mcp:latest
```

Then in Claude Desktop, add the MCP server URL (e.g. `https://mcp.example.com/mcp`). Claude Desktop handles the OAuth flow automatically — users log in via ArgoCD's Dex (Okta, GitHub, SAML, etc.).

#### Dex Configuration (required)

<details>
<summary><b>Why and how to configure ArgoCD's Dex for OAuth</b></summary>

ArgoCD uses Dex as its OIDC provider. The MCP server acts as an OAuth proxy to Dex, using the `argo-cd-cli` public client. By default, this client only allows `http://localhost` as a redirect URI. Claude Desktop needs `https://claude.ai/api/mcp/auth_callback`.

Since ArgoCD auto-registers the `argo-cd-cli` client at startup, we override it in `dex.config` with the additional redirect URI. ArgoCD prepends its auto-generated clients, and Dex uses the last definition when there are duplicate IDs — so our override wins ([ref: ArgoCD source](https://github.com/argoproj/argo-cd/blob/master/util/dex/config.go)).

Add this to your ArgoCD Helm values (or ConfigMap `argocd-cm`) under `dex.config`:

```yaml
staticClients:
  - id: argo-cd-cli
    name: Argo CD CLI
    public: true
    redirectURIs:
      - http://localhost
      - http://localhost:8085/auth/callback
      - https://claude.ai/api/mcp/auth_callback
```

**You must keep the original redirect URIs** (`http://localhost` and `http://localhost:8085/auth/callback`), otherwise `argocd login --sso` will break ([ref: #19787](https://github.com/argoproj/argo-cd/issues/19787)).

The `argo-cd-cli` client is public (no secret), so this override is safe — unlike overriding `argo-cd` which has an internal secret and will cause `invalid_client` errors.

The resulting token has `aud: argo-cd-cli`, which is in ArgoCD's hardcoded list of allowed audiences when using Dex. ArgoCD applies its RBAC policies based on the user's identity in the token.

</details>

**How it works:**
- The MCP server acts as an OAuth proxy to ArgoCD's Dex
- Uses the `argo-cd-cli` public client (no secret needed)
- The Dex `id_token` (with `aud: argo-cd-cli`) is forwarded as-is to ArgoCD
- ArgoCD validates the token and applies per-user RBAC policies
- Each user only sees the applications and resources they have access to

## How It Works

```
ArgoCD /swagger.json
         |
         v
    Parse Swagger 2.0 → 103+ Endpoint[] in memory
         |
    +----+----+
    |         |
 search    execute
    |         |
    v         v
 LLM finds   LLM calls
 endpoints   ArgoCD API
```

1. At startup, the server fetches ArgoCD's Swagger spec
2. It parses every endpoint (method, path, summary, parameters, request body schema)
3. **`search_operations`** — keyword or semantic search across all endpoints
4. **`execute_operation`** — generic HTTP proxy to ArgoCD

## Semantic Search (optional)

Enable Ollama-powered vector search for better results on natural language queries:

```bash
docker compose up --build -d  # Starts Ollama + argocd-mcp with embeddings
```

Set `EMBEDDINGS_ENABLED=true`, `OLLAMA_URL`, and `EMBEDDINGS_MODEL` (defaults to `nomic-embed-text`).

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ARGOCD_BASE_URL` | Yes | | ArgoCD server URL |
| `ARGOCD_TOKEN` | When `AUTH_MODE=token` | | ArgoCD API token |
| `AUTH_MODE` | No | `token` | `token` (static) or `oauth` (Dex SSO) |
| `DEX_CLIENT_ID` | When `AUTH_MODE=oauth` | `argo-cd-cli` | Dex client ID |
| `SERVER_BASE_URL` | When `AUTH_MODE=oauth` | `http://localhost:8080` | Public URL of this server |
| `ARGOCD_SPEC_URL` | No | `{base}/swagger.json` | Override spec URL |
| `MCP_TRANSPORT` | No | `stdio` | `stdio` or `http` |
| `MCP_ADDR` | No | `:8080` | HTTP listen address |
| `EMBEDDINGS_ENABLED` | No | `false` | Enable Ollama vector search |
| `OLLAMA_URL` | No | `http://localhost:11434/api` | Ollama API URL |
| `EMBEDDINGS_MODEL` | No | `nomic-embed-text` | Ollama embedding model |

## Build from source

```bash
make build
ARGOCD_BASE_URL=https://argocd.example.com ARGOCD_TOKEN=xxx ./bin/argocd-mcp
```

## License

MIT

package toolgen

import (
	"context"
	"encoding/json"

	"github.com/matthisholleville/argocd-mcp/internal/gateway"
)

// GatewayAdapter wraps a gateway.Gateway to satisfy the Executor interface.
type GatewayAdapter struct {
	gw *gateway.Gateway
}

// NewGatewayAdapter creates an Executor from a gateway.Gateway.
func NewGatewayAdapter(gw *gateway.Gateway) *GatewayAdapter {
	return &GatewayAdapter{gw: gw}
}

func (a *GatewayAdapter) Execute(ctx context.Context, params ExecuteParams) (json.RawMessage, error) {
	return a.gw.Execute(ctx, gateway.ExecuteParams{
		Method:      params.Method,
		Path:        params.Path,
		QueryParams: params.QueryParams,
		Body:        params.Body,
	})
}

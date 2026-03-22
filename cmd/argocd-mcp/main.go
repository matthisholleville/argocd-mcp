package main

import (
	"fmt"
	"os"

	"github.com/matthisholleville/argocd-mcp/internal/config"
	"github.com/matthisholleville/argocd-mcp/internal/server"
)

// Set via -ldflags at build time.
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	if err := server.Run(cfg, version); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

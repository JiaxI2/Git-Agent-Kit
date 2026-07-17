// Command gia-mcp exposes Git Agent Kit over MCP stdio transport.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/adapters/legacy"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/adapters/mcp"
)

func main() {
	repository := os.Getenv("GIA_REPO")
	if repository == "" {
		repository = "."
	}
	server := mcp.Server{Repository: repository, Services: legacy.NewServices(repository, os.Getenv("GIA_CONFIG"))}
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// examples/mcp-mock/main.go — 仅用于 tests/integration 与本地调试
package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Message string `json:"message"`
}

type echoOutput struct {
	Message string `json:"message"`
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-mock", Version: "v0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echo",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, echoOutput, error) {
		return nil, echoOutput{Message: args.Message}, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("Server failed: %v", err)
		os.Exit(1)
	}
}

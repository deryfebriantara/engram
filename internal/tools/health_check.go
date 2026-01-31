package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/embedder"
	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var healthCheckTool = mcp.NewTool("health_check",
	mcp.WithDescription("Check the health of the memory system, including ChromaDB and Ollama connectivity, and report the total memory count."),
)

func NewHealthCheckHandler(svc *memory.Service, emb *embedder.OllamaEmbedder) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		chromaStatus := "ok"
		if err := svc.Healthy(ctx); err != nil {
			chromaStatus = fmt.Sprintf("error: %s", err)
		}

		ollamaStatus := "ok"
		if err := emb.Healthy(); err != nil {
			ollamaStatus = fmt.Sprintf("error: %s", err)
		}

		count := -1
		if chromaStatus == "ok" {
			if c, err := svc.Count(ctx); err == nil {
				count = c
			}
		}

		result := fmt.Sprintf(
			"Memory System Health:\n"+
				"  ChromaDB:     %s\n"+
				"  Ollama:       %s (model: %s)\n"+
				"  Total memories: %d",
			chromaStatus, ollamaStatus, emb.Model(), count,
		)

		return mcp.NewToolResultText(result), nil
	}
}

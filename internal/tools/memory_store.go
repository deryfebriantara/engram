package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var memoryStoreTool = mcp.NewTool("memory_store",
	mcp.WithDescription("Save a new memory. Use this to store information you want to remember across sessions, such as user preferences, project decisions, patterns, or facts."),
	mcp.WithString("content",
		mcp.Required(),
		mcp.Description("Memory text to embed and store"),
	),
	mcp.WithString("category",
		mcp.Description("Category of memory"),
		mcp.Enum("preference", "project", "pattern", "decision", "fact"),
	),
	mcp.WithArray("tags",
		mcp.Description("Tags for filtering and organization"),
	),
	mcp.WithString("source",
		mcp.Description("Project or session context where this memory originated"),
	),
)

func NewMemoryStoreHandler(svc *memory.Service) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("content is required"), nil
		}

		category := req.GetString("category", "fact")
		source := req.GetString("source", "")
		tags := req.GetStringSlice("tags", nil)

		outcome, err := svc.Store(ctx, memory.StoreRequest{
			Content:  content,
			Category: category,
			Tags:     tags,
			Source:   source,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to store memory: %s", err)), nil
		}

		data, _ := json.MarshalIndent(outcome.Memory, "", "  ")
		if outcome.SupersededID != "" {
			return mcp.NewToolResultText(fmt.Sprintf("Merged with existing similar memory (superseded %s):\n%s", outcome.SupersededID, string(data))), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Memory stored successfully:\n%s", string(data))), nil
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var memoryDeleteTool = mcp.NewTool("memory_delete",
	mcp.WithDescription("Delete a memory by its ID."),
	mcp.WithString("id",
		mcp.Required(),
		mcp.Description("UUID of the memory to delete"),
	),
)

func NewMemoryDeleteHandler(svc *memory.Service) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		if err := svc.Delete(ctx, id); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete memory: %s", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Memory %s deleted successfully.", id)), nil
	}
}

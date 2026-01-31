package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var memoryListTool = mcp.NewTool("memory_list",
	mcp.WithDescription("List memories with optional filters. Use this to browse stored memories by category or tags."),
	mcp.WithString("category",
		mcp.Description("Filter by category"),
		mcp.Enum("preference", "project", "pattern", "decision", "fact"),
	),
	mcp.WithArray("tags",
		mcp.Description("Filter by tags"),
	),
	mcp.WithNumber("limit",
		mcp.Description("Maximum number of results (1-50, default 10)"),
	),
	mcp.WithNumber("offset",
		mcp.Description("Pagination offset (default 0)"),
	),
)

func NewMemoryListHandler(svc *memory.Service) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := req.GetString("category", "")
		tags := req.GetStringSlice("tags", nil)
		limit := req.GetInt("limit", 10)
		offset := req.GetInt("offset", 0)

		memories, err := svc.List(ctx, memory.ListRequest{
			Category: category,
			Tags:     tags,
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list memories: %s", err)), nil
		}

		if len(memories) == 0 {
			return mcp.NewToolResultText("No memories found."), nil
		}

		data, _ := json.MarshalIndent(memories, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Found %d memories:\n%s", len(memories), string(data))), nil
	}
}

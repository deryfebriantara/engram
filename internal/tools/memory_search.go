package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var memorySearchTool = mcp.NewTool("memory_search",
	mcp.WithDescription("Search memories using semantic similarity. Use natural language queries to find relevant memories."),
	mcp.WithString("query",
		mcp.Required(),
		mcp.Description("Natural language search query"),
	),
	mcp.WithString("category",
		mcp.Description("Filter by category"),
		mcp.Enum("preference", "project", "pattern", "decision", "fact"),
	),
	mcp.WithArray("tags",
		mcp.Description("Filter by tags"),
	),
	mcp.WithNumber("limit",
		mcp.Description("Maximum number of results (1-20, default 5)"),
	),
)

func NewMemorySearchHandler(svc *memory.Service) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query is required"), nil
		}

		category := req.GetString("category", "")
		tags := req.GetStringSlice("tags", nil)
		limit := req.GetInt("limit", 5)

		results, err := svc.Search(ctx, memory.SearchRequest{
			Query:    query,
			Category: category,
			Tags:     tags,
			Limit:    limit,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to search memories: %s", err)), nil
		}

		if len(results) == 0 {
			return mcp.NewToolResultText("No memories found matching your query."), nil
		}

		data, _ := json.MarshalIndent(results, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Found %d memories:\n%s", len(results), string(data))), nil
	}
}

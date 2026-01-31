package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

var memoryUpdateTool = mcp.NewTool("memory_update",
	mcp.WithDescription("Update an existing memory. If content changes, the embedding is automatically re-generated."),
	mcp.WithString("id",
		mcp.Required(),
		mcp.Description("UUID of the memory to update"),
	),
	mcp.WithString("content",
		mcp.Description("New content for the memory"),
	),
	mcp.WithString("category",
		mcp.Description("New category"),
		mcp.Enum("preference", "project", "pattern", "decision", "fact"),
	),
	mcp.WithArray("tags",
		mcp.Description("New tags (replaces all existing tags)"),
	),
)

func NewMemoryUpdateHandler(svc *memory.Service) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}

		updateReq := memory.UpdateRequest{ID: id}

		args := req.GetArguments()
		if v, ok := args["content"].(string); ok {
			updateReq.Content = &v
		}
		if v, ok := args["category"].(string); ok {
			updateReq.Category = &v
		}
		if v, ok := args["tags"]; ok {
			if tagSlice, ok := v.([]interface{}); ok {
				tags := make([]string, 0, len(tagSlice))
				for _, t := range tagSlice {
					if s, ok := t.(string); ok {
						tags = append(tags, s)
					}
				}
				updateReq.Tags = tags
			}
		}

		mem, err := svc.Update(ctx, updateReq)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update memory: %s", err)), nil
		}

		data, _ := json.MarshalIndent(mem, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Memory updated successfully:\n%s", string(data))), nil
	}
}

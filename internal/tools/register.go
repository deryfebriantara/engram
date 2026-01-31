package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/deryfebriantara/my-claude-memory/internal/embedder"
	"github.com/deryfebriantara/my-claude-memory/internal/memory"
)

func Register(s *server.MCPServer, svc *memory.Service, emb *embedder.OllamaEmbedder) {
	s.AddTools(
		server.ServerTool{Tool: memoryStoreTool, Handler: NewMemoryStoreHandler(svc)},
		server.ServerTool{Tool: memorySearchTool, Handler: NewMemorySearchHandler(svc)},
		server.ServerTool{Tool: memoryListTool, Handler: NewMemoryListHandler(svc)},
		server.ServerTool{Tool: memoryDeleteTool, Handler: NewMemoryDeleteHandler(svc)},
		server.ServerTool{Tool: memoryUpdateTool, Handler: NewMemoryUpdateHandler(svc)},
		server.ServerTool{Tool: healthCheckTool, Handler: NewHealthCheckHandler(svc, emb)},
	)
}

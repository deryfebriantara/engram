package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/deryfebriantara/my-claude-memory/internal/chromastore"
	"github.com/deryfebriantara/my-claude-memory/internal/config"
	"github.com/deryfebriantara/my-claude-memory/internal/embedder"
	"github.com/deryfebriantara/my-claude-memory/internal/memory"
	"github.com/deryfebriantara/my-claude-memory/internal/tools"
)

func main() {
	logger := log.New(os.Stderr, "[claude-memory] ", log.LstdFlags)

	cfg := config.Load()

	emb, err := embedder.NewOllama(cfg.OllamaURL, cfg.OllamaModel)
	if err != nil {
		logger.Fatalf("Failed to create embedder: %v", err)
	}

	ctx := context.Background()
	store, err := chromastore.New(ctx, cfg.ChromaURL, cfg.CollectionName, emb)
	if err != nil {
		logger.Fatalf("Failed to create store: %v", err)
	}

	svc := memory.NewService(store, cfg.DedupThreshold)

	s := server.NewMCPServer(
		"claude-memory",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	tools.Register(s, svc, emb)

	logger.Println("Starting claude-memory MCP server via stdio...")
	if err := server.ServeStdio(s, server.WithErrorLogger(logger)); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

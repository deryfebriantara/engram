BINARY=claude-memory-server
CMD=./cmd/claude-memory-server
CLI_BINARY=engram
CLI_CMD=./cmd/engram
INSTALL_DIR=$(HOME)/.local/bin
HOOKS_DIR=$(HOME)/.claude/hooks

.PHONY: build install test infra infra-down register install-hooks clean

build:
	go build -o $(BINARY) $(CMD)
	go build -o $(CLI_BINARY) $(CLI_CMD)

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	cp $(CLI_BINARY) $(INSTALL_DIR)/$(CLI_BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY) and $(INSTALL_DIR)/$(CLI_BINARY)"

test:
	go test ./...

infra:
	docker compose up -d
	@echo "Waiting for ChromaDB..."
	@until curl -sf http://localhost:8000/api/v2/heartbeat > /dev/null 2>&1; do sleep 1; done
	@echo "ChromaDB is ready."

infra-down:
	docker compose down

register:
	@echo "Register this MCP server in Claude Code with:"
	@echo ""
	@echo "  claude mcp add claude-memory $(INSTALL_DIR)/$(BINARY)"
	@echo ""

install-hooks:
	mkdir -p $(HOOKS_DIR)
	cp hooks/engram-recall.sh $(HOOKS_DIR)/engram-recall.sh
	cp hooks/engram-capture.sh $(HOOKS_DIR)/engram-capture.sh
	chmod +x $(HOOKS_DIR)/engram-recall.sh $(HOOKS_DIR)/engram-capture.sh
	@echo "Installed hooks to $(HOOKS_DIR)/"
	@echo ""
	@echo "These are not registered yet. Add to ~/.claude/settings.json:"
	@echo '  UserPromptSubmit -> ~/.claude/hooks/engram-recall.sh'
	@echo '  SessionEnd       -> ~/.claude/hooks/engram-capture.sh'

clean:
	rm -f $(BINARY) $(CLI_BINARY)

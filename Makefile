BINARY=claude-memory-server
CMD=./cmd/claude-memory-server
INSTALL_DIR=$(HOME)/.local/bin

.PHONY: build install test infra infra-down register clean

build:
	go build -o $(BINARY) $(CMD)

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"

test:
	go test ./...

infra:
	docker compose up -d
	@echo "Waiting for ChromaDB..."
	@until curl -sf http://localhost:8000/api/v1/heartbeat > /dev/null 2>&1; do sleep 1; done
	@echo "ChromaDB is ready."

infra-down:
	docker compose down

register:
	@echo "Register this MCP server in Claude Code with:"
	@echo ""
	@echo "  claude mcp add claude-memory $(INSTALL_DIR)/$(BINARY)"
	@echo ""

clean:
	rm -f $(BINARY)

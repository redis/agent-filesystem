PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin

.PHONY: all module mount cli clean test install uninstall install-skill install-skill-local uninstall-skill-local mcp-up mcp-down mcp-logs install-mcp-auggie uninstall-mcp-auggie

all: module mount cli

module:
	$(MAKE) -C module

mount:
	$(MAKE) -C mount

cli:
	$(MAKE) -C cli

install: cli
	@mkdir -p "$(BINDIR)"
	@ln -sf "$(PWD)/raf" "$(BINDIR)/raf"
	@echo "Installed raf -> $(BINDIR)/raf"

uninstall:
	@rm -f "$(BINDIR)/raf"
	@echo "Removed $(BINDIR)/raf"

clean:
	$(MAKE) -C module clean
	$(MAKE) -C mount clean
	$(MAKE) -C cli clean
	$(RM) fs.so fs.xo path.xo

test: module
	$(MAKE) -C module test

# Install skill to all detected agents (requires Node.js/npx)
install-skill:
	@echo "Installing redis-fs skill to all detected agents..."
	npx skills add . --skill redis-fs -g -y

# Install skill to Claude Code only (no Node.js required)
install-skill-local:
	@mkdir -p ~/.claude/skills/redis-fs
	@ln -sf $(PWD)/skills/redis-fs/SKILL.md ~/.claude/skills/redis-fs/SKILL.md
	@echo "Installed redis-fs skill to ~/.claude/skills/redis-fs/"

# Uninstall skill from Claude Code
uninstall-skill-local:
	@rm -rf ~/.claude/skills/redis-fs
	@echo "Uninstalled redis-fs skill from ~/.claude/skills/"

# Build and start MCP server (HTTP) with Redis
mcp-up:
	docker-compose up -d --build
	@echo ""
	@echo "Redis-FS MCP server running at http://localhost:8089/sse"
	@echo "Health check: http://localhost:8089/health"
	@echo ""
	@echo "To add to Auggie, run: make install-mcp-auggie"

# Stop MCP server and Redis
mcp-down:
	docker-compose down

# View MCP server logs
mcp-logs:
	docker-compose logs -f mcp-server

# Add MCP server to Auggie CLI (HTTP transport)
install-mcp-auggie:
	auggie mcp add-json --replace redis-fs '{"type":"sse","url":"http://localhost:8089/sse"}'
	@echo "Added redis-fs MCP server to Auggie (HTTP/SSE)"

# Remove MCP server from Auggie CLI
uninstall-mcp-auggie:
	auggie mcp remove redis-fs
	@echo "Removed redis-fs MCP server from Auggie"

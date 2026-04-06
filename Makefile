PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
NPM ?= npm
UI_DIR ?= ui
WEB_DEV_SCRIPT := scripts/web-dev.sh
UI_NODE_MODULES := $(UI_DIR)/node_modules
AFS_WEB_SERVER_ADDR ?= 127.0.0.1:8091
AFS_WEB_ALLOW_ORIGIN ?= *
AFS_WEB_API_BASE_URL ?= http://127.0.0.1:8091
AFS_WEB_UI_HOST ?= 127.0.0.1
AFS_WEB_UI_PORT ?= 5173

.PHONY: all module mount cli clean test install uninstall install-skill install-skill-local uninstall-skill-local mcp-up mcp-down mcp-logs install-mcp-auggie uninstall-mcp-auggie web-install web-server web-ui web-dev

all: module mount cli

module:
	$(MAKE) -C module

mount:
	$(MAKE) -C mount

cli:
	$(MAKE) -C cli

install: cli
	@mkdir -p "$(BINDIR)"
	@ln -sf "$(PWD)/afs" "$(BINDIR)/afs"
	@echo "Installed afs -> $(BINDIR)/afs"

uninstall:
	@rm -f "$(BINDIR)/afs"
	@echo "Removed $(BINDIR)/afs"

clean:
	$(MAKE) -C module clean
	$(MAKE) -C mount clean
	$(MAKE) -C cli clean
	$(RM) fs.so fs.xo path.xo

test: module
	$(MAKE) -C module test

$(UI_NODE_MODULES):
	cd "$(UI_DIR)" && $(NPM) install

web-install: $(UI_NODE_MODULES)

web-server: cli
	./afs-server --listen "$(AFS_WEB_SERVER_ADDR)" --allow-origin "$(AFS_WEB_ALLOW_ORIGIN)"

web-ui: $(UI_NODE_MODULES)
	cd "$(UI_DIR)" && VITE_AFS_API_BASE_URL="$(AFS_WEB_API_BASE_URL)" $(NPM) run dev -- --host "$(AFS_WEB_UI_HOST)" --port "$(AFS_WEB_UI_PORT)"

web-dev: cli $(UI_NODE_MODULES)
	@AFS_WEB_SERVER_BIN="$(PWD)/afs-server" \
	AFS_WEB_SERVER_ADDR="$(AFS_WEB_SERVER_ADDR)" \
	AFS_WEB_ALLOW_ORIGIN="$(AFS_WEB_ALLOW_ORIGIN)" \
	AFS_WEB_API_BASE_URL="$(AFS_WEB_API_BASE_URL)" \
	AFS_WEB_UI_DIR="$(PWD)/$(UI_DIR)" \
	AFS_WEB_UI_HOST="$(AFS_WEB_UI_HOST)" \
	AFS_WEB_UI_PORT="$(AFS_WEB_UI_PORT)" \
	AFS_WEB_NPM_BIN="$(NPM)" \
	"$(PWD)/$(WEB_DEV_SCRIPT)"

# Install skill to all detected agents (requires Node.js/npx)
install-skill:
	@echo "Installing agent-filesystem skill to all detected agents..."
	npx skills add . --skill agent-filesystem -g -y

# Install skill to Claude Code only (no Node.js required)
install-skill-local:
	@mkdir -p ~/.claude/skills/agent-filesystem
	@ln -sf $(PWD)/skills/agent-filesystem/SKILL.md ~/.claude/skills/agent-filesystem/SKILL.md
	@echo "Installed agent-filesystem skill to ~/.claude/skills/agent-filesystem/"

# Uninstall skill from Claude Code
uninstall-skill-local:
	@rm -rf ~/.claude/skills/agent-filesystem
	@echo "Uninstalled agent-filesystem skill from ~/.claude/skills/"

# Build and start MCP server (HTTP) with Redis
mcp-up:
	docker-compose up -d --build
	@echo ""
	@echo "Agent Filesystem MCP server running at http://localhost:8089/sse"
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
	auggie mcp add-json --replace agent-filesystem '{"type":"sse","url":"http://localhost:8089/sse"}'
	@echo "Added agent-filesystem MCP server to Auggie (HTTP/SSE)"

# Remove MCP server from Auggie CLI
uninstall-mcp-auggie:
	auggie mcp remove agent-filesystem
	@echo "Removed agent-filesystem MCP server from Auggie"

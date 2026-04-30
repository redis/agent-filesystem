PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
NPM ?= npm
UI_DIR ?= ui

# Version metadata injected via ldflags. Core AFS releases use product tags
# named `vX.Y.Z` or `afs-vX.Y.Z`; package-specific SDK tags are intentionally
# ignored so they never leak into the CLI/control-plane version string.
# Consumers can override AFS_VERSION at make time to force a specific value,
# e.g. for CI tagging.
AFS_VERSION_BASE ?= v0.1.0
AFS_VERSION ?= $(shell AFS_VERSION_BASE="$(AFS_VERSION_BASE)" scripts/resolve-afs-version.sh 2>/dev/null || echo dev)
AFS_COMMIT := $(shell git rev-parse --short=7 HEAD 2>/dev/null)
AFS_BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_LDFLAGS := -X github.com/redis/agent-filesystem/internal/version.Version=$(AFS_VERSION) \
                   -X github.com/redis/agent-filesystem/internal/version.Commit=$(AFS_COMMIT) \
                   -X github.com/redis/agent-filesystem/internal/version.BuildDate=$(AFS_BUILD_DATE)
WEB_DEV_SCRIPT := scripts/web-dev.sh
UI_NODE_MODULES := $(UI_DIR)/node_modules
AFS_WEB_SERVER_ADDR ?= 127.0.0.1:8091
AFS_WEB_ALLOW_ORIGIN ?= *
AFS_WEB_API_BASE_URL ?= http://127.0.0.1:8091
AFS_WEB_CLIENT_MODE ?=
AFS_WEB_UI_HOST ?= 127.0.0.1
AFS_WEB_UI_PORT ?= 5173

.DEFAULT_GOAL := all

.PHONY: help all mount commands afs afs-control-plane afs-control-plane-noui clean test install uninstall install-skill install-skill-local uninstall-skill-local templates-generate web-install web-build embed-ui web-server web-ui web-dev

help: ## Show repo-specific make targets and common variables.
	@printf '%s\n' 'AFS make targets:'
	@printf '  %-20s %s\n' 'help' 'Show this help.'
	@printf '  %-20s %s\n' 'all' 'Build mount helpers and Go commands.'
	@printf '  %-20s %s\n' 'mount' 'Build the FUSE and NFS mount helpers.'
	@printf '  %-20s %s\n' 'commands' 'Build the afs and afs-control-plane binaries.'
	@printf '  %-20s %s\n' 'afs' 'Build the afs CLI binary.'
	@printf '  %-20s %s\n' 'afs-control-plane' 'Build the HTTP control-plane binary.'
	@printf '  %-20s %s\n' 'test' 'Run Go unit tests for the active product surfaces.'
	@printf '  %-20s %s\n' 'clean' 'Remove compiled artifacts.'
	@printf '  %-20s %s\n' 'install' 'Symlink ./afs into $(BINDIR).'
	@printf '  %-20s %s\n' 'uninstall' 'Remove the installed afs symlink from $(BINDIR).'
	@printf '  %-20s %s\n' 'templates-generate' 'Regenerate UI template data from templates/.'
	@printf '  %-20s %s\n' 'web-install' 'Install UI dependencies into $(UI_DIR).'
	@printf '  %-20s %s\n' 'web-server' 'Run afs-control-plane on $(AFS_WEB_SERVER_ADDR).'
	@printf '  %-20s %s\n' 'web-ui' 'Run the Vite UI against $(AFS_WEB_API_BASE_URL).'
	@printf '  %-20s %s\n' 'web-dev' 'Run the control plane and UI together.'
	@printf '  %-20s %s\n' 'install-skill' 'Install the skill via npx skills.'
	@printf '  %-20s %s\n' 'install-skill-local' 'Install the skill into ~/.claude/skills.'
	@printf '  %-20s %s\n' 'uninstall-skill-local' 'Remove the locally installed Claude skill.'
	@printf '\n%s\n' 'Common overrides:'
	@printf '  %-24s %s\n' 'BINDIR=/path/bin' 'Install destination for make install.'
	@printf '  %-24s %s\n' 'AFS_WEB_SERVER_ADDR=host:port' 'Bind address for web-server/web-dev.'
	@printf '  %-24s %s\n' 'AFS_WEB_API_BASE_URL=url' 'API base URL passed to the UI.'
	@printf '  %-24s %s\n' 'AFS_WEB_CLIENT_MODE=demo' 'Explicitly opt into demo UI fixtures.'
	@printf '  %-24s %s\n' 'AFS_WEB_UI_HOST=host' 'Host for the Vite dev server.'
	@printf '  %-24s %s\n' 'AFS_WEB_UI_PORT=port' 'Port for the Vite dev server.'
	@printf '\n%s\n' 'Note: GNU make handles `make --help` itself before reading this Makefile, so use `make help` for repo-specific target docs.'

all: ## Build mount helpers and Go commands.
all: mount commands

mount: ## Build the FUSE and NFS mount helpers.
	$(MAKE) -C mount

commands: ## Build the afs and afs-control-plane binaries.
commands: afs afs-control-plane

afs: ## Build the afs CLI binary.
	go build -ldflags "$(VERSION_LDFLAGS)" -o afs ./cmd/afs

afs-control-plane: ## Build the HTTP control-plane binary (with embedded UI).
afs-control-plane: embed-ui
	go build -ldflags "$(VERSION_LDFLAGS)" -o afs-control-plane ./cmd/afs-control-plane

afs-control-plane-noui: ## Build the control-plane binary without embedded UI.
	go build -ldflags "$(VERSION_LDFLAGS)" -o afs-control-plane ./cmd/afs-control-plane

install: ## Symlink ./afs into $(BINDIR).
install: afs
	@mkdir -p "$(BINDIR)"
	@ln -sf "$(CURDIR)/afs" "$(BINDIR)/afs"
	@echo "Installed afs -> $(BINDIR)/afs"

uninstall: ## Remove the installed afs symlink from $(BINDIR).
	@rm -f "$(BINDIR)/afs"
	@echo "Removed $(BINDIR)/afs"

clean: ## Remove compiled artifacts.
	$(MAKE) -C mount clean
	$(RM) afs afs-control-plane afs-server

test: ## Run Go unit tests for the active product surfaces.
	go test ./cmd/... ./deploy/... ./internal/...
	cd mount && go test ./...

$(UI_NODE_MODULES):
	cd "$(UI_DIR)" && $(NPM) install

web-install: ## Install UI dependencies into $(UI_DIR).
web-install: $(UI_NODE_MODULES)

templates-generate: ## Regenerate UI template data from templates/.
	cd "$(UI_DIR)" && $(NPM) run templates:generate

web-build: ## Build the UI for production.
web-build: $(UI_NODE_MODULES)
	cd "$(UI_DIR)" && $(NPM) run build

embed-ui: ## Build UI and copy into Go embed directory.
embed-ui: web-build
	rm -rf internal/uistatic/dist
	cp -r "$(UI_DIR)/dist" internal/uistatic/dist
	touch internal/uistatic/dist/.keep

web-server: ## Run afs-control-plane on $(AFS_WEB_SERVER_ADDR).
web-server: afs-control-plane
	./afs-control-plane --listen "$(AFS_WEB_SERVER_ADDR)" --allow-origin "$(AFS_WEB_ALLOW_ORIGIN)"

web-ui: ## Run the Vite UI against $(AFS_WEB_API_BASE_URL).
web-ui: $(UI_NODE_MODULES)
	cd "$(UI_DIR)" && VITE_AFS_API_BASE_URL="$(AFS_WEB_API_BASE_URL)" VITE_AFS_CLIENT_MODE="$(AFS_WEB_CLIENT_MODE)" $(NPM) run dev -- --host "$(AFS_WEB_UI_HOST)" --port "$(AFS_WEB_UI_PORT)"

web-dev: ## Run the control plane and UI together.
web-dev: commands $(UI_NODE_MODULES)
	@AFS_WEB_SERVER_BIN="$(PWD)/afs-control-plane" \
	AFS_WEB_SERVER_ADDR="$(AFS_WEB_SERVER_ADDR)" \
	AFS_WEB_ALLOW_ORIGIN="$(AFS_WEB_ALLOW_ORIGIN)" \
	AFS_WEB_API_BASE_URL="$(AFS_WEB_API_BASE_URL)" \
	AFS_WEB_CLIENT_MODE="$(AFS_WEB_CLIENT_MODE)" \
	AFS_WEB_UI_DIR="$(PWD)/$(UI_DIR)" \
	AFS_WEB_UI_HOST="$(AFS_WEB_UI_HOST)" \
	AFS_WEB_UI_PORT="$(AFS_WEB_UI_PORT)" \
	AFS_WEB_NPM_BIN="$(NPM)" \
	"$(PWD)/$(WEB_DEV_SCRIPT)"

# Install skill to all detected agents (requires Node.js/npx)
install-skill: ## Install the skill via npx skills.
	@echo "Installing agent-filesystem skill to all detected agents..."
	npx skills add . --skill agent-filesystem -g -y

# Install skill to Claude Code only (no Node.js required)
install-skill-local: ## Install the skill into ~/.claude/skills.
	@mkdir -p ~/.claude/skills/agent-filesystem
	@ln -sf $(PWD)/skills/agent-filesystem/SKILL.md ~/.claude/skills/agent-filesystem/SKILL.md
	@echo "Installed agent-filesystem skill to ~/.claude/skills/agent-filesystem/"

# Uninstall skill from Claude Code
uninstall-skill-local: ## Remove the locally installed Claude skill.
	@rm -rf ~/.claude/skills/agent-filesystem
	@echo "Uninstalled agent-filesystem skill from ~/.claude/skills/"

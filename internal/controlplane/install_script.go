package controlplane

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"text/template"
)

// installScriptTemplate is the source of /install.sh. It points the downloader
// at the same control plane that served the script, so whichever domain the
// user curls (redis-afs.com, agentfilesystem.ai, localhost:8091) is where the
// binary comes from.
const installScriptTemplate = `#!/usr/bin/env bash
#
# Agent Filesystem CLI installer
#
# Usage: curl -fsSL {{.BaseURL}}/install.sh | bash
#

set -euo pipefail

CONTROL_PLANE="{{.BaseURL}}"
INSTALL_DIR="${AFS_INSTALL_DIR:-$HOME/.afs/bin}"
BIN_NAME="afs"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn()  { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
fail()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

# Detect OS.
raw_os=$(uname -s)
case "$raw_os" in
  Darwin) os="darwin" ;;
  Linux)  os="linux"  ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT)
    fail "Windows is not supported yet. See {{.BaseURL}}/docs for alternatives." ;;
  *) fail "Unsupported operating system: $raw_os" ;;
esac

# Detect architecture.
raw_arch=$(uname -m)
case "$raw_arch" in
  x86_64|amd64)   arch="amd64" ;;
  arm64|aarch64)  arch="arm64" ;;
  *) fail "Unsupported architecture: $raw_arch" ;;
esac

info "Installing afs for ${os}/${arch}"

command -v curl >/dev/null 2>&1 || fail "curl is required to install afs."

mkdir -p "$INSTALL_DIR"

tmp_file=$(mktemp)
cleanup() { rm -f "$tmp_file"; }
trap cleanup EXIT

info "Downloading from ${CONTROL_PLANE}/v1/cli"
if ! curl -fSL --progress-bar -o "$tmp_file" "${CONTROL_PLANE}/v1/cli?os=${os}&arch=${arch}"; then
  fail "Download failed. Check your network connection and try again."
fi

chmod +x "$tmp_file"

# Strip macOS quarantine so Gatekeeper lets the binary run.
if [ "$os" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$tmp_file" 2>/dev/null || true
fi

target="$INSTALL_DIR/$BIN_NAME"
mv -f "$tmp_file" "$target"
trap - EXIT

info "Installed to $target"

# PATH check.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo
    warn "$INSTALL_DIR is not on your PATH."
    echo "    Add this line to your ~/.bashrc or ~/.zshrc:"
    echo
    echo "        export PATH=\"$INSTALL_DIR:\$PATH\""
    echo
    ;;
esac

{{if eq .ProductMode "cloud"}}info "Installation complete."
echo
echo "Next:"
echo "    afs login       # sign in and link this CLI to your account"
echo "    afs up          # start syncing your current workspace"
{{else}}info "Pointing CLI at ${CONTROL_PLANE}"
if ! "$target" login --self-hosted --url "$CONTROL_PLANE" >/dev/null 2>&1; then
  warn "Could not configure the CLI automatically. Run this later:"
  echo "    afs login --self-hosted --url $CONTROL_PLANE"
fi

info "Installation complete."
echo
echo "Next:"
echo "    afs setup       # pick a workspace and local path"
echo "    afs up          # start syncing your current workspace"
{{end}}`

type installScriptData struct {
	BaseURL     string
	ProductMode string
}

func renderInstallScript(r *http.Request) (string, error) {
	tmpl, err := template.New("install").Parse(installScriptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	data := installScriptData{
		BaseURL:     installScriptBaseURL(r),
		ProductMode: ProductModeFromEnv(),
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// installScriptBaseURL derives the public origin (scheme://host) of the server
// that just handled this request, preferring proxy headers so Vercel / nginx
// in front of the control plane surface the correct public hostname.
func installScriptBaseURL(r *http.Request) string {
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = strings.ToLower(proto)
	} else if r.TLS == nil {
		scheme = "http"
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, fmt.Errorf("%s not allowed", r.Method))
		return
	}
	body, err := renderInstallScript(r)
	if err != nil {
		writeError(w, fmt.Errorf("render install script: %w", err))
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}

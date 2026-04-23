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
// user curls (afs.cloud, localhost:8091, or a self-managed host) is where
// the binary comes from.
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
CLI_CMD="afs"

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

path_line_for_shell() {
  case "$1" in
    fish)
      printf 'fish_add_path -m "%s"\n' "$INSTALL_DIR"
      ;;
    *)
      printf 'export PATH="%s:$PATH"\n' "$INSTALL_DIR"
      ;;
  esac
}

profile_file_for_shell() {
  case "$1" in
    zsh)
      printf '%s\n' "$HOME/.zshrc"
      ;;
    bash)
      if [ "$os" = "darwin" ]; then
        if [ -f "$HOME/.bash_profile" ] || [ ! -f "$HOME/.bashrc" ]; then
          printf '%s\n' "$HOME/.bash_profile"
        else
          printf '%s\n' "$HOME/.bashrc"
        fi
      else
        if [ -f "$HOME/.bashrc" ] || [ ! -f "$HOME/.bash_profile" ]; then
          printf '%s\n' "$HOME/.bashrc"
        else
          printf '%s\n' "$HOME/.bash_profile"
        fi
      fi
      ;;
    fish)
      printf '%s\n' "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"
      ;;
    *)
      return 1
      ;;
  esac
}

configure_shell_path() {
  shell_name="${SHELL##*/}"
  if [ -z "$shell_name" ]; then
    shell_name="bash"
  fi

  profile_file=$(profile_file_for_shell "$shell_name") || return 1
  path_line=$(path_line_for_shell "$shell_name")

  mkdir -p "$(dirname "$profile_file")"

  if [ -f "$profile_file" ] && grep -Fqx "$path_line" "$profile_file"; then
    info "$INSTALL_DIR is already configured in $profile_file"
    return 0
  fi

  {
    printf '\n'
    printf '# Added by Agent Filesystem installer\n'
    printf '%s\n' "$path_line"
  } >> "$profile_file"

  info "Added $INSTALL_DIR to PATH in $profile_file"
}

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

# PATH setup.
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    info "$INSTALL_DIR is already on PATH"
    ;;
  *)
    CLI_CMD="$target"
    if configure_shell_path; then
      export PATH="$INSTALL_DIR:$PATH"
      info "Open a new shell to use afs by name. Using $target for the next steps."
    else
      echo
      warn "Could not update your shell profile automatically."
      echo "    Add $INSTALL_DIR to your PATH manually if needed."
      echo
    fi
    ;;
esac

{{if eq .ProductMode "cloud"}}info "Installation complete."
echo
echo "Next:"
echo "    $CLI_CMD login       # sign in and link this CLI to your account"
echo "    $CLI_CMD up          # start syncing your current workspace"
{{else}}info "Pointing CLI at ${CONTROL_PLANE}"
if ! "$target" login --self-hosted --url "$CONTROL_PLANE" >/dev/null 2>&1; then
  warn "Could not configure the CLI automatically. Run this later:"
  echo "    $CLI_CMD login --self-hosted --url $CONTROL_PLANE"
fi

info "Installation complete."
echo
echo "Next:"
echo "    $CLI_CMD setup       # pick a workspace and local path"
echo "    $CLI_CMD up          # start syncing your current workspace"
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

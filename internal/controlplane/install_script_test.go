package controlplane

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderInstallScriptInjectsRequestOrigin(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "afs.cloud"
	req.Header.Set("X-Forwarded-Proto", "https")

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	if !strings.Contains(body, `CONTROL_PLANE="https://afs.cloud"`) {
		t.Fatalf("install script missing baked control plane; got:\n%s", body)
	}
	for _, want := range []string{
		`curl -fSL`,
		`uname -s`,
		`uname -m`,
		`CLI_CMD="afs"`,
		`INSTALL_DIR="${AFS_INSTALL_DIR:-$HOME/.afs/bin}"`,
		`configure_shell_path()`,
		`# Added by Agent Filesystem installer`,
		`# Added by Agent Filesystem installer: shell integration`,
		`AFS_ATTACH_CD_FILE`,
		`cd "\$_afs_target"`,
		`set -euo pipefail`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("install script missing %q", want)
		}
	}
	for _, unwanted := range []string{
		`Add this line to your ~/.bashrc or ~/.zshrc:`,
		`warn "$INSTALL_DIR is not on your PATH."`,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("install script should not prompt for manual PATH edits; found %q", unwanted)
		}
	}
}

func TestInstallScriptDefaultsToHostHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "localhost:8091"

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	if !strings.Contains(body, `CONTROL_PLANE="http://localhost:8091"`) {
		t.Fatalf("install script should default to http on localhost; got:\n%s", body)
	}
}

func TestInstallScriptSelfHostedAutoConfigures(t *testing.T) {
	// Default (no env var) is self-hosted; the rendered script should skip the
	// cloud sign-in hint and instead invoke the self-hosted login flow against
	// the serving control plane so `afs ws attach` works without further setup.
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "afs.internal.example"

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	for _, want := range []string{
		`"$target" auth login --self-hosted --url "$CONTROL_PLANE"`,
		`$CLI_CMD ws attach`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("self-hosted install script missing %q; got:\n%s", want, body)
		}
	}
	// `afs auth login` with `--self-hosted` is expected, but the cloud-only
	// `afs auth login` line (no flags) should not appear.
	if strings.Contains(body, "echo \"    afs auth login  # sign in") {
		t.Errorf("self-hosted install script leaked cloud sign-in hint; got:\n%s", body)
	}
}

func TestInstallScriptCloudShowsLoginPrompt(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "afs.cloud"
	req.Header.Set("X-Forwarded-Proto", "https")

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	if !strings.Contains(body, `$CLI_CMD auth login`) {
		t.Errorf("cloud install script missing `$CLI_CMD auth login`; got:\n%s", body)
	}
	if strings.Contains(body, `--self-hosted`) {
		t.Errorf("cloud install script should not auto-configure CLI; got:\n%s", body)
	}
}

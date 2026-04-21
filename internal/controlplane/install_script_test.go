package controlplane

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderInstallScriptInjectsRequestOrigin(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "redis-afs.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	if !strings.Contains(body, `CONTROL_PLANE="https://redis-afs.com"`) {
		t.Fatalf("install script missing baked control plane; got:\n%s", body)
	}
	for _, want := range []string{
		`curl -fSL`,
		`uname -s`,
		`uname -m`,
		`afs login`,
		`INSTALL_DIR="${AFS_INSTALL_DIR:-$HOME/.afs/bin}"`,
		`set -euo pipefail`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("install script missing %q", want)
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
	// cloud sign-in hint and instead invoke `afs config set` against the
	// serving control plane so `afs up` works without further setup.
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "afs.internal.example"

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	for _, want := range []string{
		`"$target" login --self-hosted --url "$CONTROL_PLANE"`,
		`afs setup`,
		`afs up`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("self-hosted install script missing %q; got:\n%s", want, body)
		}
	}
	// `afs login` with `--self-hosted` is expected, but the cloud-only
	// `afs login` line (no flags) should not appear.
	if strings.Contains(body, "echo \"    afs login       # sign in") {
		t.Errorf("self-hosted install script leaked cloud sign-in hint; got:\n%s", body)
	}
}

func TestInstallScriptCloudShowsLoginPrompt(t *testing.T) {
	t.Setenv(ProductModeEnvVar, ProductModeCloud)
	req := httptest.NewRequest("GET", "/install.sh", nil)
	req.Host = "agentfilesystem.ai"
	req.Header.Set("X-Forwarded-Proto", "https")

	body, err := renderInstallScript(req)
	if err != nil {
		t.Fatalf("renderInstallScript returned error: %v", err)
	}
	if !strings.Contains(body, `afs login`) {
		t.Errorf("cloud install script missing `afs login`; got:\n%s", body)
	}
	if strings.Contains(body, `--self-hosted`) {
		t.Errorf("cloud install script should not auto-configure CLI; got:\n%s", body)
	}
}

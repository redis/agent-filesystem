package controlplane

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderInstallScriptInjectsRequestOrigin(t *testing.T) {
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
		`afs onboard`,
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

package controlplane

import "testing"

func TestMCPProfileFileDeleteRequiresWriteAccess(t *testing.T) {
	t.Helper()

	for _, profile := range []string{MCPProfileWorkspaceRO, MCPProfileAdminRO} {
		if MCPProfileAllowsTool(profile, "file_delete") {
			t.Fatalf("MCPProfileAllowsTool(%q, file_delete) = true, want false", profile)
		}
	}

	for _, profile := range []string{MCPProfileWorkspaceRW, MCPProfileWorkspaceRWCheckpoint, MCPProfileAdminRW} {
		if !MCPProfileAllowsTool(profile, "file_delete") {
			t.Fatalf("MCPProfileAllowsTool(%q, file_delete) = false, want true", profile)
		}
	}
}

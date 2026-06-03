package controlplane

import (
	"fmt"
	"strings"
)

const (
	MCPProfileWorkspaceRO           = "workspace-ro"
	MCPProfileWorkspaceRW           = "workspace-rw"
	MCPProfileWorkspaceRWCheckpoint = "workspace-rw-checkpoint"
	MCPProfileAdminRO               = "admin-ro"
	MCPProfileAdminRW               = "admin-rw"
)

const (
	MCPCapabilityRO           = "ro"
	MCPCapabilityRW           = "rw"
	MCPCapabilityRWCheckpoint = "rw-checkpoint"
	MCPCapabilityAdmin        = "admin"
)

var (
	workspaceReadTools = map[string]struct{}{
		"file_read":                       {},
		"file_history":                    {},
		"file_read_version":               {},
		"file_diff_versions":              {},
		"file_lines":                      {},
		"file_list":                       {},
		"file_glob":                       {},
		"file_grep":                       {},
		"file_query":                      {},
		"workspace_get_versioning_policy": {},
	}
	workspaceWriteTools = map[string]struct{}{
		"file_write":                      {},
		"file_create_exclusive":           {},
		"file_replace":                    {},
		"file_insert":                     {},
		"file_delete":                     {},
		"file_delete_lines":               {},
		"file_patch":                      {},
		"file_restore_version":            {},
		"file_undelete":                   {},
		"workspace_set_versioning_policy": {},
	}
	workspaceCheckpointTools = map[string]struct{}{
		"checkpoint_list":    {},
		"checkpoint_create":  {},
		"checkpoint_restore": {},
	}
	adminTools = map[string]struct{}{
		"afs_status":                      {},
		"workspace_list":                  {},
		"workspace_current":               {},
		"workspace_use":                   {},
		"workspace_create":                {},
		"workspace_fork":                  {},
		"workspace_get_versioning_policy": {},
		"workspace_set_versioning_policy": {},
	}
)

func NormalizeMCPProfile(raw string) (string, error) {
	profile := strings.TrimSpace(strings.ToLower(raw))
	if profile == "" {
		return MCPProfileWorkspaceRW, nil
	}
	switch profile {
	case MCPProfileWorkspaceRO, MCPProfileWorkspaceRW, MCPProfileWorkspaceRWCheckpoint, MCPProfileAdminRO, MCPProfileAdminRW:
		return profile, nil
	default:
		return "", fmt.Errorf("unsupported mcp profile %q", raw)
	}
}

func NormalizeMCPCapability(raw string) (string, error) {
	capability := strings.TrimSpace(strings.ToLower(raw))
	if capability == "" {
		return MCPCapabilityRW, nil
	}
	switch capability {
	case MCPCapabilityRO, MCPCapabilityRW, MCPCapabilityRWCheckpoint, MCPCapabilityAdmin:
		return capability, nil
	default:
		return "", fmt.Errorf("unsupported mcp capability %q", raw)
	}
}

func MCPCapabilityFromProfile(profile string) string {
	normalizedProfile, err := NormalizeMCPProfile(profile)
	if err != nil {
		return MCPCapabilityRW
	}
	switch normalizedProfile {
	case MCPProfileWorkspaceRO, MCPProfileAdminRO:
		return MCPCapabilityRO
	case MCPProfileWorkspaceRW:
		return MCPCapabilityRW
	case MCPProfileWorkspaceRWCheckpoint:
		return MCPCapabilityRWCheckpoint
	case MCPProfileAdminRW:
		return MCPCapabilityAdmin
	default:
		return MCPCapabilityRW
	}
}

func MCPProfileFromCapability(scope, capability string) string {
	normalizedCapability, err := NormalizeMCPCapability(capability)
	if err != nil {
		normalizedCapability = MCPCapabilityRW
	}
	if isControlPlaneScope(scope) {
		switch normalizedCapability {
		case MCPCapabilityRO:
			return MCPProfileAdminRO
		default:
			return MCPProfileAdminRW
		}
	}
	switch normalizedCapability {
	case MCPCapabilityRO:
		return MCPProfileWorkspaceRO
	case MCPCapabilityRWCheckpoint, MCPCapabilityAdmin:
		return MCPProfileWorkspaceRWCheckpoint
	default:
		return MCPProfileWorkspaceRW
	}
}

func MCPProfileAllowsTool(profile, tool string) bool {
	normalizedProfile, err := NormalizeMCPProfile(profile)
	if err != nil {
		return false
	}
	tool = strings.TrimSpace(tool)
	switch normalizedProfile {
	case MCPProfileWorkspaceRO:
		return inToolSet(workspaceReadTools, tool)
	case MCPProfileWorkspaceRW:
		return inToolSet(workspaceReadTools, tool) || inToolSet(workspaceWriteTools, tool)
	case MCPProfileWorkspaceRWCheckpoint:
		return inToolSet(workspaceReadTools, tool) || inToolSet(workspaceWriteTools, tool) || inToolSet(workspaceCheckpointTools, tool)
	case MCPProfileAdminRO:
		return inToolSet(adminTools, tool) || inToolSet(workspaceReadTools, tool) || tool == "checkpoint_list"
	case MCPProfileAdminRW:
		return inToolSet(adminTools, tool) || inToolSet(workspaceReadTools, tool) || inToolSet(workspaceWriteTools, tool) || inToolSet(workspaceCheckpointTools, tool)
	default:
		return false
	}
}

func MCPProfileIsReadonly(profile string) bool {
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case MCPProfileWorkspaceRO, MCPProfileAdminRO:
		return true
	default:
		return false
	}
}

func MCPProfileIsWorkspaceBound(profile string) bool {
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case MCPProfileWorkspaceRO, MCPProfileWorkspaceRW, MCPProfileWorkspaceRWCheckpoint:
		return true
	default:
		return false
	}
}

func MCPProfileIncludesCheckpoint(profile string) bool {
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case MCPProfileWorkspaceRWCheckpoint, MCPProfileAdminRO, MCPProfileAdminRW:
		return true
	default:
		return false
	}
}

func inToolSet(set map[string]struct{}, tool string) bool {
	_, ok := set[tool]
	return ok
}

package controlplane

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

const (
	WorkspaceVersioningModeOff   = "off"
	WorkspaceVersioningModeAll   = "all"
	WorkspaceVersioningModePaths = "paths"
)

type WorkspaceVersioningPolicy struct {
	Mode                 string   `json:"mode"`
	IncludeGlobs         []string `json:"include_globs,omitempty"`
	ExcludeGlobs         []string `json:"exclude_globs,omitempty"`
	MaxVersionsPerFile   int      `json:"max_versions_per_file,omitempty"`
	MaxAgeDays           int      `json:"max_age_days,omitempty"`
	MaxTotalBytes        int64    `json:"max_total_bytes,omitempty"`
	LargeFileCutoffBytes int64    `json:"large_file_cutoff_bytes,omitempty"`
}

func DefaultWorkspaceVersioningPolicy() WorkspaceVersioningPolicy {
	return WorkspaceVersioningPolicy{Mode: WorkspaceVersioningModeOff}
}

func NormalizeWorkspaceVersioningPolicy(policy WorkspaceVersioningPolicy) WorkspaceVersioningPolicy {
	normalized := policy
	normalized.Mode = strings.TrimSpace(strings.ToLower(normalized.Mode))
	if normalized.Mode == "" {
		normalized.Mode = WorkspaceVersioningModeOff
	}
	normalized.IncludeGlobs = normalizeGlobList(normalized.IncludeGlobs)
	normalized.ExcludeGlobs = normalizeGlobList(normalized.ExcludeGlobs)
	return normalized
}

func ValidateWorkspaceVersioningPolicy(policy WorkspaceVersioningPolicy) error {
	switch policy.Mode {
	case WorkspaceVersioningModeOff, WorkspaceVersioningModeAll, WorkspaceVersioningModePaths:
	default:
		return fmt.Errorf("unsupported workspace versioning mode %q", policy.Mode)
	}
	if policy.MaxVersionsPerFile < 0 {
		return fmt.Errorf("max_versions_per_file must be non-negative")
	}
	if policy.MaxAgeDays < 0 {
		return fmt.Errorf("max_age_days must be non-negative")
	}
	if policy.MaxTotalBytes < 0 {
		return fmt.Errorf("max_total_bytes must be non-negative")
	}
	if policy.LargeFileCutoffBytes < 0 {
		return fmt.Errorf("large_file_cutoff_bytes must be non-negative")
	}
	if policy.Mode == WorkspaceVersioningModePaths && len(policy.IncludeGlobs) == 0 {
		return fmt.Errorf("include_globs must not be empty when mode is %q", WorkspaceVersioningModePaths)
	}
	for _, glob := range append(slices.Clone(policy.IncludeGlobs), policy.ExcludeGlobs...) {
		if strings.TrimSpace(glob) == "" {
			return fmt.Errorf("glob patterns must not be empty")
		}
	}
	return nil
}

func normalizeGlobList(globs []string) []string {
	if len(globs) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(globs))
	seen := make(map[string]struct{}, len(globs))
	for _, glob := range globs {
		trimmed := strings.TrimSpace(glob)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func WorkspaceVersioningPolicyTracksPath(policy WorkspaceVersioningPolicy, rawPath string) (bool, error) {
	normalized, err := normalizeVersionedPath(rawPath)
	if err != nil {
		return false, err
	}
	relativePath := strings.TrimPrefix(normalized, "/")
	if relativePath == "" {
		return false, nil
	}
	switch policy.Mode {
	case WorkspaceVersioningModeOff:
		return false, nil
	case WorkspaceVersioningModeAll:
		return !matchesAnyVersioningGlob(policy.ExcludeGlobs, relativePath), nil
	case WorkspaceVersioningModePaths:
		if !matchesAnyVersioningGlob(policy.IncludeGlobs, relativePath) {
			return false, nil
		}
		return !matchesAnyVersioningGlob(policy.ExcludeGlobs, relativePath), nil
	default:
		return false, fmt.Errorf("unsupported workspace versioning mode %q", policy.Mode)
	}
}

func matchesAnyVersioningGlob(globs []string, relativePath string) bool {
	for _, glob := range globs {
		if matchVersioningGlob(strings.TrimSpace(glob), relativePath) {
			return true
		}
	}
	return false
}

func matchVersioningGlob(glob, relativePath string) bool {
	pattern := strings.Trim(strings.TrimSpace(glob), "/")
	candidate := strings.Trim(strings.TrimSpace(relativePath), "/")
	if pattern == "" || candidate == "" {
		return false
	}
	return matchVersioningGlobSegments(strings.Split(pattern, "/"), strings.Split(candidate, "/"))
}

func matchVersioningGlobSegments(patternSegments, pathSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0
	}
	if patternSegments[0] == "**" {
		if len(patternSegments) == 1 {
			return true
		}
		for index := 0; index <= len(pathSegments); index++ {
			if matchVersioningGlobSegments(patternSegments[1:], pathSegments[index:]) {
				return true
			}
		}
		return false
	}
	if len(pathSegments) == 0 {
		return false
	}
	matched, err := path.Match(patternSegments[0], pathSegments[0])
	if err != nil || !matched {
		return false
	}
	return matchVersioningGlobSegments(patternSegments[1:], pathSegments[1:])
}

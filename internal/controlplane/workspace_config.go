package controlplane

import (
	"fmt"
	"strings"
)

const (
	WorkspaceQueryChunkStrategyAuto  = "auto"
	WorkspaceQueryChunkStrategyRegex = "regex"
)

type WorkspaceConfig struct {
	Versioning WorkspaceVersioningPolicy `json:"versioning"`
	Query      WorkspaceQueryConfig      `json:"query"`
}

type WorkspaceQueryConfig struct {
	Embeddings WorkspaceQueryEmbeddingsConfig `json:"embeddings"`
}

type WorkspaceQueryEmbeddingsConfig struct {
	Enabled       bool   `json:"enabled"`
	Model         string `json:"model,omitempty"`
	ChunkStrategy string `json:"chunk_strategy,omitempty"`
}

type workspaceConfigRecord struct {
	Query WorkspaceQueryConfig `json:"query"`
}

func DefaultWorkspaceConfig() WorkspaceConfig {
	return WorkspaceConfig{
		Versioning: DefaultWorkspaceVersioningPolicy(),
	}
}

func NormalizeWorkspaceConfig(cfg WorkspaceConfig) WorkspaceConfig {
	normalized := cfg
	normalized.Versioning = NormalizeWorkspaceVersioningPolicy(normalized.Versioning)
	normalized.Query.Embeddings.Model = strings.TrimSpace(normalized.Query.Embeddings.Model)
	normalized.Query.Embeddings.ChunkStrategy = strings.TrimSpace(strings.ToLower(normalized.Query.Embeddings.ChunkStrategy))
	return normalized
}

func ValidateWorkspaceConfig(cfg WorkspaceConfig) error {
	if err := ValidateWorkspaceVersioningPolicy(cfg.Versioning); err != nil {
		return err
	}
	switch cfg.Query.Embeddings.ChunkStrategy {
	case "", WorkspaceQueryChunkStrategyAuto, WorkspaceQueryChunkStrategyRegex:
	default:
		return fmt.Errorf("unsupported query embeddings chunk strategy %q", cfg.Query.Embeddings.ChunkStrategy)
	}
	return nil
}

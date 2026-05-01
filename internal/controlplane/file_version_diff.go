package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type FileVersionDiffOperand struct {
	Ref       string `json:"ref,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Ordinal   int64  `json:"ordinal,omitempty"`
}

type FileVersionDiffResponse struct {
	WorkspaceID string `json:"workspace_id"`
	Path        string `json:"path"`
	From        string `json:"from"`
	To          string `json:"to"`
	Binary      bool   `json:"binary"`
	Diff        string `json:"diff,omitempty"`
}

func (s *Service) DiffFileVersions(ctx context.Context, workspace, rawPath string, from, to FileVersionDiffOperand) (FileVersionDiffResponse, error) {
	normalizedPath, err := normalizeVersionedPath(rawPath)
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	left, err := s.resolveDiffOperand(ctx, workspace, normalizedPath, from)
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	right, err := s.resolveDiffOperand(ctx, workspace, normalizedPath, to)
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	response := FileVersionDiffResponse{
		WorkspaceID: workspace,
		Path:        normalizedPath,
		From:        left.label,
		To:          right.label,
		Binary:      left.binary || right.binary,
	}
	if response.Binary {
		return response, nil
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(left.content),
		B:        difflib.SplitLines(right.content),
		FromFile: left.label,
		ToFile:   right.label,
		Context:  3,
	})
	if err != nil {
		return FileVersionDiffResponse{}, err
	}
	response.Diff = diff
	return response, nil
}

type resolvedDiffOperand struct {
	label   string
	content string
	binary  bool
}

func (s *Service) resolveDiffOperand(ctx context.Context, workspace, normalizedPath string, operand FileVersionDiffOperand) (resolvedDiffOperand, error) {
	switch strings.TrimSpace(strings.ToLower(operand.Ref)) {
	case "head":
		content, err := s.getFileContent(ctx, workspace, "head", normalizedPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return resolvedDiffOperand{}, err
		}
		if errors.Is(err, os.ErrNotExist) {
			content, err = s.getFileContent(ctx, workspace, "working-copy", normalizedPath)
			if err != nil {
				return resolvedDiffOperand{}, err
			}
		}
		return resolvedDiffOperand{label: "head", content: content.Content, binary: content.Binary}, nil
	case "working-copy":
		content, err := s.getFileContent(ctx, workspace, "working-copy", normalizedPath)
		if err != nil {
			return resolvedDiffOperand{}, err
		}
		return resolvedDiffOperand{label: "working-copy", content: content.Content, binary: content.Binary}, nil
	}

	version, err := s.resolveSelectedFileVersion(ctx, workspace, FileVersionSelector{
		VersionID: operand.VersionID,
		FileID:    operand.FileID,
		Ordinal:   operand.Ordinal,
	})
	if err != nil {
		return resolvedDiffOperand{}, err
	}
	if version.Path != normalizedPath {
		return resolvedDiffOperand{}, fmt.Errorf("version %q belongs to %q, not %q", version.VersionID, version.Path, normalizedPath)
	}
	content, err := s.buildFileVersionContentResponse(ctx, workspace, version)
	if err != nil {
		return resolvedDiffOperand{}, err
	}
	label := "version:" + version.VersionID
	if strings.TrimSpace(operand.VersionID) == "" && strings.TrimSpace(operand.FileID) != "" && operand.Ordinal > 0 {
		label = fmt.Sprintf("%s@%d", operand.FileID, operand.Ordinal)
	}
	return resolvedDiffOperand{
		label:   label,
		content: content.Content,
		binary:  content.Binary,
	}, nil
}

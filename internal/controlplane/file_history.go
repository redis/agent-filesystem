package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

type FileHistoryRequest struct {
	Path        string `json:"path"`
	NewestFirst bool   `json:"-"`
	Limit       int    `json:"limit,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
}

type FileHistoryLineage struct {
	FileID      string        `json:"file_id"`
	State       string        `json:"state"`
	CurrentPath string        `json:"current_path"`
	Versions    []FileVersion `json:"versions"`
}

type FileHistoryResponse struct {
	WorkspaceID string               `json:"workspace_id"`
	Path        string               `json:"path"`
	Order       string               `json:"order"`
	Lineages    []FileHistoryLineage `json:"lineages"`
	NextCursor  string               `json:"next_cursor,omitempty"`
}

type FileVersionContentResponse struct {
	WorkspaceID string `json:"workspace_id"`
	FileID      string `json:"file_id"`
	VersionID   string `json:"version_id"`
	Ordinal     int64  `json:"ordinal"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Source      string `json:"source,omitempty"`
	Content     string `json:"content,omitempty"`
	Target      string `json:"target,omitempty"`
	Binary      bool   `json:"binary,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Language    string `json:"language,omitempty"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"created_at"`
}

func (s *Service) GetFileHistory(ctx context.Context, workspace, rawPath string, newestFirst bool) (FileHistoryResponse, error) {
	return s.GetFileHistoryPage(ctx, workspace, FileHistoryRequest{
		Path:        rawPath,
		NewestFirst: newestFirst,
	})
}

func (s *Service) GetFileHistoryPage(ctx context.Context, workspace string, req FileHistoryRequest) (FileHistoryResponse, error) {
	return s.getFileHistoryPage(ctx, workspace, req)
}

func (s *Service) GetFileVersionContent(ctx context.Context, workspace, versionID string) (FileVersionContentResponse, error) {
	return s.getFileVersionContent(ctx, workspace, versionID)
}

func (s *Service) GetFileVersionContentAtOrdinal(ctx context.Context, workspace, fileID string, ordinal int64) (FileVersionContentResponse, error) {
	return s.getFileVersionContentAtOrdinal(ctx, workspace, fileID, ordinal)
}

type fileHistoryCursor struct {
	FileID  string `json:"file_id"`
	Ordinal int64  `json:"ordinal"`
}

func (s *Service) getFileHistory(ctx context.Context, workspace, rawPath string, newestFirst bool) (FileHistoryResponse, error) {
	return s.getFileHistoryPage(ctx, workspace, FileHistoryRequest{
		Path:        rawPath,
		NewestFirst: newestFirst,
	})
}

func (s *Service) getFileHistoryPage(ctx context.Context, workspace string, req FileHistoryRequest) (FileHistoryResponse, error) {
	normalizedPath, err := normalizeVersionedPath(req.Path)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	if req.Limit < 0 {
		return FileHistoryResponse{}, fmt.Errorf("invalid limit %d", req.Limit)
	}
	versionIDs, err := s.store.ListPathHistoryVersionIDs(ctx, workspace, normalizedPath)
	if err != nil {
		return FileHistoryResponse{}, err
	}
	if len(versionIDs) == 0 {
		return FileHistoryResponse{}, os.ErrNotExist
	}
	if req.NewestFirst {
		slices.Reverse(versionIDs)
	}

	orderedVersions := make([]FileVersion, 0, len(versionIDs))
	for _, versionID := range versionIDs {
		version, err := s.store.GetFileVersion(ctx, workspace, versionID)
		if err != nil {
			return FileHistoryResponse{}, err
		}
		orderedVersions = append(orderedVersions, version)
	}

	start := 0
	if cursor := strings.TrimSpace(req.Cursor); cursor != "" {
		marker, err := decodeFileHistoryCursor(cursor)
		if err != nil {
			return FileHistoryResponse{}, err
		}
		matched := false
		for index, version := range orderedVersions {
			if version.FileID == marker.FileID && version.Ordinal == marker.Ordinal {
				start = index + 1
				matched = true
				break
			}
		}
		if !matched {
			return FileHistoryResponse{}, fmt.Errorf("invalid cursor %q", cursor)
		}
	}

	selected := orderedVersions[start:]
	nextCursor := ""
	if req.Limit > 0 && len(selected) > req.Limit {
		selected = selected[:req.Limit]
		nextCursor, err = encodeFileHistoryCursor(selected[len(selected)-1].FileID, selected[len(selected)-1].Ordinal)
		if err != nil {
			return FileHistoryResponse{}, err
		}
	}

	fileOrder := make([]string, 0, len(selected))
	lineageIndex := make(map[string]int, len(selected))
	lineages := make([]FileHistoryLineage, 0, len(selected))
	for _, version := range selected {
		index, ok := lineageIndex[version.FileID]
		if !ok {
			lineage, err := s.store.GetFileLineage(ctx, workspace, version.FileID)
			if err != nil {
				return FileHistoryResponse{}, err
			}
			fileOrder = append(fileOrder, version.FileID)
			lineageIndex[version.FileID] = len(lineages)
			lineages = append(lineages, FileHistoryLineage{
				FileID:      lineage.FileID,
				State:       lineage.State,
				CurrentPath: lineage.CurrentPath,
				Versions:    []FileVersion{version},
			})
			continue
		}
		lineages[index].Versions = append(lineages[index].Versions, version)
	}

	if len(fileOrder) == 0 && start > len(orderedVersions) {
		return FileHistoryResponse{}, fmt.Errorf("invalid cursor %q", req.Cursor)
	}

	return FileHistoryResponse{
		WorkspaceID: workspace,
		Path:        normalizedPath,
		Order:       ternaryString(req.NewestFirst, "desc", "asc"),
		Lineages:    lineages,
		NextCursor:  nextCursor,
	}, nil
}

func encodeFileHistoryCursor(fileID string, ordinal int64) (string, error) {
	payload, err := json.Marshal(fileHistoryCursor{
		FileID:  strings.TrimSpace(fileID),
		Ordinal: ordinal,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeFileHistoryCursor(raw string) (fileHistoryCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return fileHistoryCursor{}, fmt.Errorf("invalid cursor %q", raw)
	}
	var cursor fileHistoryCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return fileHistoryCursor{}, fmt.Errorf("invalid cursor %q", raw)
	}
	if strings.TrimSpace(cursor.FileID) == "" || cursor.Ordinal <= 0 {
		return fileHistoryCursor{}, fmt.Errorf("invalid cursor %q", raw)
	}
	return fileHistoryCursor{
		FileID:  strings.TrimSpace(cursor.FileID),
		Ordinal: cursor.Ordinal,
	}, nil
}

func fileHistoryCursorForVersion(version FileVersion) (string, error) {
	return encodeFileHistoryCursor(version.FileID, version.Ordinal)
}

func (s *Service) getFileVersionContent(ctx context.Context, workspace, versionID string) (FileVersionContentResponse, error) {
	version, err := s.store.GetFileVersion(ctx, workspace, versionID)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return s.buildFileVersionContentResponse(ctx, workspace, version)
}

func (s *Service) getFileVersionContentAtOrdinal(ctx context.Context, workspace, fileID string, ordinal int64) (FileVersionContentResponse, error) {
	version, err := s.store.GetFileVersionAtOrdinal(ctx, workspace, fileID, ordinal)
	if err != nil {
		return FileVersionContentResponse{}, err
	}
	return s.buildFileVersionContentResponse(ctx, workspace, version)
}

func (s *Service) buildFileVersionContentResponse(ctx context.Context, workspace string, version FileVersion) (FileVersionContentResponse, error) {
	response := FileVersionContentResponse{
		WorkspaceID: workspace,
		FileID:      version.FileID,
		VersionID:   version.VersionID,
		Ordinal:     version.Ordinal,
		Path:        version.Path,
		Kind:        version.Kind,
		Source:      version.Source,
		Size:        version.SizeBytes,
		CreatedAt:   version.CreatedAt.UTC().Format(time.RFC3339),
	}
	switch version.Kind {
	case FileVersionKindSymlink:
		response.Target = version.Target
		response.Content = version.Target
		return response, nil
	case FileVersionKindTombstone:
		return response, nil
	case FileVersionKindFile:
		if version.BlobID == "" {
			response.Binary = true
			response.ContentType = "application/octet-stream"
			return response, nil
		}
		data, err := s.store.GetBlob(ctx, workspace, version.BlobID)
		if err != nil {
			return FileVersionContentResponse{}, err
		}
		if isBinary(data) {
			response.Binary = true
			return response, nil
		}
		response.Content = string(data)
		response.Encoding = "utf-8"
		response.ContentType = contentType(version.Path, "file")
		response.Language = language(version.Path)
		return response, nil
	default:
		return FileVersionContentResponse{}, fmt.Errorf("unsupported version kind %q", version.Kind)
	}
}

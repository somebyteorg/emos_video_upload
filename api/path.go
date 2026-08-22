package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) resolvePath(raw string) (string, error) {
	root, err := filepath.EvalSymlinks(s.cfg.VideoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve VIDEO_ROOT: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return root, nil
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", err
	}
	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	realCandidate, err = filepath.Abs(realCandidate)
	if err != nil {
		return "", err
	}
	if !isWithin(root, realCandidate) {
		return "", errors.New("path is outside VIDEO_ROOT")
	}
	return realCandidate, nil
}

func (s *Server) resolveVideoFile(raw string) (string, os.FileInfo, error) {
	path, err := s.resolvePath(raw)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, errors.New("video path is not a regular file")
	}
	if !isVideoExtension(info.Name()) {
		return "", nil, errors.New("file extension is not a supported video format")
	}
	if info.Size() <= 0 {
		return "", nil, errors.New("video file is empty")
	}
	return path, info, nil
}

// Go's MIME table covers common formats; keep a small fallback for video
// containers that are not registered on every Linux installation.
func isVideoExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if mediaType := mime.TypeByExtension(ext); strings.HasPrefix(mediaType, "video/") {
		return true
	}
	switch ext {
	case ".mkv", ".mp4", ".m4v", ".mov", ".avi", ".wmv", ".flv", ".f4v", ".ts", ".mts", ".m2ts", ".mpg", ".mpeg", ".vob", ".3gp", ".3g2", ".ogv", ".asf", ".rm", ".rmvb", ".mxf", ".divx", ".webm":
		return true
	default:
		return false
	}
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sourceFileID(path string, info os.FileInfo) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())))
	return hex.EncodeToString(hash[:8])
}

func uploadCacheKey(path string, size, mtime int64, itemType string, itemID int64, season, episode int, storage, taskID string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"%s|%d|%d|%s|%d|%d|%d|%s|%s",
		path, size, mtime, itemType, itemID, season, episode, storage, taskID,
	)))
	return hex.EncodeToString(hash[:])
}

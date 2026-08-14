package safepath

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidFolderID = errors.New("无效的任务名称 / 文件夹 ID")
	ErrInvalidFilename = errors.New("无效的文件名")
)

// folder IDs: Pixiv artwork ids, task_<ms>, or simple alnum names — no path separators.
var folderIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidateFolderID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return ErrInvalidFolderID
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return ErrInvalidFolderID
	}
	for _, r := range id {
		if r < 32 || !unicode.IsPrint(r) {
			return ErrInvalidFolderID
		}
	}
	if !folderIDRe.MatchString(id) {
		return ErrInvalidFolderID
	}
	return nil
}

// SanitizeFilename returns a basename safe for joining under a folder directory.
func SanitizeFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidFilename
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "." || name == ".." || name == "" {
		return "", ErrInvalidFilename
	}
	if strings.ContainsAny(name, `/\`) {
		return "", ErrInvalidFilename
	}
	for _, r := range name {
		if r < 32 {
			return "", ErrInvalidFilename
		}
	}
	return name, nil
}

// FilenameFromURL extracts and sanitizes the last path segment of a URL.
func FilenameFromURL(rawURL string) string {
	u := rawURL
	if i := strings.Index(u, "?"); i >= 0 {
		u = u[:i]
	}
	parts := strings.Split(u, "/")
	name := parts[len(parts)-1]
	if cleaned, err := SanitizeFilename(name); err == nil {
		return cleaned
	}
	return fmt.Sprintf("image_%d.jpg", time.Now().UnixNano())
}

// PathAllowedAsFolder checks a DB-stored absolute path is safe to use for folderID.
// Prefer paths under downloadRoot; legacy paths outside root are allowed only when
// Base(path) matches folderID and the cleaned path has no ".." segments.
func PathAllowedAsFolder(downloadRoot, folderID, storedPath string) (abs string, ok bool) {
	if ValidateFolderID(folderID) != nil {
		return "", false
	}
	p := strings.TrimSpace(storedPath)
	if p == "" {
		return "", false
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(filepath.Base(abs), folderID) {
		return "", false
	}
	for _, part := range strings.Split(abs, string(filepath.Separator)) {
		if part == ".." {
			return "", false
		}
	}
	// Prefer under DOWNLOAD_DIR; legacy DB paths outside root still need base match.
	if root, err := filepath.Abs(downloadRoot); err == nil && pathUnder(root, abs) {
		return abs, true
	}
	return abs, true
}

func pathUnder(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." {
		return false
	}
	sep := string(filepath.Separator)
	return !strings.HasPrefix(rel, ".."+sep)
}

package server

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Xephyrus/download-widget/internal/safepath"
)

var (
	errInvalidFolderID = errors.New("无效ID")
	errPathForbidden   = errors.New("访问被拒绝")
)

func (s *Server) resolveFolderPath(folderID string) (string, error) {
	folderID = strings.TrimSpace(folderID)
	if err := safepath.ValidateFolderID(folderID); err != nil {
		return "", errInvalidFolderID
	}

	absRoot, err := filepath.Abs(s.downloadRoot)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(absRoot, folderID)
	if !pathUnderRoot(absRoot, joined) {
		return "", errPathForbidden
	}
	if st, err := os.Stat(joined); err == nil && st.IsDir() {
		return joined, nil
	}

	// DOWNLOAD_DIR may differ from where files were indexed; fall back to DB path.
	if folder, err := s.store.GetFolder(folderID); err == nil && folder != nil {
		if absDB, ok := safepath.PathAllowedAsFolder(absRoot, folderID, folder.Path); ok {
			if st, err := os.Stat(absDB); err == nil && st.IsDir() {
				return absDB, nil
			}
		}
	}

	return joined, nil
}

// parseFolderFilePath extracts folderID + filename from "/prefix/{id}/{file}".
func parseFolderFilePath(path, prefix string) (folderID, filename string, ok bool) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	folderID, err1 := url.PathUnescape(parts[0])
	filename, err2 := url.PathUnescape(parts[1])
	if err1 != nil || err2 != nil {
		return "", "", false
	}
	if _, err := safepath.SanitizeFilename(filename); err != nil {
		return "", "", false
	}
	if err := safepath.ValidateFolderID(folderID); err != nil {
		return "", "", false
	}
	return folderID, filename, true
}

func pathUnderRoot(root, path string) bool {
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

func (s *Server) deleteFolder(folderID string) error {
	absPath, err := s.resolveFolderPath(folderID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err == nil {
		if err := os.RemoveAll(absPath); err != nil {
			return fmt.Errorf("删除目录失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := s.store.DeleteFolder(folderID); err != nil {
		return fmt.Errorf("删除数据库记录失败: %w", err)
	}
	s.clearThumbCache(folderID)
	return nil
}

// clearFolderContents deletes all files under the folder on disk and in DB,
// then recreates an empty directory and keeps/refreshes the folder row.
func (s *Server) clearFolderContents(folderID string) error {
	absPath, err := s.resolveFolderPath(folderID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); err == nil {
		if err := os.RemoveAll(absPath); err != nil {
			return fmt.Errorf("清空目录失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return fmt.Errorf("重建目录失败: %w", err)
	}
	if err := s.store.ClearFolderFiles(folderID); err != nil {
		return fmt.Errorf("同步数据库失败: %w", err)
	}
	if err := s.store.UpsertFolder(folderID, folderID, absPath); err != nil {
		return fmt.Errorf("更新文件夹记录失败: %w", err)
	}
	s.clearThumbCache(folderID)
	return nil
}

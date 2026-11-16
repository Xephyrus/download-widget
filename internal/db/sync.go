package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// SyncFromDisk scans downloadDir and rebuilds folder/file metadata from disk.
// Orphan DB folders not present under downloadDir are removed — but only when the
// scan actually finds folders. An empty/wrong DOWNLOAD_DIR must not wipe the index.
func (s *Store) SyncFromDisk(downloadDir string) (folders int, files int, err error) {
	if s.root == "" {
		s.root = filepath.Clean(downloadDir)
	}
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, fmt.Errorf("下载目录不存在: %s", downloadDir)
		}
		return 0, 0, err
	}

	type folderScan struct {
		id   string
		path string
		info os.FileInfo
	}
	scanned := make([]folderScan, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folderID := strings.TrimSpace(entry.Name())
		if folderID == "" || strings.HasPrefix(folderID, ".") {
			continue
		}
		folderPath := filepath.Join(downloadDir, entry.Name())
		info, statErr := os.Stat(folderPath)
		if statErr != nil || !info.IsDir() {
			continue
		}
		scanned = append(scanned, folderScan{id: folderID, path: folderPath, info: info})
	}

	existing := 0
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFolders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, _ []byte) error {
			existing++
			return nil
		})
	})

	// Wrong DOWNLOAD_DIR (empty project ./downloads) used to prune every record.
	if len(scanned) == 0 {
		if existing > 0 {
			return 0, 0, fmt.Errorf(
				"跳过同步：目录 %s 下没有作品文件夹，但数据库已有 %d 条记录（请检查 DOWNLOAD_DIR）",
				downloadDir, existing,
			)
		}
		return 0, 0, nil
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		seen := make(map[string]struct{}, len(scanned))
		fb := tx.Bucket(bucketFolders)

		_ = tx.DeleteBucket(bucketFiles)
		filesRoot, err := tx.CreateBucket(bucketFiles)
		if err != nil {
			return err
		}

		for _, item := range scanned {
			folderID := item.id
			folderPath := item.path
			created, modified := diskTimes(item.info)
			rec := folderRec{
				Path:       s.compactFolderPath(folderID, folderPath),
				CreatedAt:  created.Unix(),
				ModifiedAt: modified.Unix(),
			}
			// Preserve custom metadata (name / tags) across disk sync.
			if raw := fb.Get([]byte(folderID)); raw != nil {
				if prev, ok := decodeFolder(raw); ok {
					rec.Name = prev.Name
					rec.Tags = prev.Tags
				}
			}
			if err := putFolder(fb, folderID, rec); err != nil {
				return err
			}
			seen[folderID] = struct{}{}
			folders++

			fileEntries, err := os.ReadDir(folderPath)
			if err != nil {
				continue
			}
			for _, fe := range fileEntries {
				if fe.IsDir() {
					continue
				}
				name := fe.Name()
				lower := strings.ToLower(name)
				if !(strings.HasSuffix(lower, ".jpg") ||
					strings.HasSuffix(lower, ".jpeg") ||
					strings.HasSuffix(lower, ".png") ||
					strings.HasSuffix(lower, ".gif") ||
					strings.HasSuffix(lower, ".webp") ||
					strings.HasSuffix(lower, ".bmp") ||
					strings.HasSuffix(lower, ".tiff")) {
					continue
				}
				fi, err := fe.Info()
				if err != nil {
					continue
				}
				if err := filesRoot.Put(fileKey(folderID, name), encodeFile(fi.Size(), 0)); err != nil {
					return err
				}
				files++
			}

			if err := s.refreshFolderStatsTx(tx, folderID, false); err != nil {
				return err
			}
			raw := fb.Get([]byte(folderID))
			if updated, ok := decodeFolder(raw); ok {
				updated.CreatedAt = created.Unix()
				updated.ModifiedAt = modified.Unix()
				updated.Path = s.compactFolderPath(folderID, folderPath)
				// Name/Tags already preserved above; keep them through stats refresh.
				_ = putFolder(fb, folderID, updated)
			}
		}

		var orphans []string
		_ = fb.ForEach(func(k, _ []byte) error {
			id := string(k)
			if _, ok := seen[id]; !ok {
				orphans = append(orphans, id)
			}
			return nil
		})
		for _, id := range orphans {
			_ = fb.Delete([]byte(id))
		}
		return nil
	})
	return folders, files, err
}

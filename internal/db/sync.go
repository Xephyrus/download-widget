package db

import (
	"os"
	"path/filepath"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// SyncFromDisk scans downloadDir and rebuilds folder/file metadata from disk.
// Orphan DB folders not present under downloadDir are removed.
func (s *Store) SyncFromDisk(downloadDir string) (folders int, files int, err error) {
	if s.root == "" {
		s.root = filepath.Clean(downloadDir)
	}
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		seen := make(map[string]struct{})
		fb := tx.Bucket(bucketFolders)

		_ = tx.DeleteBucket(bucketFiles)
		filesRoot, err := tx.CreateBucket(bucketFiles)
		if err != nil {
			return err
		}

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
			if statErr != nil {
				continue
			}

			created, modified := diskTimes(info)
			rec := folderRec{
				Path:       s.compactFolderPath(folderID, folderPath),
				CreatedAt:  created.Unix(),
				ModifiedAt: modified.Unix(),
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

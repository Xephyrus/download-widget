package db

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketFolders = []byte("folders")
	bucketFiles   = []byte("files") // flat keys: folderID + "/" + filename
)

type Store struct {
	db   *bolt.DB
	path string
	root string // DOWNLOAD_DIR; empty folder Path means Join(root, id)
}

type Folder struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	FileCount  int       `json:"fileCount"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"createdAt"`
	ModifiedAt time.Time `json:"modifiedAt"`
	Path       string    `json:"path"`
	Tags       []string  `json:"tags,omitempty"`
	Files      []string  `json:"files,omitempty"`
}

// Compact folder record. Key is folder id.
type folderRec struct {
	Name       string   `json:"n,omitempty"`
	Path       string   `json:"p,omitempty"`
	FileCount  int      `json:"f,omitempty"`
	Size       int64    `json:"s,omitempty"`
	CreatedAt  int64    `json:"c,omitempty"` // unix seconds
	ModifiedAt int64    `json:"m,omitempty"`
	Tags       []string `json:"t,omitempty"`
}

type ListFilter struct {
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
	Search       string
	Tag          string // exact tag match (case-insensitive)
	ExactMatch   bool
	MinFileCount *int
	MaxFileCount *int
	StartDate    *time.Time
	EndDate      *time.Time
}

type ListResult struct {
	Folders    []Folder
	Total      int
	TotalFiles int
	TotalSize  int64
}

func Open(dbPath string) (*Store, bool, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, false, err
	}
	migrated, err := retireLegacySQLite(dbPath)
	if err != nil {
		return nil, false, err
	}

	bdb, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, false, err
	}
	s := &Store{db: bdb, path: dbPath}
	if err := s.migrate(); err != nil {
		_ = bdb.Close()
		return nil, false, err
	}
	return s, migrated, nil
}

func (s *Store) SetRoot(downloadDir string) {
	if s == nil {
		return
	}
	s.root = filepath.Clean(downloadDir)
}

func retireLegacySQLite(dbPath string) (migrated bool, err error) {
	f, err := os.Open(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var hdr [16]byte
	n, _ := f.Read(hdr[:])
	_ = f.Close()
	if n < 16 || string(hdr[:15]) != "SQLite format 3" {
		return false, nil
	}
	bak := dbPath + ".sqlite.bak"
	_ = os.Remove(bak)
	if err := os.Rename(dbPath, bak); err != nil {
		return false, fmt.Errorf("迁移旧 SQLite 库失败: %w", err)
	}
	return true, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketFolders); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketFiles); err != nil {
			return err
		}
		return nil
	})
}

// Compact rewrites the DB into a new file (bucket copy), reclaiming freelist waste.
func (s *Store) Compact() error {
	if s.db == nil || s.path == "" {
		return nil
	}
	tmp := s.path + ".compact"
	_ = os.Remove(tmp)
	dst, err := bolt.Open(tmp, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return err
	}
	err = s.db.View(func(stx *bolt.Tx) error {
		return stx.ForEach(func(name []byte, sb *bolt.Bucket) error {
			return dst.Update(func(dtx *bolt.Tx) error {
				dbkt, err := dtx.CreateBucketIfNotExists(name)
				if err != nil {
					return err
				}
				return copyBucket(sb, dbkt)
			})
		})
	})
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := s.db.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.db = nil
	bak := s.path + ".old"
	_ = os.Remove(bak)
	if err := os.Rename(s.path, bak); err != nil {
		bdb, oerr := bolt.Open(s.path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
		if oerr == nil {
			s.db = bdb
		}
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Rename(bak, s.path)
		bdb, oerr := bolt.Open(s.path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
		if oerr == nil {
			s.db = bdb
		}
		return err
	}
	_ = os.Remove(bak)
	bdb, err := bolt.Open(s.path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return err
	}
	s.db = bdb
	return nil
}

func copyBucket(src, dst *bolt.Bucket) error {
	return src.ForEach(func(k, v []byte) error {
		if v == nil {
			// nested bucket
			sb := src.Bucket(k)
			if sb == nil {
				return nil
			}
			db, err := dst.CreateBucketIfNotExists(k)
			if err != nil {
				return err
			}
			return copyBucket(sb, db)
		}
		return dst.Put(k, v)
	})
}

func (s *Store) UpsertFolder(id, name, path string) error {
	now := time.Now().Unix()
	createdAt, modifiedAt := now, now
	if info, err := os.Stat(path); err == nil {
		c, m := diskTimes(info)
		createdAt, modifiedAt = c.Unix(), m.Unix()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFolders)
		rec := folderRec{Path: s.compactFolderPath(id, path), CreatedAt: createdAt, ModifiedAt: modifiedAt}
		if name != "" && name != id {
			rec.Name = name
		}
		if raw := b.Get([]byte(id)); raw != nil {
			if prev, ok := decodeFolder(raw); ok {
				rec.FileCount = prev.FileCount
				rec.Size = prev.Size
				rec.Tags = prev.Tags
				if rec.Name == "" {
					rec.Name = prev.Name
				}
			}
		}
		return putFolder(b, id, rec)
	})
}

// SetFolderTags replaces tags for a folder. Pass nil/empty to clear.
func (s *Store) SetFolderTags(id string, tags []string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("无效的文件夹 ID")
	}
	normalized := NormalizeTags(tags)
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFolders)
		raw := b.Get([]byte(id))
		if raw == nil {
			return fmt.Errorf("文件夹不存在")
		}
		rec, ok := decodeFolder(raw)
		if !ok {
			return fmt.Errorf("文件夹记录损坏")
		}
		rec.Tags = normalized
		return putFolder(b, id, rec)
	})
}

// ListAllTags returns unique tags across all folders, sorted case-insensitively.
func (s *Store) ListAllTags() ([]string, error) {
	seen := map[string]string{}
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFolders)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			rec, ok := decodeFolder(v)
			if !ok {
				return nil
			}
			for _, tag := range rec.Tags {
				key := strings.ToLower(tag)
				if _, ok := seen[key]; !ok {
					seen[key] = tag
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for _, tag := range seen {
		out = append(out, tag)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out, nil
}

// NormalizeTags trims, dedupes (case-insensitive), and caps length/count.
func NormalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		t = strings.Trim(t, "#　")
		if t == "" {
			continue
		}
		runes := []rune(t)
		if len(runes) > 32 {
			t = string(runes[:32])
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func ParseTagsCSV(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
	})
	return NormalizeTags(parts)
}

func (s *Store) FileExists(folderID, baseName string) (string, bool, error) {
	want := strings.ToLower(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
	if want == "" {
		return "", false, nil
	}
	prefix := folderID + "/"
	var found string
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketFiles).Cursor()
		baseLower := strings.ToLower(baseName)
		for k, _ := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
			name := string(k[len(prefix):])
			lower := strings.ToLower(name)
			if lower == baseLower || strings.TrimSuffix(lower, filepath.Ext(lower)) == want {
				found = name
				return errStop
			}
		}
		return nil
	})
	if err == errStop {
		err = nil
	}
	if err != nil {
		return "", false, err
	}
	if found == "" {
		return "", false, nil
	}
	return found, true, nil
}

var errStop = fmt.Errorf("stop")

func (s *Store) AddFile(folderID, filename string, size int64, path string) error {
	created := time.Now().Unix()
	if info, err := os.Stat(path); err == nil {
		c, _ := diskTimes(info)
		created = c.Unix()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketFiles).Put(fileKey(folderID, filename), encodeFile(size, created)); err != nil {
			return err
		}
		return s.refreshFolderStatsTx(tx, folderID, true)
	})
}

func (s *Store) refreshFolderStatsTx(tx *bolt.Tx, folderID string, touchModified bool) error {
	prefix := folderID + "/"
	var count int
	var total int64
	c := tx.Bucket(bucketFiles).Cursor()
	for k, v := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
		size, _, ok := decodeFile(v)
		if !ok {
			continue
		}
		count++
		total += size
	}
	b := tx.Bucket(bucketFolders)
	raw := b.Get([]byte(folderID))
	if raw == nil {
		return nil
	}
	rec, ok := decodeFolder(raw)
	if !ok {
		return nil
	}
	rec.FileCount = count
	rec.Size = total
	if touchModified {
		abs := s.resolveRecPath(folderID, rec)
		if info, err := os.Stat(abs); err == nil {
			_, m := diskTimes(info)
			rec.ModifiedAt = m.Unix()
		} else {
			rec.ModifiedAt = time.Now().Unix()
		}
	}
	return putFolder(b, folderID, rec)
}

func (s *Store) ListFolders(f ListFilter) (*ListResult, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}

	all := make([]Folder, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketFolders).ForEach(func(k, v []byte) error {
			rec, ok := decodeFolder(v)
			if !ok {
				return nil
			}
			item := folderFromRec(string(k), rec)
			item.Path = s.resolveRecPath(item.ID, rec)
			if !matchFolder(item, f) {
				return nil
			}
			all = append(all, item)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	sortFolders(all, f.SortBy, f.SortOrder)

	var totalFiles int
	var totalSize int64
	for _, item := range all {
		totalFiles += item.FileCount
		totalSize += item.Size
	}

	total := len(all)
	start := (f.Page - 1) * f.PageSize
	if start > total {
		start = total
	}
	end := start + f.PageSize
	if end > total {
		end = total
	}

	return &ListResult{
		Folders:    all[start:end],
		Total:      total,
		TotalFiles: totalFiles,
		TotalSize:  totalSize,
	}, nil
}

func matchFolder(item Folder, f ListFilter) bool {
	if f.Tag != "" && !folderHasTag(item.Tags, f.Tag) {
		return false
	}
	if f.Search != "" {
		q := strings.ToLower(f.Search)
		if f.ExactMatch {
			if item.Name != f.Search && !folderHasTag(item.Tags, f.Search) {
				return false
			}
		} else {
			nameHit := strings.Contains(strings.ToLower(item.Name), q)
			tagHit := false
			for _, t := range item.Tags {
				if strings.Contains(strings.ToLower(t), q) {
					tagHit = true
					break
				}
			}
			if !nameHit && !tagHit {
				return false
			}
		}
	}
	if f.MinFileCount != nil && item.FileCount < *f.MinFileCount {
		return false
	}
	if f.MaxFileCount != nil && item.FileCount > *f.MaxFileCount {
		return false
	}
	if f.StartDate != nil && item.CreatedAt.Before(f.StartDate.UTC()) {
		return false
	}
	if f.EndDate != nil && item.CreatedAt.After(f.EndDate.UTC()) {
		return false
	}
	return true
}

func folderHasTag(tags []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, t := range tags {
		if strings.ToLower(t) == want {
			return true
		}
	}
	return false
}

func sortFolders(all []Folder, sortBy, sortOrder string) {
	desc := !strings.EqualFold(sortOrder, "asc")
	less := func(i, j int) bool {
		var cmp int
		switch sortBy {
		case "name":
			cmp = strings.Compare(all[i].Name, all[j].Name)
		case "modifiedAt":
			cmp = compareTime(all[i].ModifiedAt, all[j].ModifiedAt)
		case "fileCount":
			cmp = all[i].FileCount - all[j].FileCount
		case "size":
			switch {
			case all[i].Size < all[j].Size:
				cmp = -1
			case all[i].Size > all[j].Size:
				cmp = 1
			}
		default:
			cmp = compareTime(all[i].CreatedAt, all[j].CreatedAt)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	}
	sort.SliceStable(all, less)
}

func compareTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func (s *Store) GetFolder(id string) (*Folder, error) {
	var out *Folder
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucketFolders).Get([]byte(id))
		if raw == nil {
			return nil
		}
		rec, ok := decodeFolder(raw)
		if !ok {
			return nil
		}
		item := folderFromRec(id, rec)
		item.Path = s.resolveRecPath(id, rec)
		out = &item
		return nil
	})
	return out, err
}

func (s *Store) ListImageFiles(folderID string) ([]string, error) {
	prefix := folderID + "/"
	files := make([]string, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketFiles).Cursor()
		for k, _ := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
			name := string(k[len(prefix):])
			lower := strings.ToLower(name)
			switch {
			case strings.HasSuffix(lower, ".jpg"),
				strings.HasSuffix(lower, ".jpeg"),
				strings.HasSuffix(lower, ".png"),
				strings.HasSuffix(lower, ".gif"),
				strings.HasSuffix(lower, ".webp"),
				strings.HasSuffix(lower, ".bmp"),
				strings.HasSuffix(lower, ".tiff"):
				files = append(files, name)
			}
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func (s *Store) DeleteFolder(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketFolders).Delete([]byte(id)); err != nil {
			return err
		}
		return deleteFolderFiles(tx, id)
	})
}

func (s *Store) ClearFolderFiles(folderID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := deleteFolderFiles(tx, folderID); err != nil {
			return err
		}
		b := tx.Bucket(bucketFolders)
		raw := b.Get([]byte(folderID))
		if raw == nil {
			return nil
		}
		rec, ok := decodeFolder(raw)
		if !ok {
			return nil
		}
		rec.FileCount = 0
		rec.Size = 0
		if info, err := os.Stat(s.resolveRecPath(folderID, rec)); err == nil {
			c, m := diskTimes(info)
			rec.CreatedAt = c.Unix()
			rec.ModifiedAt = m.Unix()
		} else {
			rec.ModifiedAt = time.Now().Unix()
		}
		return putFolder(b, folderID, rec)
	})
}

func deleteFolderFiles(tx *bolt.Tx, folderID string) error {
	prefix := folderID + "/"
	b := tx.Bucket(bucketFiles)
	var keys [][]byte
	c := b.Cursor()
	for k, _ := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
		keys = append(keys, append([]byte(nil), k...))
	}
	for _, k := range keys {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func fileKey(folderID, filename string) []byte {
	return []byte(folderID + "/" + filename)
}

func encodeFile(size, createdUnix int64) []byte {
	// Prefer compact 8-byte size-only records; created time lives on disk.
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(size))
	_ = createdUnix
	return buf
}

func decodeFile(v []byte) (size, createdUnix int64, ok bool) {
	switch len(v) {
	case 8:
		return int64(binary.LittleEndian.Uint64(v)), 0, true
	case 16:
		return int64(binary.LittleEndian.Uint64(v[0:8])), int64(binary.LittleEndian.Uint64(v[8:16])), true
	}
	// legacy JSON
	var legacy struct {
		Size      int64  `json:"size"`
		S         int64  `json:"s"`
		CreatedAt string `json:"createdAt"`
		C         int64  `json:"c"`
	}
	if json.Unmarshal(v, &legacy) != nil {
		return 0, 0, false
	}
	size = legacy.Size
	if size == 0 {
		size = legacy.S
	}
	createdUnix = legacy.C
	if createdUnix == 0 && legacy.CreatedAt != "" {
		if t := parseTime(legacy.CreatedAt); !t.IsZero() {
			createdUnix = t.Unix()
		}
	}
	return size, createdUnix, true
}

func putFolder(b *bolt.Bucket, id string, rec folderRec) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return b.Put([]byte(id), raw)
}

func decodeFolder(raw []byte) (folderRec, bool) {
	var rec folderRec
	if err := json.Unmarshal(raw, &rec); err != nil {
		return folderRec{}, false
	}
	// legacy verbose JSON
	if rec.Path == "" && rec.CreatedAt == 0 && rec.ModifiedAt == 0 {
		var legacy struct {
			Name       string `json:"name"`
			Path       string `json:"path"`
			FileCount  int    `json:"fileCount"`
			Size       int64  `json:"size"`
			CreatedAt  string `json:"createdAt"`
			ModifiedAt string `json:"modifiedAt"`
			N          string `json:"n"`
			P          string `json:"p"`
			F          int    `json:"f"`
			S          int64  `json:"s"`
			C          int64  `json:"c"`
			M          int64  `json:"m"`
		}
		if json.Unmarshal(raw, &legacy) != nil {
			return folderRec{}, false
		}
		rec.Name = legacy.N
		if rec.Name == "" {
			rec.Name = legacy.Name
		}
		rec.Path = legacy.P
		if rec.Path == "" {
			rec.Path = legacy.Path
		}
		rec.FileCount = legacy.F
		if rec.FileCount == 0 {
			rec.FileCount = legacy.FileCount
		}
		rec.Size = legacy.S
		if rec.Size == 0 {
			rec.Size = legacy.Size
		}
		rec.CreatedAt = legacy.C
		rec.ModifiedAt = legacy.M
		if rec.CreatedAt == 0 && legacy.CreatedAt != "" {
			if t := parseTime(legacy.CreatedAt); !t.IsZero() {
				rec.CreatedAt = t.Unix()
			}
		}
		if rec.ModifiedAt == 0 && legacy.ModifiedAt != "" {
			if t := parseTime(legacy.ModifiedAt); !t.IsZero() {
				rec.ModifiedAt = t.Unix()
			}
		}
	}
	return rec, true
}

func folderFromRec(id string, rec folderRec) Folder {
	name := rec.Name
	if name == "" {
		name = id
	}
	tags := append([]string(nil), rec.Tags...)
	return Folder{
		ID:         id,
		Name:       name,
		Path:       rec.Path, // may be relative/empty; callers use ResolveFolderPath via store helpers
		FileCount:  rec.FileCount,
		Size:       rec.Size,
		CreatedAt:  unixTime(rec.CreatedAt),
		ModifiedAt: unixTime(rec.ModifiedAt),
		Tags:       tags,
	}
}

func (s *Store) resolveRecPath(id string, rec folderRec) string {
	if s == nil {
		if rec.Path != "" {
			return rec.Path
		}
		return id
	}
	if rec.Path == "" {
		if s.root != "" {
			return filepath.Join(s.root, id)
		}
		return id
	}
	if filepath.IsAbs(rec.Path) {
		return rec.Path
	}
	if s.root != "" {
		return filepath.Join(s.root, rec.Path)
	}
	return rec.Path
}

// compactFolderPath stores "" when path is the default DOWNLOAD_DIR/<id>.
func (s *Store) compactFolderPath(id, absPath string) string {
	absPath = filepath.Clean(absPath)
	if s == nil || s.root == "" {
		return absPath
	}
	def := filepath.Clean(filepath.Join(s.root, id))
	if strings.EqualFold(absPath, def) {
		return ""
	}
	if rel, err := filepath.Rel(s.root, absPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return absPath
}

func unixTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func parseTime(v string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	return time.Time{}
}

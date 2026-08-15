package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Xephyrus/download-widget/internal/config"
	"github.com/Xephyrus/download-widget/internal/db"
	"github.com/Xephyrus/download-widget/internal/safepath"
)

type ProgressEvent struct {
	Type       string `json:"type"`
	TaskID     string `json:"taskId"`
	Current    int    `json:"current,omitempty"`
	Total      int    `json:"total,omitempty"`
	Filename   string `json:"filename,omitempty"`
	URL        string `json:"url,omitempty"`
	Filepath   string `json:"filepath,omitempty"`
	Error      string `json:"error,omitempty"`
	SuccessCount int  `json:"successCount,omitempty"`
	FailCount    int  `json:"failCount,omitempty"`
	DownloadDir  string `json:"downloadDir,omitempty"`
}

type Notifier func(ProgressEvent)

type Service struct {
	cfg    config.Config
	store  *db.Store
	client *http.Client
}

func New(cfg config.Config, store *db.Store) *Service {
	proxyURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort),
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	return &Service{
		cfg:   cfg,
		store: store,
		client: &http.Client{
			Timeout:   time.Duration(cfg.TimeoutSec) * time.Second,
			Transport: transport,
		},
	}
}

func (s *Service) BatchDownload(ctx context.Context, taskID string, urls []string, cookie, taskName string, notify Notifier) {
	folderID := strings.TrimSpace(taskName)
	if folderID == "" {
		folderID = taskID
	}
	if err := safepath.ValidateFolderID(folderID); err != nil {
		notify(ProgressEvent{Type: "task_error", TaskID: taskID, Error: err.Error()})
		return
	}
	taskDir := filepath.Join(s.cfg.DownloadDir, folderID)
	_ = os.MkdirAll(taskDir, 0o755)
	_ = s.store.UpsertFolder(folderID, folderID, taskDir)

	successCount := 0
	failCount := 0

	notifyCancelled := func() {
		notify(ProgressEvent{
			Type:         "cancelled",
			TaskID:       taskID,
			SuccessCount: successCount,
			FailCount:    failCount,
			Total:        len(urls),
			Error:        "下载已中断",
			DownloadDir:  taskDir,
		})
	}

	for i, rawURL := range urls {
		select {
		case <-ctx.Done():
			notifyCancelled()
			return
		default:
		}

		filename := safepath.FilenameFromURL(rawURL)
		savePath := filepath.Join(taskDir, filename)
		base := strings.TrimSuffix(filename, filepath.Ext(filename))

		if existing, ok, err := s.store.FileExists(folderID, filename); err == nil && ok {
			notify(ProgressEvent{
				Type:     "skipped",
				TaskID:   taskID,
				Current:  i + 1,
				Total:    len(urls),
				Filename: existing,
				URL:      rawURL,
				Filepath: filepath.Join(taskDir, existing),
			})
			continue
		} else if pathExistsAnyExt(taskDir, base) {
			existingName := findExistingName(taskDir, base)
			info, _ := os.Stat(filepath.Join(taskDir, existingName))
			var size int64
			if info != nil {
				size = info.Size()
			}
			_ = s.store.AddFile(folderID, existingName, size, filepath.Join(taskDir, existingName))
			notify(ProgressEvent{
				Type:     "skipped",
				TaskID:   taskID,
				Current:  i + 1,
				Total:    len(urls),
				Filename: existingName,
				URL:      rawURL,
				Filepath: filepath.Join(taskDir, existingName),
			})
			continue
		}

		notify(ProgressEvent{
			Type:     "start",
			TaskID:   taskID,
			Current:  i + 1,
			Total:    len(urls),
			Filename: filename,
			URL:      rawURL,
		})

		savedPath, err := s.downloadImage(ctx, rawURL, savePath, cookie)
		if err != nil {
			if ctx.Err() != nil {
				notifyCancelled()
				return
			}
			failCount++
			notify(ProgressEvent{
				Type:     "error",
				TaskID:   taskID,
				Current:  i + 1,
				Total:    len(urls),
				Filename: filename,
				URL:      rawURL,
				Error:    err.Error(),
			})
		} else {
			successCount++
			info, _ := os.Stat(savedPath)
			var size int64
			if info != nil {
				size = info.Size()
			}
			_ = s.store.AddFile(folderID, filepath.Base(savedPath), size, savedPath)
			notify(ProgressEvent{
				Type:     "success",
				TaskID:   taskID,
				Current:  i + 1,
				Total:    len(urls),
				Filename: filepath.Base(savedPath),
				URL:      rawURL,
				Filepath: savedPath,
			})
		}

		if i < len(urls)-1 {
			select {
			case <-ctx.Done():
				notifyCancelled()
				return
			case <-time.After(time.Duration(s.cfg.DelayMS) * time.Millisecond):
			}
		}
	}

	notify(ProgressEvent{
		Type:         "complete",
		TaskID:       taskID,
		SuccessCount: successCount,
		FailCount:    failCount,
		Total:        len(urls),
		DownloadDir:  taskDir,
	})
}

func (s *Service) FetchArtwork(id, cookie string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net/ajax/illust/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.pixiv.net/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pixiv 返回 HTTP %d", resp.StatusCode)
	}
	return decodeJSON(body)
}

func (s *Service) downloadImage(ctx context.Context, rawURL, savePath, cookie string) (string, error) {
	currentExt := strings.ToLower(strings.TrimPrefix(filepath.Ext(savePath), "."))
	tryExts := uniqueExts(currentExt, []string{"jpg", "png"})

	var lastErr error
	for _, ext := range tryExts {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		targetURL := rawURL
		targetPath := savePath
		if ext != currentExt {
			targetURL = replaceExtInURL(rawURL, ext)
			targetPath = replaceExtInPath(savePath, ext)
		}

		if err := s.downloadOnce(ctx, targetURL, targetPath, cookie); err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = err
			continue
		}
		return targetPath, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("所有格式下载失败: %s", rawURL)
	}
	return "", lastErr
}

func (s *Service) downloadOnce(ctx context.Context, rawURL, savePath, cookie string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.pixiv.net/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return err
	}
	tmp := savePath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, savePath)
}

var extInURL = regexp.MustCompile(`\.(jpg|png)(?:\?|$)`)

func replaceExtInURL(rawURL, ext string) string {
	return extInURL.ReplaceAllStringFunc(rawURL, func(m string) string {
		if strings.HasSuffix(m, "?") {
			return "." + ext + "?"
		}
		return "." + ext
	})
}

func replaceExtInPath(path, ext string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + "." + ext
}

func uniqueExts(first string, rest []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(rest)+1)
	if first != "" {
		seen[first] = true
		out = append(out, first)
	}
	for _, e := range rest {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func pathExistsAnyExt(dir, base string) bool {
	for _, ext := range []string{"jpg", "png"} {
		if _, err := os.Stat(filepath.Join(dir, base+"."+ext)); err == nil {
			return true
		}
	}
	return false
}

func findExistingName(dir, base string) string {
	for _, ext := range []string{"jpg", "png"} {
		name := base + "." + ext
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return base + ".jpg"
}

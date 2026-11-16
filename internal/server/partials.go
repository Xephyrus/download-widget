package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Xephyrus/download-widget/internal/db"
	"github.com/Xephyrus/download-widget/internal/safepath"
)

func (s *Server) handlePartialsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := s.buildListView(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rend.render(w, "list_partials", view)
}

func (s *Server) handlePartialsFolders(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimPrefix(r.URL.Path, "/partials/folders/")
	rawPath = strings.Trim(rawPath, "/")
	if rawPath == "" {
		http.Error(w, "无效ID", http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(rawPath, "/tags") {
		idPart := strings.TrimSuffix(rawPath, "/tags")
		idPart = strings.Trim(idPart, "/")
		folderID, err := url.PathUnescape(idPart)
		if err != nil || folderID == "" {
			http.Error(w, "无效ID", http.StatusBadRequest)
			return
		}
		s.handlePartialsFolderTags(w, r, folderID)
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, err := url.PathUnescape(rawPath)
	if err != nil || folderID == "" {
		http.Error(w, "无效ID", http.StatusBadRequest)
		return
	}
	if err := s.deleteFolder(folderID); err != nil {
		switch {
		case errors.Is(err, errInvalidFolderID):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, errPathForbidden):
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	view, err := s.buildListView(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rend.render(w, "list_partials", view)
}

func (s *Server) handlePartialsFolderTags(w http.ResponseWriter, r *http.Request, folderID string) {
	switch r.Method {
	case http.MethodGet:
		folder, err := s.store.GetFolder(folderID)
		if err != nil || folder == nil {
			http.Error(w, "文件夹不存在", http.StatusNotFound)
			return
		}
		s.rend.render(w, "folder_tags_editor", map[string]any{
			"ID":   folder.ID,
			"Name": folder.Name,
			"Tags": folder.Tags,
		})
	case http.MethodPost, http.MethodPut:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "无效的表单", http.StatusBadRequest)
			return
		}
		if err := safepath.ValidateFolderID(folderID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tags := db.ParseTagsCSV(r.FormValue("tags"))
		if err := s.store.SetFolderTags(folderID, tags); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Prefer returning refreshed list when called from gallery table.
		if strings.Contains(r.Header.Get("HX-Target"), "tableBody") || r.FormValue("refreshList") == "1" {
			view, err := s.buildListView(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.rend.render(w, "list_partials", view)
			return
		}
		folder, err := s.store.GetFolder(folderID)
		if err != nil || folder == nil {
			http.Error(w, "保存成功，但读取失败", http.StatusInternalServerError)
			return
		}
		s.rend.render(w, "folder_tags_panel", map[string]any{
			"ID":   folder.ID,
			"Name": folder.Name,
			"Tags": folder.Tags,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePartialsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.rend.render(w, "download_error", map[string]any{"Message": "无效的表单"})
		return
	}

	sample := r.FormValue("url")
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	taskName := strings.TrimSpace(r.FormValue("taskName"))
	count, _ := strconv.Atoi(r.FormValue("count"))
	if count < 1 {
		count = 1
	}
	if cookie == "" {
		w.WriteHeader(http.StatusBadRequest)
		s.rend.render(w, "download_error", map[string]any{"Message": "请输入 Pixiv Cookie"})
		return
	}
	urls, err := generatePixivURLs(sample, count)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.rend.render(w, "download_error", map[string]any{"Message": err.Error()})
		return
	}
	if taskName == "" {
		taskName = fmt.Sprintf("task_%d", time.Now().UnixMilli())
	}
	if err := safepath.ValidateFolderID(taskName); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.rend.render(w, "download_error", map[string]any{"Message": err.Error()})
		return
	}

	redownload := formTruthy(r.FormValue("redownload"))
	if redownload {
		s.cancelDownloadTasksForFolder(taskName)
	}

	proxyPort, err := parseProxyPort(r.FormValue("proxyPort"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.rend.render(w, "download_error", map[string]any{"Message": err.Error()})
		return
	}

	taskID := strconv.FormatInt(time.Now().UnixMilli(), 10)
	s.startDownloadTask(taskID, urls, cookie, taskName, redownload, proxyPort)
	s.rend.render(w, "download_submitted", map[string]any{
		"TaskID":     taskID,
		"Redownload": redownload,
	})
}

func parseProxyPort(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "0" || strings.EqualFold(v, "auto") {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("无效的代理端口")
	}
	if n != 7890 && n != 7897 {
		return 0, fmt.Errorf("不支持的代理端口 %d（可选 7897 / 7890）", n)
	}
	return n, nil
}

func formTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func (s *Server) handlePartialsDownloadCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = r.ParseForm()
	taskID := strings.TrimSpace(r.FormValue("taskId"))
	cancelled := s.cancelDownloadTask(taskID)
	if len(cancelled) == 0 {
		w.WriteHeader(http.StatusNotFound)
		s.rend.render(w, "download_error", map[string]any{"Message": "没有可中断的下载任务"})
		return
	}
	s.rend.render(w, "download_cancel_ack", map[string]any{"TaskIDs": cancelled})
}

func (s *Server) handlePartialsArtworks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/partials/artworks/"))
	if err != nil || id == "" {
		s.rend.render(w, "gallery_partial", map[string]any{"ID": id, "Error": "未识别 artwork ID。"})
		return
	}
	images, err := s.listArtworkImages(id)
	if err != nil {
		s.rend.render(w, "gallery_partial", map[string]any{"ID": id, "Error": err.Error()})
		return
	}
	s.rend.render(w, "gallery_partial", map[string]any{"ID": id, "Images": images})
}

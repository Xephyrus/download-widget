package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func (s *Server) handlePartialsDeleteFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/partials/folders/"))
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
	if count > 100 {
		count = 100
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
		if err := s.clearFolderContents(taskName); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.rend.render(w, "download_error", map[string]any{"Message": err.Error()})
			return
		}
	}

	taskID := strconv.FormatInt(time.Now().UnixMilli(), 10)
	s.startDownloadTask(taskID, urls, cookie, taskName)
	s.rend.render(w, "download_submitted", map[string]any{
		"TaskID":     taskID,
		"Redownload": redownload,
	})
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

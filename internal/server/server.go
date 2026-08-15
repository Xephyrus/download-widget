package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Xephyrus/download-widget/internal/config"
	"github.com/Xephyrus/download-widget/internal/db"
	"github.com/Xephyrus/download-widget/internal/downloader"
	"github.com/Xephyrus/download-widget/internal/safepath"

	"github.com/gorilla/websocket"
)

type Server struct {
	cfg              config.Config
	store            *db.Store
	dl               *downloader.Service
	mux              *http.ServeMux
	upgrader         websocket.Upgrader
	clientsMu        sync.Mutex
	clients          map[*websocket.Conn]struct{}
	broadcastWriteMu sync.Mutex
	downloadRoot     string
	rend             *renderer
	progressMu       sync.Mutex
	progress         map[string]*wsProgressState
	tasksMu          sync.Mutex
	tasks            map[string]*runningTask
}

func New(cfg config.Config, store *db.Store, dl *downloader.Service) *Server {
	rend, err := newRenderer()
	if err != nil {
		panic("load templates: " + err.Error())
	}
	s := &Server{
		cfg:          cfg,
		store:        store,
		dl:           dl,
		mux:          http.NewServeMux(),
		clients:      make(map[*websocket.Conn]struct{}),
		downloadRoot: cfg.DownloadDir,
		rend:         rend,
		progress:     make(map[string]*wsProgressState),
		tasks:        make(map[string]*runningTask),
		upgrader: websocket.Upgrader{
			CheckOrigin: checkWSOrigin,
		},
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

// svgContentType forces image/svg+xml for .svg files (Windows MIME registry is often wrong).
func svgContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.ToLower(r.URL.Path), ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		}
		next.ServeHTTP(w, r)
	})
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.ToLower(r.URL.Path)
		switch {
		case strings.HasPrefix(path, "vendor/"):
			w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
		case strings.HasSuffix(path, ".css"), strings.HasSuffix(path, ".js"), strings.HasSuffix(path, ".svg"):
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	fs := http.FileServer(http.Dir(s.cfg.WebDir))
	s.mux.Handle("/static/", http.StripPrefix("/static/", cacheStatic(svgContentType(fs))))
	s.mux.HandleFunc("/download/", s.handleDownloadFile)

	for _, name := range []string{
		"apple-touch-icon.png",
		"favicon-16x16.png",
		"favicon-32x32.png",
		"favicon.ico",
		"favicon.svg",
		"android-chrome-192x192.png",
		"android-chrome-512x512.png",
		"site.webmanifest",
	} {
		name := name
		s.mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/"+name {
				http.NotFound(w, r)
				return
			}
			path := filepath.Join(s.cfg.WebDir, name)
			switch {
			case strings.HasSuffix(name, ".webmanifest"):
				w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
			case strings.HasSuffix(name, ".svg"):
				w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
			}
			http.ServeFile(w, r, path)
		})
	}

	s.mux.HandleFunc("/thumb/", s.handleThumb)
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/download", s.handleDownloaderPage)
	s.mux.HandleFunc("/artworks/", s.handleArtworksPage)

	s.mux.HandleFunc("/partials/list", s.handlePartialsList)
	s.mux.HandleFunc("/partials/folders/", s.handlePartialsDeleteFolder)
	s.mux.HandleFunc("/partials/download", s.handlePartialsDownload)
	s.mux.HandleFunc("/partials/download/cancel", s.handlePartialsDownloadCancel)
	s.mux.HandleFunc("/partials/artworks/", s.handlePartialsArtworks)

	s.mux.HandleFunc("/api/list", s.handleList)
	s.mux.HandleFunc("/api/download", s.handleDownload)
	s.mux.HandleFunc("/api/download/cancel", s.handleAPIDownloadCancel)
	s.mux.HandleFunc("/api/delete/", s.handleDelete)
	s.mux.HandleFunc("/api/artworks/", s.handleArtworkImages)
	s.mux.HandleFunc("/api/artwork-id/", s.handleArtworkID)
	s.mux.HandleFunc("/ws", s.handleWS)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if websocket.IsWebSocketUpgrade(r) {
			s.handleWS(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	if websocket.IsWebSocketUpgrade(r) {
		s.handleWS(w, r)
		return
	}

	raw, err := os.ReadFile(filepath.Join(s.cfg.WebDir, "pixiv.html"))
	if err != nil {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	html := string(raw)

	view, err := s.buildListView(r)
	if err != nil {
		writeHTML(w, http.StatusOK, html)
		return
	}
	stats, err1 := s.rend.renderString("stats", view)
	rows, err2 := s.rend.renderString("list_rows", view)
	pag, err3 := s.rend.renderString("pagination", view)
	if err1 == nil {
		html = injectSSR(html, "<!--SSR_STATS_START-->", "<!--SSR_STATS_END-->", stats)
	}
	if err2 == nil {
		html = injectSSR(html, "<!--SSR_ROWS_START-->", "<!--SSR_ROWS_END-->", rows)
	}
	if err3 == nil {
		html = injectSSR(html, "<!--SSR_PAGINATION_START-->", "<!--SSR_PAGINATION_END-->", pag)
	}
	writeHTML(w, http.StatusOK, html)
}

func injectSSR(html, start, end, content string) string {
	i := strings.Index(html, start)
	j := strings.Index(html, end)
	if i < 0 || j < 0 || j < i {
		return html
	}
	return html[:i+len(start)] + "\n" + content + "\n" + html[j:]
}

func (s *Server) handleDownloaderPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.cfg.WebDir, "downloader.html"))
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, filename, ok := parseFolderFilePath(r.URL.Path, "/download/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	folderPath, err := s.resolveFolderPath(folderID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	srcPath := filepath.Join(folderPath, filename)
	if !pathUnderRoot(folderPath, srcPath) {
		http.Error(w, "访问被拒绝", http.StatusForbidden)
		return
	}
	if _, err := os.Stat(srcPath); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, srcPath)
}

func (s *Server) handleArtworksPage(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(filepath.Join(s.cfg.WebDir, "artworks.html"))
	if err != nil {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	html := string(raw)

	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/artworks/"))
	if err != nil {
		id = ""
	}
	id = strings.Trim(strings.TrimSpace(id), "/")

	data := map[string]any{"ID": id}
	if id == "" {
		data["Error"] = "未识别 artwork ID。"
	} else if images, listErr := s.listArtworkImages(id); listErr != nil {
		data["Error"] = listErr.Error()
	} else {
		data["Images"] = images
	}

	if id != "" {
		html = injectSSR(html, "<!--SSR_ID_START-->", "<!--SSR_ID_END-->", template.HTMLEscapeString(id))
	}
	if cards, err := s.rend.renderString("gallery_cards", data); err == nil {
		html = injectSSR(html, "<!--SSR_GALLERY_START-->", "<!--SSR_GALLERY_END-->", cards)
		html = strings.Replace(html, `data-ssr="0"`, `data-ssr="1"`, 1)
	}
	if status, err := s.rend.renderString("gallery_status", data); err == nil {
		html = injectSSR(html, "<!--SSR_STATUS_START-->", "<!--SSR_STATUS_END-->", status)
	}
	writeHTML(w, http.StatusOK, html)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.clientsMu.Lock()
	s.clients[conn] = struct{}{}
	s.clientsMu.Unlock()

	go func() {
		defer func() {
			s.clientsMu.Lock()
			delete(s.clients, conn)
			s.clientsMu.Unlock()
			_ = conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodPost) {
		return
	}
	var req struct {
		URLs       []string `json:"urls"`
		Cookie     string   `json:"cookie"`
		TaskName   string   `json:"taskName"`
		Redownload bool     `json:"redownload"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if len(req.URLs) == 0 {
		writeJSONErr(w, http.StatusBadRequest, "请提供有效的URL列表")
		return
	}
	if strings.TrimSpace(req.Cookie) == "" {
		writeJSONErr(w, http.StatusBadRequest, "请提供Pixiv Cookie")
		return
	}
	taskName := strings.TrimSpace(req.TaskName)
	if taskName == "" {
		taskName = fmt.Sprintf("task_%d", time.Now().UnixMilli())
	}
	if err := safepath.ValidateFolderID(taskName); err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Redownload {
		s.cancelDownloadTasksForFolder(taskName)
	}

	taskID := strconv.FormatInt(time.Now().UnixMilli(), 10)
	writeJSONOK(w, map[string]any{
		"taskId":     taskID,
		"redownload": req.Redownload,
		"message":    "任务已开始，将通过WebSocket推送进度",
	})

	s.startDownloadTask(taskID, req.URLs, req.Cookie, taskName, req.Redownload)
}

func (s *Server) handleAPIDownloadCancel(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodPost) {
		return
	}
	var req struct {
		TaskID string `json:"taskId"`
	}
	_ = jsonDecode(r, &req)
	if req.TaskID == "" {
		req.TaskID = strings.TrimSpace(r.URL.Query().Get("taskId"))
	}
	cancelled := s.cancelDownloadTask(req.TaskID)
	if len(cancelled) == 0 {
		writeJSONErr(w, http.StatusNotFound, "没有可中断的下载任务")
		return
	}
	writeJSONOK(w, map[string]any{
		"cancelled": cancelled,
		"message":   "已发送中断信号",
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	filter := parseListFilter(r)
	result, err := s.store.ListFolders(filter)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	page := filter.Page
	pageSize := filter.PageSize
	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + pageSize - 1) / pageSize
	}
	from := 0
	to := 0
	if result.Total > 0 {
		from = (page-1)*pageSize + 1
		to = from + len(result.Folders) - 1
	}

	writeJSONOK(w, map[string]any{
		"folders": result.Folders,
		"pagination": map[string]any{
			"currentPage": page,
			"pageSize":    pageSize,
			"total":       result.Total,
			"totalPages":  totalPages,
			"hasNext":     page < totalPages,
			"hasPrev":     page > 1,
			"from":        from,
			"to":          to,
		},
		"filters": listFilterEcho(r, filter),
		"stats": map[string]any{
			"totalFolders": result.Total,
			"totalFiles":   result.TotalFiles,
			"totalSize":    result.TotalSize,
		},
	})
}

func (s *Server) handleArtworkImages(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/artworks/"))
	if err != nil || id == "" {
		writeJSONErr(w, http.StatusBadRequest, "无效ID")
		return
	}
	images, err := s.listArtworkImages(id)
	if err != nil {
		switch {
		case errorsIs(err, errInvalidFolderID):
			writeJSONErr(w, http.StatusBadRequest, err.Error())
		case errorsIs(err, errFolderNotFound):
			writeJSONErr(w, http.StatusNotFound, err.Error())
		default:
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSONOK(w, map[string]any{
		"id":     id,
		"images": images,
	})
}

func (s *Server) handleArtworkID(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/artwork-id/"))
	if err != nil || strings.TrimSpace(id) == "" {
		writeJSONErr(w, http.StatusBadRequest, "无效ID")
		return
	}
	cookie := pixivCookieFromRequest(r)
	data, err := s.dl.FetchArtwork(id, cookie)
	if err != nil {
		writeJSONErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSONOK(w, data)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodDelete) {
		return
	}
	folderID, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/delete/"))
	if err != nil || folderID == "" {
		writeJSONErr(w, http.StatusBadRequest, "无效ID")
		return
	}
	if err := s.deleteFolder(folderID); err != nil {
		switch {
		case errorsIs(err, errInvalidFolderID):
			writeJSONErr(w, http.StatusBadRequest, err.Error())
		case errorsIs(err, errPathForbidden):
			writeJSONErr(w, http.StatusForbidden, err.Error())
		default:
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSONOK(w, map[string]any{"deleted": folderID})
}

func pixivCookieFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Pixiv-Cookie")); v != "" {
		return v
	}
	_ = r.ParseForm()
	if v := strings.TrimSpace(r.Form.Get("cookie")); v != "" {
		return v
	}
	return ""
}

func checkWSOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	ou, err := url.Parse(origin)
	if err != nil || ou.Host == "" {
		return false
	}
	return strings.EqualFold(ou.Host, r.Host)
}

func jsonDecode(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func errorsIs(err, target error) bool {
	return errors.Is(err, target)
}

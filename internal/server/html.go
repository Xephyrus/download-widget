package server

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Xephyrus/download-widget/internal/db"
	"github.com/Xephyrus/download-widget/internal/downloader"
)

//go:embed templates/*.html
var templateFS embed.FS

type renderer struct {
	tmpl *template.Template
}

func newRenderer() (*renderer, error) {
	funcMap := template.FuncMap{
		"formatSize": formatFileSize,
		"formatTime": formatDateTime,
		"urlquery":   url.QueryEscape,
		"urlpath":    url.PathEscape,
		"add":        func(a, b int) int { return a + b },
	}
	tmpl, err := template.New("root").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &renderer{tmpl: tmpl}, nil
}

func (r *renderer) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *renderer) renderString(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type listView struct {
	Folders      []db.Folder
	Page         int
	PageSize     int
	Total        int
	TotalPages   int
	TotalFolders int
	TotalFiles   int
	TotalSize    int64
	HasPrev      bool
	HasNext      bool
	PrevPage     int
	NextPage     int
	PageNums     []int
	Query        template.URL
}

func formatFileSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(bytes)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006/01/02 15:04")
}

func buildListQuery(q url.Values) string {
	vals := url.Values{}
	for _, key := range []string{"pageSize", "sortBy", "sortOrder", "search", "minFileCount", "maxFileCount", "exactMatch", "startDate", "endDate"} {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			vals.Set(key, v)
		}
	}
	return vals.Encode()
}

func (s *Server) buildListView(r *http.Request) (*listView, error) {
	filter := parseListFilter(r)
	page := filter.Page
	pageSize := filter.PageSize
	result, err := s.store.ListFolders(filter)
	if err != nil {
		return nil, err
	}
	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + pageSize - 1) / pageSize
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
		filter.Page = page
		result, err = s.store.ListFolders(filter)
		if err != nil {
			return nil, err
		}
	}

	start := page - 2
	if start < 1 {
		start = 1
	}
	end := page + 2
	if end > totalPages {
		end = totalPages
	}
	nums := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		nums = append(nums, i)
	}

	q := r.URL.Query()
	_ = r.ParseForm()
	for _, key := range []string{"pageSize", "sortBy", "sortOrder", "search", "minFileCount", "maxFileCount", "exactMatch", "startDate", "endDate"} {
		if q.Get(key) == "" && r.Form.Get(key) != "" {
			q.Set(key, r.Form.Get(key))
		}
	}

	return &listView{
		Folders:      result.Folders,
		Page:         page,
		PageSize:     pageSize,
		Total:        result.Total,
		TotalPages:   totalPages,
		TotalFolders: result.Total,
		TotalFiles:   result.TotalFiles,
		TotalSize:    result.TotalSize,
		HasPrev:      page > 1,
		HasNext:      page < totalPages,
		PrevPage:     page - 1,
		NextPage:     page + 1,
		PageNums:     nums,
		Query:        template.URL(buildListQuery(q)),
	}, nil
}

func writeHTML(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, html)
}

func generatePixivURLs(sample string, count int) ([]string, error) {
	sample = strings.TrimSpace(sample)
	if sample == "" {
		return nil, fmt.Errorf("请输入示例图片URL")
	}
	// take first line if multiline
	if i := strings.IndexAny(sample, "\r\n"); i >= 0 {
		sample = strings.TrimSpace(sample[:i])
	}
	const marker = "_p"
	idx := strings.LastIndex(strings.ToLower(sample), marker)
	if idx < 0 {
		return nil, fmt.Errorf("URL格式不正确，应为 ..._p0.jpg 格式")
	}
	// find _pN.ext
	rest := sample[idx+2:]
	dot := strings.Index(rest, ".")
	if dot < 0 {
		return nil, fmt.Errorf("URL格式不正确，应为 ..._p0.jpg 格式")
	}
	base := sample[:idx+2]
	ext := rest[dot+1:]
	if q := strings.IndexAny(ext, "?#"); q >= 0 {
		ext = ext[:q]
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, fmt.Sprintf("%s%d.%s", base, i, ext))
	}
	return out, nil
}

func statusFromEvent(ev downloader.ProgressEvent) (class, icon, message, linkHref, linkText string) {
	switch ev.Type {
	case "start":
		return "status-start", "↓", fmt.Sprintf("开始下载：%s（%d/%d）", ev.Filename, ev.Current, ev.Total), "", ""
	case "success":
		return "status-success", "✓", fmt.Sprintf("下载成功：%s（%d/%d）", ev.Filename, ev.Current, ev.Total), "", ""
	case "error":
		return "status-error", "!", fmt.Sprintf("下载失败：%s — %s（%d/%d）", ev.Filename, ev.Error, ev.Current, ev.Total), "", ""
	case "skipped":
		return "status-start", "↷", fmt.Sprintf("已存在跳过：%s（%d/%d）", ev.Filename, ev.Current, ev.Total), "", ""
	case "complete":
		folderID := filepath.Base(strings.TrimSpace(ev.DownloadDir))
		msg := fmt.Sprintf("任务完成：成功 %d，失败 %d。", ev.SuccessCount, ev.FailCount)
		if folderID != "" && folderID != "." && folderID != string(filepath.Separator) {
			return "status-complete", "★", msg, "/artworks/" + url.PathEscape(folderID), "查看作品"
		}
		return "status-complete", "★", msg, "", ""
	case "cancelled":
		return "status-error", "■", fmt.Sprintf("下载已中断（成功 %d，失败 %d）", ev.SuccessCount, ev.FailCount), "", ""
	case "task_error":
		return "status-error", "!", fmt.Sprintf("任务错误：%s", ev.Error), "", ""
	default:
		return "status-start", "·", ev.Type, "", ""
	}
}

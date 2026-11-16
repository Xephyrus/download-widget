package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Xephyrus/download-widget/internal/db"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// parseListFilter reads list filters from query and form (for hx-include).
func parseListFilter(r *http.Request) db.ListFilter {
	q := r.URL.Query()
	_ = r.ParseForm()
	get := func(key, fallback string) string {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			return v
		}
		if v := strings.TrimSpace(r.Form.Get(key)); v != "" {
			return v
		}
		return fallback
	}

	page, _ := strconv.Atoi(get("page", "1"))
	pageSize, _ := strconv.Atoi(get("pageSize", strconv.Itoa(defaultPageSize)))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	filter := db.ListFilter{
		Page:       page,
		PageSize:   pageSize,
		SortBy:     get("sortBy", "createdAt"),
		SortOrder:  get("sortOrder", "desc"),
		Search:     get("search", ""),
		Tag:        get("tag", ""),
		ExactMatch: get("exactMatch", "") == "true",
	}
	if v := get("minFileCount", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.MinFileCount = &n
		}
	}
	if v := get("maxFileCount", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.MaxFileCount = &n
		}
	}
	if v := get("startDate", ""); v != "" {
		if t, ok := parseFilterDate(v, false); ok {
			filter.StartDate = &t
		}
	}
	if v := get("endDate", ""); v != "" {
		if t, ok := parseFilterDate(v, true); ok {
			filter.EndDate = &t
		}
	}
	return filter
}

func parseFilterDate(v string, endOfDay bool) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t, true
	}
	return time.Time{}, false
}

func listFilterEcho(r *http.Request, filter db.ListFilter) map[string]any {
	q := r.URL.Query()
	_ = r.ParseForm()
	get := func(key string) string {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			return v
		}
		return strings.TrimSpace(r.Form.Get(key))
	}
	return map[string]any{
		"search":       nullIfEmpty(get("search")),
		"tag":          nullIfEmpty(get("tag")),
		"startDate":    nullIfEmpty(get("startDate")),
		"endDate":      nullIfEmpty(get("endDate")),
		"minFileCount": nullIfEmpty(get("minFileCount")),
		"maxFileCount": nullIfEmpty(get("maxFileCount")),
		"exactMatch":   filter.ExactMatch,
		"sortBy":       filter.SortBy,
		"sortOrder":    filter.SortOrder,
	}
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

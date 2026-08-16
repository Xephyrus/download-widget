package server

import (
	"encoding/json"
	"time"

	"github.com/Xephyrus/download-widget/internal/downloader"

	"github.com/gorilla/websocket"
)

type wsProgressState struct {
	success int
	fail    int
	skipped int
	total   int
}

func (s *Server) broadcastHTMX(ev downloader.ProgressEvent) {
	class, icon, message, linkHref, linkText := statusFromEvent(ev)
	dlState := ""
	switch ev.Type {
	case "cancelled":
		dlState = "cancelled"
	case "complete":
		dlState = "complete"
	case "task_error":
		dlState = "error"
	}
	itemHTML, err := s.rend.renderString("status_item", map[string]any{
		"Class":    class,
		"Icon":     icon,
		"Message":  message,
		"LinkHref": linkHref,
		"LinkText": linkText,
		"Time":     time.Now().Format("15:04:05"),
		"State":    dlState,
		"TaskID":   ev.TaskID,
	})
	if err == nil {
		s.broadcastText(itemHTML)
	}

	st := s.progressFor(ev)
	if st.total <= 0 && ev.Total > 0 {
		st.total = ev.Total
	}
	processed := st.success + st.fail + st.skipped
	percent := 0.0
	if st.total > 0 {
		percent = float64(processed) / float64(st.total) * 100
	}
	if ev.Type == "complete" {
		percent = 100
		st.success = ev.SuccessCount
		st.fail = ev.FailCount
		if ev.Total > 0 {
			st.total = ev.Total
		}
		processed = st.success + st.fail + st.skipped
	}
	progressHTML, err := s.rend.renderString("progress_block", map[string]any{
		"Success":   st.success,
		"Fail":      st.fail,
		"Processed": processed,
		"Total":     st.total,
		"Percent":   percent,
	})
	if err != nil {
		return
	}
	payload := map[string]any{
		"content": progressHTML,
		"target":  "#progress",
		"swap":    "innerHTML",
	}
	// Lifecycle hint so the download page can update badge/banner
	// even if a status HTML frame was dropped.
	if dlState != "" {
		payload["downloadState"] = dlState
		payload["taskId"] = ev.TaskID
		if ev.Type == "complete" {
			payload["success"] = ev.SuccessCount
			payload["fail"] = ev.FailCount
		}
	}
	raw, _ := json.Marshal(payload)
	s.broadcastText(string(raw))
}

func (s *Server) progressFor(ev downloader.ProgressEvent) wsProgressState {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	st, ok := s.progress[ev.TaskID]
	if !ok {
		st = &wsProgressState{}
		s.progress[ev.TaskID] = st
	}
	if ev.Total > 0 {
		st.total = ev.Total
	}
	switch ev.Type {
	case "success":
		st.success++
	case "error":
		st.fail++
	case "skipped":
		st.skipped++
	case "clearing":
		// prepare UI only; counts unchanged
	case "complete":
		st.success = ev.SuccessCount
		st.fail = ev.FailCount
		out := *st
		delete(s.progress, ev.TaskID)
		return out
	case "cancelled":
		st.success = ev.SuccessCount
		st.fail = ev.FailCount
		out := *st
		delete(s.progress, ev.TaskID)
		return out
	case "task_error":
		out := *st
		delete(s.progress, ev.TaskID)
		return out
	}
	return *st
}

func (s *Server) broadcastText(text string) {
	s.broadcastWriteMu.Lock()
	defer s.broadcastWriteMu.Unlock()

	s.clientsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.clients))
	for conn := range s.clients {
		conns = append(conns, conn)
	}
	s.clientsMu.Unlock()

	var dead []*websocket.Conn
	for _, conn := range conns {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(text)); err != nil {
			dead = append(dead, conn)
			continue
		}
		_ = conn.SetWriteDeadline(time.Time{})
	}
	if len(dead) == 0 {
		return
	}
	s.clientsMu.Lock()
	for _, conn := range dead {
		delete(s.clients, conn)
		_ = conn.Close()
	}
	s.clientsMu.Unlock()
}

package server

import (
	"context"

	"github.com/Xephyrus/download-widget/internal/downloader"
)

type runningTask struct {
	cancel   context.CancelFunc
	folderID string
}

func (s *Server) startDownloadTask(taskID string, urls []string, cookie, taskName string, redownload bool, proxyPort int) {
	ctx, cancel := context.WithCancel(context.Background())
	folderID := taskName
	if folderID == "" {
		folderID = taskID
	}

	s.tasksMu.Lock()
	if s.tasks == nil {
		s.tasks = make(map[string]*runningTask)
	}
	s.tasks[taskID] = &runningTask{cancel: cancel, folderID: folderID}
	s.tasksMu.Unlock()

	go func() {
		defer func() {
			s.tasksMu.Lock()
			delete(s.tasks, taskID)
			s.tasksMu.Unlock()
			cancel()
		}()
		if redownload {
			s.broadcastHTMX(downloader.ProgressEvent{
				Type:   "clearing",
				TaskID: taskID,
				Total:  len(urls),
			})
			if err := s.clearFolderContents(folderID); err != nil {
				s.broadcastHTMX(downloader.ProgressEvent{
					Type:   "task_error",
					TaskID: taskID,
					Error:  err.Error(),
				})
				return
			}
		}
		s.dl.BatchDownload(ctx, taskID, urls, cookie, taskName, proxyPort, s.broadcastHTMX)
	}()
}

// cancelDownloadTask cancels one task, or all when taskID is empty.
func (s *Server) cancelDownloadTask(taskID string) (cancelled []string) {
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	if s.tasks == nil {
		return nil
	}
	if taskID == "" {
		for id, t := range s.tasks {
			t.cancel()
			cancelled = append(cancelled, id)
		}
		return cancelled
	}
	t, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	t.cancel()
	return []string{taskID}
}

// cancelDownloadTasksForFolder cancels running downloads that write into folderID.
func (s *Server) cancelDownloadTasksForFolder(folderID string) (cancelled []string) {
	if folderID == "" {
		return nil
	}
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	for id, t := range s.tasks {
		if t != nil && t.folderID == folderID {
			t.cancel()
			cancelled = append(cancelled, id)
		}
	}
	return cancelled
}

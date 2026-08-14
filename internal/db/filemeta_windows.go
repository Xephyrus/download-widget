//go:build windows

package db

import (
	"os"
	"syscall"
	"time"
)

func diskTimes(info os.FileInfo) (created, modified time.Time) {
	modified = info.ModTime().UTC()
	created = modified
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return created, modified
	}
	created = time.Unix(0, stat.CreationTime.Nanoseconds()).UTC()
	modified = time.Unix(0, stat.LastWriteTime.Nanoseconds()).UTC()
	return created, modified
}

//go:build !windows

package db

import (
	"os"
	"time"
)

func diskTimes(info os.FileInfo) (created, modified time.Time) {
	modified = info.ModTime().UTC()
	created = modified
	return created, modified
}

package main

import (
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"

	"github.com/Xephyrus/download-widget/internal/config"
	"github.com/Xephyrus/download-widget/internal/db"
	"github.com/Xephyrus/download-widget/internal/downloader"
	"github.com/Xephyrus/download-widget/internal/server"
)

func main() {
	// Windows registry often maps .svg to a non-standard type; force the correct one.
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".svgz", "image/svg+xml")

	cfg := config.Load()
	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		log.Fatalf("创建下载目录失败: %v", err)
	}

	store, legacyMigrated, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()
	store.SetRoot(cfg.DownloadDir)

	needSync := os.Getenv("SYNC_ON_START") != "0" || legacyMigrated
	if legacyMigrated {
		log.Printf("检测到旧 SQLite 库，已备份并切换为 bbolt，将从磁盘重建索引")
	}
	if needSync {
		folders, files, syncErr := store.SyncFromDisk(cfg.DownloadDir)
		if syncErr != nil {
			log.Printf("同步磁盘到数据库失败: %v", syncErr)
		} else {
			log.Printf("已同步磁盘目录到数据库: %d 个文件夹, %d 个文件", folders, files)
		}
		if compactErr := store.Compact(); compactErr != nil {
			log.Printf("压缩数据库失败: %v", compactErr)
		} else if fi, statErr := os.Stat(cfg.DBPath); statErr == nil {
			log.Printf("数据库已压缩: %.2f MB", float64(fi.Size())/(1<<20))
		}
	}

	dl := downloader.New(cfg, store)
	srv := server.New(cfg, store, dl)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	fmt.Printf("监听地址: %s\n", addr)
	fmt.Printf("本机访问: http://127.0.0.1:%s\n", cfg.Port)
	for _, ip := range lanIPv4s() {
		fmt.Printf("局域网访问: http://%s:%s\n", ip, cfg.Port)
	}
	fmt.Printf("WebSocket: / 或 /ws\n")
	fmt.Printf("数据库: %s\n", cfg.DBPath)
	fmt.Printf("下载目录: %s\n", cfg.DownloadDir)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func lanIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil || !ip.IsPrivate() {
				continue
			}
			s := ip.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

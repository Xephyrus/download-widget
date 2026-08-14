package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host        string
	Port        string
	ProxyHost   string
	ProxyPort   int
	DelayMS     int
	TimeoutSec  int
	DownloadDir string
	DBPath      string
	WebDir      string
}

func Load() Config {
	root, _ := os.Getwd()
	cfg := Config{
		// 0.0.0.0 = 本机 + 局域网；仅本机可设 HOST=127.0.0.1
		Host:        getenv("HOST", "0.0.0.0"),
		Port:        getenv("PORT", "7892"),
		ProxyHost:   getenv("PROXY_HOST", "127.0.0.1"),
		ProxyPort:   getenvInt("PROXY_PORT", 7897),
		DelayMS:     getenvInt("DOWNLOAD_DELAY_MS", 1000),
		TimeoutSec:  getenvInt("DOWNLOAD_TIMEOUT_SEC", 30),
		DownloadDir: getenv("DOWNLOAD_DIR", filepath.Join(root, "downloads")),
		DBPath:      getenv("DB_PATH", filepath.Join(root, "data", "pixiv.db")),
		WebDir:      getenv("WEB_DIR", filepath.Join(root, "web")),
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

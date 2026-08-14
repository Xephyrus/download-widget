package server

import (
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultThumbWidth = 420
	maxThumbWidth     = 960
)

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, filename, ok := parseFolderFilePath(r.URL.Path, "/thumb/")
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

	width := defaultThumbWidth
	if v := strings.TrimSpace(r.URL.Query().Get("w")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			width = n
		}
	}
	if width > maxThumbWidth {
		width = maxThumbWidth
	}

	cacheDir := filepath.Join(s.downloadRoot, ".thumbs", folderID)
	cacheName := fmt.Sprintf("w%d_%s.jpg", width, sanitizeThumbStem(filename))
	cachePath := filepath.Join(cacheDir, cacheName)

	if serveCachedThumb(w, r, cachePath, srcPath) {
		return
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		// Still try to serve original if cache dir cannot be created.
		http.ServeFile(w, r, srcPath)
		return
	}
	if err := generateThumbJPEGLocked(cachePath, srcPath, width); err != nil {
		http.ServeFile(w, r, srcPath)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, cachePath)
}

func sanitizeThumbStem(name string) string {
	base := filepath.Base(name)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = "img"
	}
	replacer := strings.NewReplacer("/", "_", `\`, "_", "..", "_", ":", "_")
	return replacer.Replace(stem)
}

func serveCachedThumb(w http.ResponseWriter, r *http.Request, cachePath, srcPath string) bool {
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false
	}
	if cacheInfo.ModTime().Before(srcInfo.ModTime()) {
		return false
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, cachePath)
	return true
}

func generateThumbJPEGLocked(cachePath, srcPath string, maxW int) error {
	v, _ := thumbGenLocks.LoadOrStore(cachePath, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if cacheFresh(cachePath, srcPath) {
		return nil
	}
	return generateThumbJPEG(srcPath, cachePath, maxW)
}

var thumbGenLocks sync.Map

func cacheFresh(cachePath, srcPath string) bool {
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false
	}
	return !cacheInfo.ModTime().Before(srcInfo.ModTime())
}

func generateThumbJPEG(srcPath, dstPath string, maxW int) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid image size")
	}
	if w > maxW {
		outW := maxW
		outH := h * maxW / w
		if outH < 1 {
			outH = 1
		}
		img = resizeImage(img, outW, outH)
	}

	tmp := dstPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = jpeg.Encode(out, img, &jpeg.Options{Quality: 82})
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dstPath)
}

func resizeImage(src image.Image, outW, outH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func (s *Server) clearThumbCache(folderID string) {
	_ = os.RemoveAll(filepath.Join(s.downloadRoot, ".thumbs", folderID))
}

package server

import (
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	errFolderNotFound = errors.New("目录不存在")
	allDigits         = regexp.MustCompile(`\d+`)
)

func (s *Server) listArtworkImages(id string) (images []galleryImage, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errInvalidFolderID
	}
	folder, err := s.store.GetFolder(id)
	if err != nil {
		return nil, err
	}
	if folder == nil {
		return nil, errFolderNotFound
	}
	files, err := s.store.ListImageFiles(id)
	if err != nil {
		return nil, err
	}
	sortImages(files)
	images = make([]galleryImage, 0, len(files))
	for _, f := range files {
		images = append(images, galleryImage{
			Href:  "/download/" + url.PathEscape(id) + "/" + url.PathEscape(f),
			Thumb: "/thumb/" + url.PathEscape(id) + "/" + url.PathEscape(f) + "?w=420",
			Name:  f,
		})
	}
	return images, nil
}

type galleryImage struct {
	Href  string `json:"href"`
	Thumb string `json:"thumb"`
	Name  string `json:"name"`
}

func sortImages(arr []string) {
	sort.SliceStable(arr, func(i, j int) bool {
		ai := lastNumber(arr[i])
		aj := lastNumber(arr[j])
		if ai == aj {
			return arr[i] < arr[j]
		}
		return ai < aj
	})
}

func lastNumber(name string) int {
	matches := allDigits.FindAllString(name, -1)
	if len(matches) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(matches[len(matches)-1])
	return n
}

package api

import (
	"net/http"
	"path/filepath"
	"strings"
)

type VideoItem struct {
	Name        string
	Category    string
	EncodedPath string
}

func (h *Handler) HandleWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	files, err := h.Media.ListFiles()
	if err != nil {
		http.Error(w, "could not list files", http.StatusInternalServerError)
		return
	}

	// prepare the data for the template
	var items []VideoItem
	for _, f := range files {
		displayName := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))

		items = append(items, VideoItem{
			Name:        displayName,
			Category:    f.Category,
			EncodedPath: f.UUID.String(),
		})
	}

	h.render(w, "index.html", items)
}

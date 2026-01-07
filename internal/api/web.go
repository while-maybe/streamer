package api

import (
	"net/http"
	"net/url"
)

type VideoItem struct {
	Name        string
	Category    string
	EncodedPath string
}

func (h *Handler) HandleWeb(w http.ResponseWriter, r *http.Request) {
	files, err := h.Media.ListFiles()
	if err != nil {
		http.Error(w, "could not list files", http.StatusInternalServerError)
		return
	}

	// prepare the data for the template
	var items []VideoItem
	for _, f := range files {
		items = append(items, VideoItem{
			Name:        f.Name,
			Category:    f.Category,
			EncodedPath: url.QueryEscape(f.Path),
		})
	}

	h.render(w, "index.html", items)
}

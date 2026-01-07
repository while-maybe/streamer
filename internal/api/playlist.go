package api

import (
	"fmt"
	"net/http"
	"net/url"
)

func (h *Handler) HandleM3U(w http.ResponseWriter, r *http.Request) {
	mediaFiles, err := h.Media.ListFiles()
	if err != nil {
		http.Error(w, "could not list files", http.StatusInternalServerError)
		return
	}

	categoryFilter := r.URL.Query().Get("category")

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	// m3u Header
	fmt.Fprintln(w, "#EXTM3U")

	for _, f := range mediaFiles {

		// if filter has been set, skip the others
		if categoryFilter != "" && f.Category != categoryFilter {
			continue
		}

		safePath := url.PathEscape(f.Path)

		// streamURL := fmt.Sprintf("http://%s/stream?file=%s", r.Host, safePath)

		// Write the Entry to m3u
		// #EXTINF:-1,Action - Die Hard.mp4
		fmt.Fprintf(w, "#EXTINF:-1,%s - %s\n", f.Category, f.Name)
		// http://.../stream?file=Action/Die Hard.mp4
		fmt.Fprintf(w, "http://%s/stream?file=%s\n", r.Host, safePath)
	}
}

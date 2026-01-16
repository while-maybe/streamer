package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

func (h *Handler) HandleM3U(w http.ResponseWriter, r *http.Request) {
	entries := h.Media.Registry.List()

	categoryFilter := r.URL.Query().Get("category")

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	// m3u Header
	fmt.Fprintln(w, "#EXTM3U")

	for _, f := range entries {

		// if filter has been set, skip the others
		if categoryFilter != "" && f.Category != categoryFilter {
			continue
		}

		displayName := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
		// Write the Entry to m3u
		// #EXTINF:-1,Action - Die Hard.mp4
		fmt.Fprintf(w, "#EXTINF:-1,%s - %s\n", f.Category, displayName)
		// http://.../stream?file=Action/Die Hard.mp4
		fmt.Fprintf(w, "http://%s/stream?id=%s\n", r.Host, f.UUID.String())
	}
}

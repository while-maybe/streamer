package api

import (
	"errors"
	"net/http"
	"os"
	"streamer/internal/media"
	"strings"
)

func (h *Handler) AdapterDirectStream(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/direct/")

	resource, err := h.Media.OpenResource(path)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrPathOutsideRoot):
			h.logger.Warn("security alert: attempted path traversal", "path", path, "remote", r.RemoteAddr)
			http.Error(w, "forbidden", http.StatusForbidden)

		case errors.Is(err, os.ErrNotExist):
			http.Error(w, "file not found", http.StatusNotFound)

		case errors.Is(err, media.ErrUnsupportedMode):
			http.Error(w, "configuration error: unsupported mode", http.StatusNotImplemented)

		default:
			h.logger.Error("internal error opening file", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// close the file when request is done
	defer resource.Close()

	h.HandleDirectStream(w, r, resource)
}

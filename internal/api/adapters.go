package api

import (
	"errors"
	"net/http"
	"os"
	"streamer/internal/media"
	"strings"

	"github.com/gofrs/uuid/v5"
)

func (h *Handler) AdapterDirectStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/direct/")

	uuid, err := uuid.FromString(id)
	if err != nil {
		h.logger.Warn("id is not the right format", "id", id)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	entry, err := h.Media.GetEntry(uuid)
	if err != nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}

	resource, err := h.Media.OpenResource(entry.Path)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrPathOutsideRoot):
			h.logger.Warn("security alert: attempted path traversal", "path", entry.Path, "remote", r.RemoteAddr)
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

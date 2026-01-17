package api

import (
	"fmt"
	"net/http"
	"strconv"
	"streamer/internal/observability"
	"strings"

	"github.com/gofrs/uuid/v5"
)

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {

	id := r.URL.Query().Get("id")

	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	h.logger.Debug("stream request",
		"id", id,
		"range", r.Header.Get("Range"),
		"user_agent", r.Header.Get("User-Agent"),
	)

	uuid, err := uuid.FromString(id)
	if err != nil {
		h.logger.Warn("uuid parsing", "uuid", id, "err", err)
		http.Error(w, "bad id", http.StatusNotFound)
		return
	}

	// get the media entry for the given (now validated) uuid
	entry, err := h.Media.GetEntry(uuid)
	if err != nil {
		h.logger.Debug("entry for given id", "uuid", id, "err", err)
		http.Error(w, "couldn not match any media to given id", http.StatusNotFound)
		return
	}

	mount, err := h.Media.GetMount(entry.MountID)
	if err != nil {
		h.logger.Error("volume missing for entry", "vol_id", entry.MountID, "entry_id", id)
		http.Error(w, "storage volume unavailable", http.StatusServiceUnavailable)
		return
	}

	//  IO slot is available (will use semaphore)
	if err := mount.Limiter.TryAcquire(r.Context()); err != nil {
		h.logger.Warn("IO limiter reached", "id", id)
		http.Error(w, "server too busy", http.StatusServiceUnavailable)
		return
	}
	defer mount.Limiter.Release()

	resource, err := h.Media.OpenResource(entry)
	if err != nil {
		h.logger.Error("opening resource", "path", entry.Path, "err", err)
		http.Error(w, "file access error", http.StatusInternalServerError)
		return
	}
	defer resource.Close()

	// Get mime type and DLNA profile
	mimeType := getMimeType(resource.Name())

	// Set DLNA/UPnP headers BEFORE calling ServeContent
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")

	// is not here, browser will attempt to download content instead of playing
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, resource.Name()))

	// Some clients need explicit content length
	w.Header().Set("Content-Length", strconv.FormatInt(resource.Size(), 10))

	if isDLNAClient(r) {
		dlnaProfile := getDLNAProfile(resource.Name())
		// DLNA headers
		w.Header().Set("transferMode.dlna.org", "Streaming")
		w.Header().Set("contentFeatures.dlna.org", dlnaProfile)

		// Additional compatibility headers
		w.Header().Set("realTimeInfo.dlna.org", "DLNA.ORG_TLAG=*")
		w.Header().Set("Connection", "close")
	}

	h.logger.Debug("serving file",
		"name", resource.Name(),
		"bytes", resource.Size(),
		"mime_type", mimeType,
	)

	observability.ActiveStreams.Inc()
	defer observability.ActiveStreams.Dec()

	// Let ServeContent handle range requests and actual streaming
	http.ServeContent(w, r, resource.Name(), resource.ModTime(), resource)
}

// isDLNAClient checks if the request is from a DLNA/UPnP device
func isDLNAClient(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	// Check for DLNA/UPnP indicators
	dlnaIndicators := []string{
		"dlna",
		"upnp",
		"foobar", // Foobar2000
		"kodi",
		"xbmc",
		"plex",
		"roku",
		"lg ",     // LG Smart TVs
		"samsung", // Samsung Smart TVs
		"sony",    // Sony devices
		"playstation",
		"xbox",
		"windows media player",
		"vlc", // VLC can act as DLNA client
	}

	for _, indicator := range dlnaIndicators {
		if strings.Contains(ua, indicator) {
			return true
		}
	}

	// Check for specific DLNA headers
	if r.Header.Get("getcontentFeatures.dlna.org") != "" || r.Header.Get("transferMode.dlna.org") != "" {
		return true
	}
	return false
}

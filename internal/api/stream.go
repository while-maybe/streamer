package api

import (
	"fmt"
	"net/http"
	"strconv"
	"streamer/internal/observability"
	"strings"
)

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("file")

	if relPath == "" {
		http.Error(w, "file parameter is required", http.StatusBadRequest)
		return
	}

	h.logger.Debug("stream request",
		"file", relPath,
		"range", r.Header.Get("Range"),
		"user_agent", r.Header.Get("User-Agent"),
	)

	// Open the file to get info
	file, err := h.Media.OpenFile(relPath)
	if err != nil {
		h.logger.Error("opening file", "relpath", relPath, "err", err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		h.logger.Error("file info", "err", err)
		http.Error(w, "could not stat file", http.StatusInternalServerError)
		return
	}

	// Get mime type and DLNA profile
	mimeType := getMimeType(relPath)

	// Set DLNA/UPnP headers BEFORE calling ServeContent
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")

	// is not here, browser will attempt to download content instead of playing
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileInfo.Name()))

	// Some clients need explicit content length
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	if isDLNAClient(r) {
		dlnaProfile := getDLNAProfile(relPath)
		// DLNA headers
		w.Header().Set("transferMode.dlna.org", "Streaming")
		w.Header().Set("contentFeatures.dlna.org", dlnaProfile)

		// Additional compatibility headers
		w.Header().Set("realTimeInfo.dlna.org", "DLNA.ORG_TLAG=*")
		w.Header().Set("Connection", "close")
	}

	h.logger.Debug("serving file",
		"name", fileInfo.Name(),
		"bytes", fileInfo.Size(),
		"mime_type", mimeType,
	)

	observability.ActiveStreams.Inc()
	defer observability.ActiveStreams.Dec()

	// Let ServeContent handle range requests and actual streaming
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file)
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

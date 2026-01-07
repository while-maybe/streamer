package api

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"streamer/internal/media"
	"strings"
)

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    SOAPBody `xml:"Body"`
}

type SOAPBody struct {
	Browse                   *BrowseRequest                   `xml:"Browse"`
	GetSearchCapabilities    *GetSearchCapabilitiesRequest    `xml:"GetSearchCapabilities"`
	GetSortCapabilities      *GetSortCapabilitiesRequest      `xml:"GetSortCapabilities"`
	GetSystemUpdateID        *GetSystemUpdateIDRequest        `xml:"GetSystemUpdateID"`
	GetProtocolInfo          *GetProtocolInfoRequest          `xml:"GetProtocolInfo"`
	GetCurrentConnectionIDs  *GetCurrentConnectionIDsRequest  `xml:"GetCurrentConnectionIDs"`
	GetCurrentConnectionInfo *GetCurrentConnectionInfoRequest `xml:"GetCurrentConnectionInfo"`
}

type BrowseRequest struct {
	ObjectID       string `xml:"ObjectID"`
	BrowseFlag     string `xml:"BrowseFlag"`
	Filter         string `xml:"Filter"`
	StartingIndex  int    `xml:"StartingIndex"`
	RequestedCount int    `xml:"RequestedCount"`
	SortCriteria   string `xml:"SortCriteria"`
}

type GetSearchCapabilitiesRequest struct{}
type GetSortCapabilitiesRequest struct{}
type GetSystemUpdateIDRequest struct{}
type GetProtocolInfoRequest struct{}
type GetCurrentConnectionIDsRequest struct{}
type GetCurrentConnectionInfoRequest struct {
	ConnectionID int `xml:"ConnectionID"`
}

type browseResponseData struct {
	Result         string
	NumberReturned int
	TotalMatches   int
}

func (h *Handler) HandleDummyControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	bodyStr := string(body)

	if strings.Contains(r.URL.Path, "/content/") {
		h.handleContentDirectoryAction(w, r, bodyStr)
		return
	}

	if strings.Contains(r.URL.Path, "/connection/") {
		h.handleConnectionManagerAction(w, r, bodyStr)
		return
	}

	http.Error(w, "Unknown service", http.StatusNotImplemented)
}

func (h *Handler) handleContentDirectoryAction(w http.ResponseWriter, r *http.Request, bodyStr string) {
	var envelope SOAPEnvelope
	if err := xml.Unmarshal([]byte(bodyStr), &envelope); err != nil {
		h.logger.Error("failed to parse SOAP", "err", err)
		http.Error(w, "Invalid SOAP request", http.StatusBadRequest)
		return
	}

	if envelope.Body.Browse != nil {
		h.handleBrowse(w, r, envelope.Body.Browse)
		return
	}

	if envelope.Body.GetSearchCapabilities != nil {
		h.handleGetSearchCapabilities(w)
		return
	}

	if envelope.Body.GetSortCapabilities != nil {
		h.handleGetSortCapabilities(w)
		return
	}

	if envelope.Body.GetSystemUpdateID != nil {
		h.handleGetSystemUpdateID(w)
		return
	}

	http.Error(w, "Unknown action", http.StatusNotImplemented)
}

func (h *Handler) handleConnectionManagerAction(w http.ResponseWriter, _ *http.Request, bodyStr string) {
	var envelope SOAPEnvelope
	if err := xml.Unmarshal([]byte(bodyStr), &envelope); err != nil {
		http.Error(w, "Invalid SOAP request", http.StatusBadRequest)
		return
	}

	if envelope.Body.GetProtocolInfo != nil {
		h.handleGetProtocolInfo(w)
		return
	}

	if envelope.Body.GetCurrentConnectionIDs != nil {
		h.handleGetCurrentConnectionIDs(w)
		return
	}

	if envelope.Body.GetCurrentConnectionInfo != nil {
		h.handleGetCurrentConnectionInfo(w)
		return
	}

	http.Error(w, "unknown action", http.StatusNotImplemented)
}
func (h *Handler) handleBrowse(w http.ResponseWriter, r *http.Request, browse *BrowseRequest) {
	allFiles, err := h.Media.ListFiles()
	if err != nil {
		http.Error(w, "failed to list files", http.StatusInternalServerError)
		return
	}

	startIndex := browse.StartingIndex
	requestedCount := browse.RequestedCount

	if requestedCount == 0 {
		requestedCount = len(allFiles)
	}

	endIndex := min(startIndex+requestedCount, len(allFiles))
	startIndex = min(startIndex, len(allFiles))

	mediaFiles := allFiles[startIndex:endIndex]

	didl := h.generateDIDL(mediaFiles, r.Host)
	escapedDIDL := escapeXML(didl)

	h.logger.Debug("browse returned", "returned", len(mediaFiles), "total", len(allFiles), "remote", r.RemoteAddr)

	data := browseResponseData{
		Result:         escapedDIDL,
		NumberReturned: len(mediaFiles),
		TotalMatches:   len(allFiles),
	}
	h.render(w, "browse_response.xml", data)
}

func (h *Handler) handleGetSearchCapabilities(w http.ResponseWriter) {
	h.render(w, "search_caps.xml", nil)
}

func (h *Handler) handleGetSortCapabilities(w http.ResponseWriter) {
	h.render(w, "sort_caps.xml", nil)
}

func (h *Handler) handleGetSystemUpdateID(w http.ResponseWriter) {
	h.render(w, "system_update_id.xml", nil)
}

func (h *Handler) handleGetProtocolInfo(w http.ResponseWriter) {
	h.render(w, "protocol_info.xml", nil)
}

func (h *Handler) handleGetCurrentConnectionIDs(w http.ResponseWriter) {
	h.render(w, "connection_ids.xml", nil)
}

func (h *Handler) handleGetCurrentConnectionInfo(w http.ResponseWriter) {
	h.render(w, "connection_info.xml", nil)
}

func (h *Handler) generateDIDL(files []media.Video, host string) string {
	var items strings.Builder

	items.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	items.WriteString(`xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	items.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `)
	items.WriteString(`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`)

	for i, file := range files {
		itemID := fmt.Sprintf("%d", i+1)

		// Use simple /direct/ path - cleaner and works better
		pathParts := strings.Split(file.Path, "/")
		for j, part := range pathParts {
			pathParts[j] = url.PathEscape(part)
		}
		encodedPath := strings.Join(pathParts, "/")

		// Use the host from the request - this matches what Nova expects
		streamURL := fmt.Sprintf("http://%s/direct/%s", host, encodedPath)

		fileSize := h.getFileSize(file.Path)
		mimeType := getMimeType(file.Name)

		// Try without any DLNA profile - just basic HTTP
		protocolInfo := fmt.Sprintf("http-get:*:%s:*", mimeType)

		items.WriteString(fmt.Sprintf(`
	<item id="%s" parentID="0" restricted="1">
		<dc:title>%s</dc:title>
		<upnp:class>object.item.videoItem</upnp:class>
		<res protocolInfo="%s" size="%d">%s</res>
	</item>`, itemID, escapeXML(file.Name), protocolInfo, fileSize, escapeXML(streamURL)))
	}

	items.WriteString("\n</DIDL-Lite>")
	return items.String()
}

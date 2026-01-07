package api

import (
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"streamer/internal/media"
	"text/template"
	"time"
)

// Config holds settings specific to the API/DLNA layer
type Config struct {
	FriendlyName string
	UUID         string
}

type Handler struct {
	Media     *media.Manager
	templates map[string]*template.Template
	logger    *slog.Logger
	config    Config
}

//go:embed templates/*
var templateFS embed.FS

func NewHandler(m *media.Manager, cfg Config, logger *slog.Logger) (*Handler, error) {
	tmpls, err := loadTemplates(templateFS)
	if err != nil {
		return nil, err
	}

	// these are the required templates
	required := []string{
		"content_scpd.xml",
		"connection_scpd.xml",
		"device_description.xml",
		"index.html",
		"browse_response.xml",
		"protocol_info.xml",
		"search_caps.xml",
		"sort_caps.xml",
		"system_update_id.xml",
		"connection_ids.xml",
		"connection_info.xml",
	}

	for _, name := range required {
		if _, ok := tmpls[name]; !ok {
			return nil, fmt.Errorf("missing required template: %s", name)
		}
	}

	return &Handler{
		Media:     m,
		templates: tmpls,
		logger:    logger,
		config:    cfg,
	}, nil
}

func (h *Handler) HandleDirectStream(w http.ResponseWriter, r *http.Request, res media.Resource) {
	resName := res.Name()

	// Set DLNA headers
	mimeType := getMimeType(resName)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", getDLNAProfile(resName))

	http.ServeContent(w, r, resName, res.ModTime(), res)
}

func (h *Handler) HandleSCPD(w http.ResponseWriter, r *http.Request) {
	// static xml file so the data argument should be nil
	h.render(w, "content_scpd.xml", nil)
}

func (h *Handler) HandleConnectionSCPD(w http.ResponseWriter, r *http.Request) {
	h.render(w, "connection_scpd.xml", nil)
}

func (h *Handler) HandleXML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Server", "Linux/3.10.0 UPnP/1.0 DLNADOC/1.50 GoStream/1.0")

	data := struct {
		UUID         string
		BaseURL      string
		FriendlyName string
	}{
		UUID:         h.config.UUID,
		BaseURL:      fmt.Sprintf("http://%s", r.Host),
		FriendlyName: h.config.FriendlyName,
	}

	w.Header().Set("EXT", "")
	h.render(w, "device_description.xml", data)
}

func (h *Handler) HandleDummyEvent(w http.ResponseWriter, r *http.Request) {
	// For SUBSCRIBE, return 200 OK with minimal headers
	if r.Method == "SUBSCRIBE" {
		w.Header().Set("SID", "uuid:dummy-subscription-"+h.config.UUID)
		w.Header().Set("TIMEOUT", "Second-1800")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == "UNSUBSCRIBE" {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func loadTemplates(tfs embed.FS) (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)

	entries, err := tfs.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read template dir: %w", err)
	}

	for _, entry := range entries {
		// folders are not needed
		if entry.IsDir() {
			continue
		}

		content, err := tfs.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", entry.Name(), err)
		}

		tmpl := template.Must(template.New(entry.Name()).Parse(string(content)))

		templates[entry.Name()] = tmpl
	}
	return templates, nil
}

func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	tmpl, ok := h.templates[name]
	if !ok {
		// shouldn't get here never happen due to NewHandler checks
		h.logger.Error("template not found", "name", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Automate Content-Type based on the template extension
	var contentType string
	switch filepath.Ext(name) {
	case ".xml":
		contentType = "text/xml; charset=utf-8"
	case ".html":
		contentType = "text/html; charset=utf-8"
		// add new case case for JSON, CSS, etc here
	case ".css":
		contentType = "text/css; charset=utf-8"
	default:
		contentType = "text/plain; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))

	// specific headers for specific files have to be done before calling render or passed in

	err := tmpl.Execute(w, data)
	if err != nil {
		// Note: If Execute fails halfway, the status code 200 is already sentbut it's standdard behavior for streaming templates
		h.logger.Error("error executing template", "name", name, "err", err)
	}
}

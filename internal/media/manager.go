package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
)

// ResourceMode determines how resources are opened
type ResourceMode int

const (
	ModeUnknown ResourceMode = iota
	ModeFileDirect
	ModeFileBuffered
	// more modes here: S3, HTTP, etc
)

type Manager struct {
	RootPath   string
	BufferSize int
	Mode       ResourceMode
	Registry   *Registry
}

type Video struct {
	UUID     uuid.UUID
	Name     string
	Path     string
	Category string
	Size     int64
}

func NewManager(rootPath string, bufferSize int, mode ResourceMode) *Manager {
	return &Manager{
		RootPath:   rootPath,
		BufferSize: bufferSize,
		Mode:       mode,
		Registry:   NewRegistry(),
	}
}

func (m *Manager) GetEntry(uuid uuid.UUID) (*Entry, error) {
	if uuid.IsNil() {
		return nil, errors.New("id cannot be nil")
	}

	return m.Registry.Get(uuid)
}

func (m *Manager) ListFiles() ([]Video, error) {
	entries := m.Registry.List()

	results := make([]Video, 0, len(entries))
	for _, e := range entries {
		results = append(results, Video{
			UUID: e.UUID,
			Name: e.Name,
			// Path:     e.Path, // Note: Frontend shouldn't see this, but helpful for debugging
			Category: e.Category,
			Size:     e.Size,
		})
	}
	return results, nil
}

func (m *Manager) OpenResource(path string) (Resource, error) {
	switch m.Mode {
	case ModeFileDirect:
		return m.openDirectFile(path)
	case ModeFileBuffered:
		return m.openBufferedFile(path)
	default:
		return nil, fmt.Errorf("open resource: %w (mode: %d)", ErrUnsupportedMode, m.Mode)
	}
}

func (m *Manager) openDirectFile(path string) (*FileResource, error) {
	file, err := m.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open direct file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return newFileResource(file, info), nil
}

func (m *Manager) openBufferedFile(path string) (*BufferedFileResource, error) {
	file, err := m.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open buffered file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return newBufferedFileResource(file, info, m.BufferSize), nil
}

func (m *Manager) StartScanning(ctx context.Context, logger *slog.Logger) {
	logger.Info("scanner starting", "path", m.RootPath)

	// run once before ticker ticks
	if err := m.Registry.Scan(m.RootPath); err != nil {
		// proper log an error here!
		logger.Error("could not scan", "path", m.RootPath, "err", err)
	}

	defaultTickerDuration := 5 * time.Minute
	ticker := time.NewTicker(defaultTickerDuration)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.Registry.Scan(m.RootPath); err != nil {
					// proper log an error here!
					logger.Error("could not scan", "path", m.RootPath, "err", err)
				}
			}
		}
	}()
}

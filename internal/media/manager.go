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

// MountPoint represents a Mount Point in a media manager (runtime) context (a specific root folder we need to scan and capture)
type MountPoint struct {
	ID       string
	RootPath string
	Limiter  *IOLimiter
}

type Manager struct {
	BufferSize int
	Mode       ResourceMode
	Registry   *Registry
	Volumes    map[string]*MountPoint // key means volume ID ("vol1", "vol2")
}

type Video struct {
	UUID     uuid.UUID
	Name     string
	Path     string
	Category string
	Size     int64
}

func NewMount(id, rootPath string, maxIO int) *MountPoint {
	return &MountPoint{
		ID:       id,
		RootPath: rootPath,
		Limiter:  NewIOLimiter(maxIO),
	}
}

func NewManager(bufferSize int, mode ResourceMode) *Manager {
	return &Manager{
		BufferSize: bufferSize,
		Mode:       mode,
		Registry:   NewRegistry(),
		Volumes:    make(map[string]*MountPoint),
	}
}

// AddVolume creates the runtime volume and limiter
func (m *Manager) AddMount(id, rootPath string, limiter *IOLimiter) {
	m.Volumes[id] = &MountPoint{
		ID:       id,
		RootPath: rootPath,
		Limiter:  limiter,
	}
}

func (m *Manager) GetMount(volumeID string) (*MountPoint, error) {
	if volumeID == "" {
		return nil, errors.New("volume id is not given")
	}

	vol, ok := m.Volumes[volumeID]
	if !ok {
		return nil, errors.New("cannot find the volume with that id")
	}

	return vol, nil
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

func (m *Manager) OpenResource(entry *Entry) (Resource, error) {
	vol, ok := m.Volumes[entry.MountID]
	if !ok {
		return nil, fmt.Errorf("volume %q not found", entry.MountID)
	}

	switch m.Mode {
	case ModeFileDirect:
		return m.openDirectFile(vol.RootPath, entry.Path)
	case ModeFileBuffered:
		return m.openBufferedFile(vol.RootPath, entry.Path)
	default:
		return nil, fmt.Errorf("open resource: %w (mode: %d)", ErrUnsupportedMode, m.Mode)
	}
}

func (m *Manager) openDirectFile(rootPath, path string) (*FileResource, error) {
	file, err := m.OpenFile(rootPath, path)
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

func (m *Manager) openBufferedFile(rootPath, path string) (*BufferedFileResource, error) {
	file, err := m.OpenFile(rootPath, path)
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
	scanAll := func() {
		// Iterate over all logical volumes
		for _, vol := range m.Volumes {
			if err := m.Registry.Scan(vol.ID, vol.RootPath); err != nil {
				logger.Error("scan failed", "vol_id", vol.ID, "path", vol.RootPath, "err", err)
			}
		}
	}

	go func() {
		logger.Info("background scanner started")
		scanAll()

		defaultTickerDuration := 5 * time.Minute
		ticker := time.NewTicker(defaultTickerDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scanAll()
			}
		}
	}()
}

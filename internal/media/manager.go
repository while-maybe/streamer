package media

import (
	"fmt"
	"io/fs"
	"path/filepath"
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
}

type Video struct {
	Name     string
	Path     string
	Category string
}

func (m *Manager) ListFiles() ([]Video, error) {
	var results []Video

	err := filepath.WalkDir(m.RootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("list files: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(m.RootPath, path)
		if err != nil {
			return fmt.Errorf("list files: get relative path: %w", err)
		}

		relPath = filepath.ToSlash(relPath)

		category := filepath.Dir(relPath)
		if category == "." {
			category = "uncategorized"
		}

		results = append(results, Video{
			Name:     d.Name(),
			Path:     relPath,
			Category: category,
		})
		return nil
	})
	return results, err
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

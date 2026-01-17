package media

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gofrs/uuid/v5"
)

type Entry struct {
	UUID     uuid.UUID
	VolumeID string
	Path     string
	Name     string
	Category string
	Size     int64
	// CachedChunks map[int][]byte
}

type Registry struct {
	mu      sync.RWMutex
	byUUID  map[uuid.UUID]*Entry // lookup UUID -> *Entry
	byPath  map[string]uuid.UUID // lookup Path -> UUID
	updates chan registryUpdate
}

func NewRegistry() *Registry {
	return &Registry{
		byUUID:  make(map[uuid.UUID]*Entry),
		byPath:  make(map[string]uuid.UUID),
		updates: make(chan registryUpdate, 1),
	}
}

func NewEntry(volID, path, name, category string, size int64) (*Entry, error) {
	if volID == "" || path == "" || name == "" || size == 0 {
		return nil, errors.New("an entry needs a path, name and size")
	}
	// create a new uuid otherwise
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	return &Entry{
		UUID:     id,
		VolumeID: volID,
		Path:     path,
		Name:     name,
		Category: category,
		Size:     size,
		// CachedChunks: make(map[int][]byte, 0),
	}, nil
}

func (r *Registry) Get(uuid uuid.UUID) (*Entry, error) {
	if uuid.IsNil() {
		return nil, errors.New("id cannot be nil")
	}

	r.mu.RLock()
	entry, ok := r.byUUID[uuid]
	r.mu.RUnlock()

	if !ok {
		return nil, errors.New("no such entry")
	}

	return entry, nil
}

func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]Entry, 0, len(r.byUUID))

	for _, e := range r.byUUID {
		entries = append(entries, *e)
	}

	// previously we ranged through a map so need to sort here for predictable order
	slices.SortFunc(entries, func(a, b Entry) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})
	return entries

}

func (r *Registry) Add(e *Entry) {
	if e == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.byUUID[e.UUID] = e
	r.byPath[e.Path] = e.UUID
}

func (r *Registry) Remove(path string) {
	if path == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	uuid, ok := r.byPath[path]
	if !ok {
		// does not exist
		return
	}
	delete(r.byPath, path)
	delete(r.byUUID, uuid)
}

type registryUpdate struct {
	toAdd    []*Entry
	toRemove []string
}

func (r *Registry) Scan(volID, rootPath string) error {

	type fileMetadata struct {
		path, name, category string
		size                 int64
	}

	meta := make(map[string]fileMetadata)
	allowedExtensions := []string{".mp4", ".m4v"}

	err := fs.WalkDir(os.DirFS(rootPath), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(allowedExtensions, ext) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		category := filepath.Dir(path)
		if category == "." {
			category = "Uncategorized"
		}

		// Store RAW data. Don't create Entry yet.
		meta[path] = fileMetadata{
			path:     path,
			name:     d.Name(),
			category: category,
			size:     info.Size(),
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("walkdir: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for deletions
	for uuid, entry := range r.byUUID {

		// check if on the right volume
		if entry.VolumeID != volID {
			continue
		}
		if _, ok := meta[entry.Path]; !ok {
			delete(r.byPath, entry.Path)
			delete(r.byUUID, uuid)
		}
	}

	// Check for additions / updates
	for path, fileMeta := range meta {

		// check if the path exists
		if existingUUID, ok := r.byPath[path]; !ok {

			// check if on the right volume
			if r.byUUID[existingUUID].VolumeID != volID {
				continue
			}

			entry, err := NewEntry(volID, fileMeta.path, fileMeta.name, fileMeta.category, fileMeta.size)
			if err != nil {
				continue
			}

			r.byUUID[entry.UUID] = entry
			r.byPath[entry.Path] = entry.UUID
		}
	}

	return nil
}

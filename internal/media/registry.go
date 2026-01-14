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
	Path     string
	Name     string
	Category string
	Size     int64
	// CachedChunks map[int][]byte
}

type Registry struct {
	mu     sync.RWMutex
	byUUID map[uuid.UUID]*Entry // lookup UUID -> *Entry
	byPath map[string]uuid.UUID // lookup Path -> UUID
}

func NewRegistry() *Registry {
	return &Registry{
		byUUID: make(map[uuid.UUID]*Entry),
		byPath: make(map[string]uuid.UUID),
	}
}

func NewEntry(path, name, category string, size int64) (*Entry, error) {
	if path == "" || name == "" || size == 0 {
		return nil, errors.New("an entry needs a path, name and size")
	}
	// create a new uuid otherwise
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}

	name = strings.TrimSuffix(name, filepath.Ext(name))

	return &Entry{
		UUID:     id,
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

func (r *Registry) Scan(rootPath string) error {
	foundOnDisk := make(map[string]struct{})
	allowedExtensions := []string{".mp4", ".m4v"}

	err := fs.WalkDir(os.DirFS(rootPath), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// for now skip
			// maybe transient IO error?
			return nil
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !slices.Contains(allowedExtensions, ext) {
			// Skip this file, it's not a format we want for now
			return nil
		}

		// mark the file as ok
		foundOnDisk[path] = struct{}{}

		r.mu.RLock()
		if _, ok := r.byPath[path]; ok {
			return nil
		}
		r.mu.RUnlock()

		// create a new entry
		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Calculate Category (Parent folder)
		category := filepath.Dir(path)
		if category == "." {
			category = "Uncategorized"
		}

		newEntry, err := NewEntry(path, d.Name(), category, info.Size())
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}

		// add it to cache
		r.Add(newEntry)
		return nil
	})

	if err != nil {
		return fmt.Errorf("walkdir: %w", err)
	}

	// gather entries where files are not accessible but entries still in cache
	var pathsToDelete []string
	r.mu.RLock()
	for path := range r.byPath {
		if _, ok := foundOnDisk[path]; !ok {
			pathsToDelete = append(pathsToDelete, path)
		}
	}
	r.mu.RUnlock()

	// remove them
	for _, k := range pathsToDelete {
		r.Remove(k)
	}

	return nil
}

package media

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

func (m *Manager) OpenFile(relPath string) (*os.File, error) {
	f, err := os.OpenInRoot(m.RootPath, relPath)
	if err != nil {
		switch {
		// os.OpenInRoot returns fs.ErrInvalid if the path escapes the root
		case errors.Is(err, fs.ErrInvalid):
			// to avoid error msg stuttering, jus wrap the original error here
			return nil, fmt.Errorf("%w (%w)", ErrPathOutsideRoot, err)
		// case errors.Is(err, fs.ErrPermission):
		// 	// can be differentiated if needed
		// 	return nil, fmt.Errorf("open file: %w (%w)", ErrPathOutsideRoot, err)
		default:
			return nil, err
		}
	}
	return f, nil
}

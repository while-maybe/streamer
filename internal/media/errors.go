package media

import "errors"

var (
	ErrUnsupportedMode = errors.New("unsupported resource mode")
	ErrPathOutsideRoot = errors.New("path outside root directory")
)

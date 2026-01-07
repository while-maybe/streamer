package media

import (
	"io"
	"time"
)

// Resource represents a streamable media resource with metadata. Implementations can source from filesystem, S3, HTTP, etc.
type Resource interface {
	io.ReadSeekCloser
	Name() string
	ModTime() time.Time
	Size() int64
}

// ensure interface satisfaction
var (
	_ Resource = (*FileResource)(nil)
	_ Resource = (*BufferedFileResource)(nil)
)

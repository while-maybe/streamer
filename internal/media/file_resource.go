package media

import (
	"os"
	"time"
)

// FileResource provides direct file access without buffering
type FileResource struct {
	file *os.File
	info os.FileInfo
}

func newFileResource(file *os.File, info os.FileInfo) *FileResource {
	return &FileResource{
		file: file,
		info: info,
	}
}

func (f *FileResource) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *FileResource) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *FileResource) Close() error {
	return f.file.Close()
}

// satisfy the media Resource interface
func (f *FileResource) Name() string       { return f.info.Name() }
func (f *FileResource) ModTime() time.Time { return f.info.ModTime() }
func (f *FileResource) Size() int64        { return f.info.Size() }

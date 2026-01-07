package media

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

type BufferedFileResource struct {
	file   *os.File
	reader *bufio.Reader
	info   os.FileInfo
}

func newBufferedFileResource(file *os.File, info os.FileInfo, bufferSize int) *BufferedFileResource {
	return &BufferedFileResource{
		file:   file,
		reader: bufio.NewReaderSize(file, bufferSize),
		info:   info,
	}
}

func (b *BufferedFileResource) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *BufferedFileResource) Seek(offset int64, whence int) (int64, error) {
	fileName := b.Name()

	// Optimization: Getting current position (ftell) should NOT nuke the buffer
	if whence == io.SeekCurrent && offset == 0 {
		curPos, err := b.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, fmt.Errorf("seek buffer %q: %w", fileName, err)
		}
		return curPos - int64(b.reader.Buffered()), nil
	}

	// Optimization: Forward seek within the current buffer
	if whence == io.SeekCurrent && offset > 0 && offset <= int64(b.reader.Buffered()) {
		_, err := b.reader.Discard(int(offset))
		if err != nil {
			return 0, err // Should technically not happen if Buffered() is correct
		}
		// Return the new logical position
		curPos, err := b.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, fmt.Errorf("seek buffer %q: %w", fileName, err)
		}
		return curPos - int64(b.reader.Buffered()), err
	}

	// For relative seeks, account for buffered data
	if whence == io.SeekCurrent {
		// Adjust offset by how much data is still in the buffer
		// (file is ahead of where the reader appears to be)
		offset -= int64(b.reader.Buffered())
	}

	newPos, err := b.file.Seek(offset, whence)
	if err != nil {
		return 0, fmt.Errorf("seek buffer %q: %w", fileName, err)
	}

	// Seeking invalidates buffer
	b.reader.Reset(b.file)
	return newPos, nil
}

func (b *BufferedFileResource) Close() error {
	return b.file.Close()
}

func (b *BufferedFileResource) Name() string       { return b.info.Name() }
func (b *BufferedFileResource) ModTime() time.Time { return b.info.ModTime() }
func (b *BufferedFileResource) Size() int64        { return b.info.Size() }

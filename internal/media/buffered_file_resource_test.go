// internal/media/buffered_file_resource_test.go
package media

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestBufferedFileResourceSeek(t *testing.T) {
	// Create temp file with known content
	content := []byte("0123456789ABCDEFGHIJ") // 20 bytes
	tmpFile, err := os.CreateTemp("", "test-*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatal(err)
	}

	// Reopen for reading
	tmpFile.Close()
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	info, _ := file.Stat()

	// Create buffered resource with small buffer (5 bytes)
	br := newBufferedFileResource(file, info, 5)
	defer br.Close()

	// Read first 3 bytes - fills buffer with "01234"
	buf := make([]byte, 3)
	n, err := br.Read(buf)
	if err != nil || n != 3 {
		t.Fatalf("First read failed: %v, %d bytes", err, n)
	}
	if !bytes.Equal(buf, []byte("012")) {
		t.Errorf("Expected '012', got '%s'", buf)
	}

	// Seek to position 10 (should be 'A')
	newPos, err := br.Seek(10, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if newPos != 10 {
		t.Errorf("Expected position 10, got %d", newPos)
	}

	// Read after seek - should get 'A', NOT stale buffer data
	buf = make([]byte, 3)
	n, err = br.Read(buf)
	if err != nil || n != 3 {
		t.Fatalf("Read after seek failed: %v, %d bytes", err, n)
	}

	if !bytes.Equal(buf, []byte("ABC")) {
		t.Errorf("❌ SEEK BUG! Expected 'ABC' from position 10, got '%s'", buf)
		t.Errorf("This means buffer wasn't properly reset after seek")
	}

	// Seek backwards to position 5 (should be '5')
	newPos, err = br.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek backwards failed: %v", err)
	}

	buf = make([]byte, 3)
	n, err = br.Read(buf)
	if err != nil || n != 3 {
		t.Fatalf("Read after backwards seek failed: %v", err)
	}

	if !bytes.Equal(buf, []byte("567")) {
		t.Errorf("❌ SEEK BUG! Expected '567' from position 5, got '%s'", buf)
	}
}

func TestBufferedFileResourceSeekRelative(t *testing.T) {
	content := []byte("0123456789ABCDEFGHIJ")
	tmpFile, err := os.CreateTemp("", "test-*.dat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(content)
	tmpFile.Close()

	file, _ := os.Open(tmpFile.Name())
	defer file.Close()
	info, _ := file.Stat()

	br := newBufferedFileResource(file, info, 5)
	defer br.Close()

	// Read to position 5
	buf := make([]byte, 5)
	br.Read(buf)

	// Seek forward 5 bytes from current position (should be at 10 = 'A')
	newPos, err := br.Seek(5, io.SeekCurrent)
	if err != nil || newPos != 10 {
		t.Fatalf("Relative seek failed: %v, pos=%d", err, newPos)
	}

	buf = make([]byte, 1)
	br.Read(buf)

	if buf[0] != 'A' {
		t.Errorf("Expected 'A' after relative seek, got '%c'", buf[0])
	}
}

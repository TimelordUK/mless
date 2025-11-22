package io

import (
	"os"

	"golang.org/x/exp/mmap"
)

// MappedFile provides memory-mapped read access to a file
type MappedFile struct {
	reader *mmap.ReaderAt
	size   int64
	path   string
}

// OpenMapped opens a file with memory mapping
func OpenMapped(path string) (*MappedFile, error) {
	reader, err := mmap.Open(path)
	if err != nil {
		return nil, err
	}

	// Get file size
	info, err := os.Stat(path)
	if err != nil {
		reader.Close()
		return nil, err
	}

	return &MappedFile{
		reader: reader,
		size:   info.Size(),
		path:   path,
	}, nil
}

// ReadAt reads len(p) bytes at offset
func (m *MappedFile) ReadAt(p []byte, off int64) (int, error) {
	return m.reader.ReadAt(p, off)
}

// Size returns the file size
func (m *MappedFile) Size() int64 {
	return m.size
}

// Path returns the file path
func (m *MappedFile) Path() string {
	return m.path
}

// Close closes the memory mapping
func (m *MappedFile) Close() error {
	return m.reader.Close()
}

// Refresh re-opens the file if it has grown, returns true if size changed
func (m *MappedFile) Refresh() (bool, error) {
	info, err := os.Stat(m.path)
	if err != nil {
		return false, err
	}

	newSize := info.Size()
	if newSize <= m.size {
		return false, nil
	}

	// File has grown, re-open it
	m.reader.Close()

	reader, err := mmap.Open(m.path)
	if err != nil {
		return false, err
	}

	m.reader = reader
	m.size = newSize
	return true, nil
}

// PreviousSize returns the size before last refresh (for incremental indexing)
func (m *MappedFile) PreviousSize() int64 {
	return m.size
}

// ReadRange reads bytes from start to end
func (m *MappedFile) ReadRange(start, end int64) ([]byte, error) {
	if end > m.size {
		end = m.size
	}
	if start >= end {
		return nil, nil
	}

	buf := make([]byte, end-start)
	_, err := m.reader.ReadAt(buf, start)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

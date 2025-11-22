package index

import (
	"bytes"

	mlessio "github.com/user/mless/internal/io"
)

// LineIndex stores byte offsets for each line in a file
type LineIndex struct {
	offsets []int64 // byte offset of each line start
	file    *mlessio.MappedFile
}

// BuildLineIndex scans the file and builds a line offset index
func BuildLineIndex(file *mlessio.MappedFile) (*LineIndex, error) {
	size := file.Size()
	if size == 0 {
		return &LineIndex{
			offsets: []int64{0},
			file:    file,
		}, nil
	}

	// Estimate initial capacity (assume ~100 bytes per line)
	estimatedLines := int(size/100) + 1
	offsets := make([]int64, 0, estimatedLines)
	offsets = append(offsets, 0) // First line starts at 0

	// Read in chunks to find newlines
	const chunkSize = 64 * 1024 // 64KB chunks
	buf := make([]byte, chunkSize)

	var pos int64
	for pos < size {
		readSize := chunkSize
		if pos+int64(readSize) > size {
			readSize = int(size - pos)
		}

		n, err := file.ReadAt(buf[:readSize], pos)
		if err != nil {
			return nil, err
		}

		// Find all newlines in this chunk
		chunk := buf[:n]
		offset := 0
		for {
			idx := bytes.IndexByte(chunk[offset:], '\n')
			if idx == -1 {
				break
			}
			lineStart := pos + int64(offset) + int64(idx) + 1
			if lineStart < size {
				offsets = append(offsets, lineStart)
			}
			offset += idx + 1
		}

		pos += int64(n)
	}

	return &LineIndex{
		offsets: offsets,
		file:    file,
	}, nil
}

// LineCount returns the total number of lines
func (idx *LineIndex) LineCount() int {
	return len(idx.offsets)
}

// GetLine returns the content of line at given index (0-based)
func (idx *LineIndex) GetLine(lineNum int) ([]byte, error) {
	if lineNum < 0 || lineNum >= len(idx.offsets) {
		return nil, nil
	}

	start := idx.offsets[lineNum]
	var end int64
	if lineNum+1 < len(idx.offsets) {
		end = idx.offsets[lineNum+1]
	} else {
		end = idx.file.Size()
	}

	content, err := idx.file.ReadRange(start, end)
	if err != nil {
		return nil, err
	}

	// Trim trailing newline
	content = bytes.TrimRight(content, "\r\n")
	return content, nil
}

// GetLines returns a range of lines efficiently
func (idx *LineIndex) GetLines(start, count int) ([][]byte, error) {
	if start < 0 {
		start = 0
	}
	if start >= len(idx.offsets) {
		return nil, nil
	}
	if start+count > len(idx.offsets) {
		count = len(idx.offsets) - start
	}

	lines := make([][]byte, count)
	for i := 0; i < count; i++ {
		line, err := idx.GetLine(start + i)
		if err != nil {
			return nil, err
		}
		lines[i] = line
	}
	return lines, nil
}

// ByteOffset returns the byte offset of a line
func (idx *LineIndex) ByteOffset(lineNum int) int64 {
	if lineNum < 0 || lineNum >= len(idx.offsets) {
		return -1
	}
	return idx.offsets[lineNum]
}

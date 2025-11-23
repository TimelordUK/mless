package index

import (
	"bytes"
	"time"

	mlessio "github.com/TimelordUK/mless/internal/io"
	"github.com/TimelordUK/mless/pkg/logformat"
)

// LineIndex stores byte offsets for each line in a file
type LineIndex struct {
	offsets    []int64      // byte offset of each line start
	timestamps []*time.Time // parsed timestamp for each line (nil if not parsed)
	file       *mlessio.MappedFile
	tsParser   *logformat.TimestampParser
}

// BuildLineIndex scans the file and builds a line offset index
func BuildLineIndex(file *mlessio.MappedFile) (*LineIndex, error) {
	size := file.Size()
	if size == 0 {
		return &LineIndex{
			offsets:  []int64{0},
			file:     file,
			tsParser: logformat.NewTimestampParser(),
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
		offsets:  offsets,
		file:     file,
		tsParser: logformat.NewTimestampParser(),
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

// GetTimestamp returns the parsed timestamp for a line (lazy parsing)
func (idx *LineIndex) GetTimestamp(lineNum int) *time.Time {
	if lineNum < 0 || lineNum >= len(idx.offsets) {
		return nil
	}

	// Initialize timestamps slice if needed
	if idx.timestamps == nil {
		idx.timestamps = make([]*time.Time, len(idx.offsets))
	}

	// Extend if needed (for appended lines)
	for len(idx.timestamps) < len(idx.offsets) {
		idx.timestamps = append(idx.timestamps, nil)
	}

	// Return cached timestamp if already parsed
	if idx.timestamps[lineNum] != nil {
		return idx.timestamps[lineNum]
	}

	// Parse timestamp from line content
	content, err := idx.GetLine(lineNum)
	if err != nil || content == nil {
		return nil
	}

	ts := idx.tsParser.Parse(content)
	idx.timestamps[lineNum] = ts
	return ts
}

// FindLineAtTime finds the first line at or after the given time
// Returns -1 if no such line exists
func (idx *LineIndex) FindLineAtTime(target time.Time) int {
	// Binary search would be better for large files, but for now linear scan
	// from the end since we often look for recent times
	for i := 0; i < len(idx.offsets); i++ {
		ts := idx.GetTimestamp(i)
		if ts != nil && !ts.Before(target) {
			return i
		}
	}
	return -1
}

// FindLineBeforeTime finds the last line before the given time
func (idx *LineIndex) FindLineBeforeTime(target time.Time) int {
	lastBefore := -1
	for i := 0; i < len(idx.offsets); i++ {
		ts := idx.GetTimestamp(i)
		if ts != nil {
			if ts.Before(target) {
				lastBefore = i
			} else {
				break
			}
		}
	}
	return lastBefore
}

// FindNearestLineAtTime finds the line with timestamp closest to the given time
func (idx *LineIndex) FindNearestLineAtTime(target time.Time) int {
	lineAfter := idx.FindLineAtTime(target)
	lineBefore := idx.FindLineBeforeTime(target)

	// If only one exists, return it
	if lineAfter < 0 && lineBefore < 0 {
		return -1
	}
	if lineAfter < 0 {
		return lineBefore
	}
	if lineBefore < 0 {
		return lineAfter
	}

	// Compare distances
	tsAfter := idx.GetTimestamp(lineAfter)
	tsBefore := idx.GetTimestamp(lineBefore)
	if tsAfter == nil {
		return lineBefore
	}
	if tsBefore == nil {
		return lineAfter
	}

	diffAfter := tsAfter.Sub(target)
	diffBefore := target.Sub(*tsBefore)

	if diffBefore <= diffAfter {
		return lineBefore
	}
	return lineAfter
}

// AppendNewLines indexes new content from oldSize to current file size
func (idx *LineIndex) AppendNewLines(oldSize int64) error {
	size := idx.file.Size()
	if size <= oldSize {
		return nil
	}

	// Check if the old content ended with a newline
	// If so, oldSize is the start of a new line
	if oldSize > 0 {
		lastByte := make([]byte, 1)
		_, err := idx.file.ReadAt(lastByte, oldSize-1)
		if err != nil {
			return err
		}
		if lastByte[0] == '\n' {
			// Previous content ended with newline, so oldSize is start of new line
			idx.offsets = append(idx.offsets, oldSize)
		}
	} else if oldSize == 0 && len(idx.offsets) == 0 {
		// Empty file getting first content
		idx.offsets = append(idx.offsets, 0)
	}

	// Read in chunks from oldSize to end to find more newlines
	const chunkSize = 64 * 1024
	buf := make([]byte, chunkSize)

	pos := oldSize
	for pos < size {
		readSize := chunkSize
		if pos+int64(readSize) > size {
			readSize = int(size - pos)
		}

		n, err := idx.file.ReadAt(buf[:readSize], pos)
		if err != nil {
			return err
		}

		// Find all newlines in this chunk
		chunk := buf[:n]
		offset := 0
		for {
			i := bytes.IndexByte(chunk[offset:], '\n')
			if i == -1 {
				break
			}
			lineStart := pos + int64(offset) + int64(i) + 1
			if lineStart < size {
				idx.offsets = append(idx.offsets, lineStart)
			}
			offset += i + 1
		}

		pos += int64(n)
	}

	return nil
}

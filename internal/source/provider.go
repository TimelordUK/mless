package source

import "time"

// LogLevel represents a log severity level
type LogLevel int

const (
	LevelUnknown LogLevel = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// SourceInfo identifies where a line came from (for merged views)
type SourceInfo struct {
	Path  string
	Index int // which source in a merged view
}

// Line represents a single line with optional metadata
type Line struct {
	Content   []byte
	Timestamp *time.Time
	Level     LogLevel
	Source    *SourceInfo
	OriginalIndex int // line number in original file
}

// LineProvider is the core abstraction for accessing lines
// The viewport only interacts with this interface
type LineProvider interface {
	// LineCount returns total number of lines
	LineCount() int

	// GetLine returns line at index (0-based)
	GetLine(index int) (*Line, error)

	// GetLines returns a range of lines efficiently
	GetLines(start, count int) ([]*Line, error)
}

// FilePosition represents a position in a source file
type FilePosition struct {
	Path       string
	LineNumber int
	ByteOffset int64
}

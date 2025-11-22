package source

import (
	"time"

	"github.com/user/mless/pkg/logformat"
)

// Re-export LogLevel types for convenience
type LogLevel = logformat.LogLevel

const (
	LevelUnknown = logformat.LevelUnknown
	LevelTrace   = logformat.LevelTrace
	LevelDebug   = logformat.LevelDebug
	LevelInfo    = logformat.LevelInfo
	LevelWarn    = logformat.LevelWarn
	LevelError   = logformat.LevelError
	LevelFatal   = logformat.LevelFatal
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

package logformat

import (
	"strings"
	"unicode"

	"github.com/TimelordUK/mless/internal/config"
)

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

// LevelDetector detects log levels from line content
type LevelDetector struct {
	patterns map[LogLevel][]string
}

// NewLevelDetector creates a detector from config
func NewLevelDetector(cfg *config.LogLevelConfig) *LevelDetector {
	return &LevelDetector{
		patterns: map[LogLevel][]string{
			LevelTrace: cfg.TracePatterns,
			LevelDebug: cfg.DebugPatterns,
			LevelInfo:  cfg.InfoPatterns,
			LevelWarn:  cfg.WarnPatterns,
			LevelError: cfg.ErrorPatterns,
			LevelFatal: cfg.FatalPatterns,
		},
	}
}

// Detect returns the log level for a line
func (d *LevelDetector) Detect(content []byte) LogLevel {
	line := string(content)

	// Only look at the prefix of the line (first 150 chars) for level detection
	// Log levels typically appear near the start, after timestamp
	prefix := line
	if len(prefix) > 150 {
		prefix = prefix[:150]
	}

	// Check in order of severity (most specific first)
	if d.matchLevel(prefix, LevelFatal) {
		return LevelFatal
	}
	if d.matchLevel(prefix, LevelError) {
		return LevelError
	}
	if d.matchLevel(prefix, LevelWarn) {
		return LevelWarn
	}
	if d.matchLevel(prefix, LevelInfo) {
		return LevelInfo
	}
	if d.matchLevel(prefix, LevelDebug) {
		return LevelDebug
	}
	if d.matchLevel(prefix, LevelTrace) {
		return LevelTrace
	}

	return LevelUnknown
}

// matchLevel checks if any pattern for the level matches in the prefix
func (d *LevelDetector) matchLevel(prefix string, level LogLevel) bool {
	for _, pattern := range d.patterns[level] {
		if matchPattern(prefix, pattern) {
			return true
		}
	}
	return false
}

// matchPattern checks if a pattern matches with appropriate boundaries
// Bracketed patterns like [ERROR] match anywhere
// Bare patterns like ERROR require word boundaries
func matchPattern(text, pattern string) bool {
	// Bracketed patterns are precise - match anywhere
	if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
		return strings.Contains(text, pattern)
	}

	// For bare patterns, require word boundaries
	idx := strings.Index(text, pattern)
	if idx == -1 {
		return false
	}

	// Check character before pattern (should be non-alphanumeric or start of string)
	if idx > 0 {
		before := rune(text[idx-1])
		if unicode.IsLetter(before) || unicode.IsDigit(before) || before == '_' {
			return false
		}
	}

	// Check character after pattern (should be non-alphanumeric or end of string)
	endIdx := idx + len(pattern)
	if endIdx < len(text) {
		after := rune(text[endIdx])
		if unicode.IsLetter(after) || unicode.IsDigit(after) || after == '_' {
			return false
		}
	}

	return true
}

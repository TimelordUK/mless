package logformat

import (
	"strings"

	"github.com/user/mless/internal/config"
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

	// Check in order of severity (most specific first)
	// Check fatal first as it's most important to identify
	for _, pattern := range d.patterns[LevelFatal] {
		if strings.Contains(line, pattern) {
			return LevelFatal
		}
	}

	for _, pattern := range d.patterns[LevelError] {
		if strings.Contains(line, pattern) {
			return LevelError
		}
	}

	for _, pattern := range d.patterns[LevelWarn] {
		if strings.Contains(line, pattern) {
			return LevelWarn
		}
	}

	for _, pattern := range d.patterns[LevelInfo] {
		if strings.Contains(line, pattern) {
			return LevelInfo
		}
	}

	for _, pattern := range d.patterns[LevelDebug] {
		if strings.Contains(line, pattern) {
			return LevelDebug
		}
	}

	for _, pattern := range d.patterns[LevelTrace] {
		if strings.Contains(line, pattern) {
			return LevelTrace
		}
	}

	return LevelUnknown
}

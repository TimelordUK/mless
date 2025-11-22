package logformat

import (
	"strings"

	"github.com/user/mless/internal/config"
	"github.com/user/mless/internal/source"
)

// LevelDetector detects log levels from line content
type LevelDetector struct {
	patterns map[source.LogLevel][]string
}

// NewLevelDetector creates a detector from config
func NewLevelDetector(cfg *config.LogLevelConfig) *LevelDetector {
	return &LevelDetector{
		patterns: map[source.LogLevel][]string{
			source.LevelTrace: cfg.TracePatterns,
			source.LevelDebug: cfg.DebugPatterns,
			source.LevelInfo:  cfg.InfoPatterns,
			source.LevelWarn:  cfg.WarnPatterns,
			source.LevelError: cfg.ErrorPatterns,
			source.LevelFatal: cfg.FatalPatterns,
		},
	}
}

// Detect returns the log level for a line
func (d *LevelDetector) Detect(content []byte) source.LogLevel {
	line := string(content)

	// Check in order of severity (most specific first)
	// Check fatal first as it's most important to identify
	for _, pattern := range d.patterns[source.LevelFatal] {
		if strings.Contains(line, pattern) {
			return source.LevelFatal
		}
	}

	for _, pattern := range d.patterns[source.LevelError] {
		if strings.Contains(line, pattern) {
			return source.LevelError
		}
	}

	for _, pattern := range d.patterns[source.LevelWarn] {
		if strings.Contains(line, pattern) {
			return source.LevelWarn
		}
	}

	for _, pattern := range d.patterns[source.LevelInfo] {
		if strings.Contains(line, pattern) {
			return source.LevelInfo
		}
	}

	for _, pattern := range d.patterns[source.LevelDebug] {
		if strings.Contains(line, pattern) {
			return source.LevelDebug
		}
	}

	for _, pattern := range d.patterns[source.LevelTrace] {
		if strings.Contains(line, pattern) {
			return source.LevelTrace
		}
	}

	return source.LevelUnknown
}

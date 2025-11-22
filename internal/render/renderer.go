package render

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/config"
	"github.com/user/mless/internal/source"
	"github.com/user/mless/pkg/logformat"
)

// Renderer applies styling to lines
type Renderer interface {
	Render(line *source.Line) string
}

// LogLevelRenderer colors lines based on log level
type LogLevelRenderer struct {
	detector *logformat.LevelDetector
	styles   map[source.LogLevel]lipgloss.Style
}

// NewLogLevelRenderer creates a renderer with config
func NewLogLevelRenderer(cfg *config.Config) *LogLevelRenderer {
	detector := logformat.NewLevelDetector(&cfg.LogLevels)

	styles := map[source.LogLevel]lipgloss.Style{
		source.LevelUnknown: lipgloss.NewStyle(),
		source.LevelTrace:   lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Trace)),
		source.LevelDebug:   lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Debug)),
		source.LevelInfo:    lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Info)),
		source.LevelWarn:    lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Warn)),
		source.LevelError:   lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Error)),
		source.LevelFatal:   lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Theme.Levels.Fatal)),
	}

	return &LogLevelRenderer{
		detector: detector,
		styles:   styles,
	}
}

// Render applies log level styling to a line
func (r *LogLevelRenderer) Render(line *source.Line) string {
	// Detect level if not already set
	level := line.Level
	if level == source.LevelUnknown {
		level = r.detector.Detect(line.Content)
	}

	style := r.styles[level]
	return style.Render(string(line.Content))
}

// PlainRenderer renders without styling
type PlainRenderer struct{}

// NewPlainRenderer creates a plain renderer
func NewPlainRenderer() *PlainRenderer {
	return &PlainRenderer{}
}

// Render returns the line content as-is
func (r *PlainRenderer) Render(line *source.Line) string {
	return string(line.Content)
}

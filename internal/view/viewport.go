package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/render"
	"github.com/user/mless/internal/source"
)

// Viewport manages the visible portion of content
// It knows nothing about log formats, filters, or file sources
// It only knows how to display lines from a LineProvider
type Viewport struct {
	provider source.LineProvider
	renderer render.Renderer

	// Dimensions
	width  int
	height int

	// Scroll position
	scrollOffset int

	// Styling
	lineNumberStyle lipgloss.Style
	contentStyle    lipgloss.Style
	highlightStyle  lipgloss.Style

	// Options
	showLineNumbers bool
	wrapLines       bool

	// Highlighted line (original index, -1 for none)
	highlightedLine int
}

// NewViewport creates a new viewport
func NewViewport(width, height int) *Viewport {
	return &Viewport{
		width:           width,
		height:          height,
		scrollOffset:    0,
		showLineNumbers: true,
		wrapLines:       false,
		lineNumberStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		contentStyle:    lipgloss.NewStyle(),
		highlightStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true),
		renderer:        render.NewPlainRenderer(),
		highlightedLine: -1,
	}
}

// SetHighlightedLine sets which original line index to highlight (-1 for none)
func (v *Viewport) SetHighlightedLine(originalIndex int) {
	v.highlightedLine = originalIndex
}

// ClearHighlight removes any line highlight
func (v *Viewport) ClearHighlight() {
	v.highlightedLine = -1
}

// SetRenderer sets the line renderer
func (v *Viewport) SetRenderer(r render.Renderer) {
	v.renderer = r
}

// SetProvider sets the line provider
func (v *Viewport) SetProvider(provider source.LineProvider) {
	v.provider = provider
	v.scrollOffset = 0
}

// SetSize updates viewport dimensions
func (v *Viewport) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampScroll()
}

// ScrollDown scrolls down by n lines
func (v *Viewport) ScrollDown(n int) {
	v.scrollOffset += n
	v.clampScroll()
}

// ScrollUp scrolls up by n lines
func (v *Viewport) ScrollUp(n int) {
	v.scrollOffset -= n
	v.clampScroll()
}

// PageDown scrolls down by one page
func (v *Viewport) PageDown() {
	v.ScrollDown(v.height - 1)
}

// PageUp scrolls up by one page
func (v *Viewport) PageUp() {
	v.ScrollUp(v.height - 1)
}

// GotoTop scrolls to the beginning
func (v *Viewport) GotoTop() {
	v.scrollOffset = 0
}

// GotoBottom scrolls to the end
func (v *Viewport) GotoBottom() {
	if v.provider == nil {
		return
	}
	v.scrollOffset = v.provider.LineCount() - v.height
	v.clampScroll()
}

// GotoLine scrolls to a specific line
func (v *Viewport) GotoLine(line int) {
	v.scrollOffset = line
	v.clampScroll()
}

// CurrentLine returns the current top line number
func (v *Viewport) CurrentLine() int {
	return v.scrollOffset
}

// clampScroll ensures scroll offset is within valid bounds
func (v *Viewport) clampScroll() {
	if v.provider == nil {
		v.scrollOffset = 0
		return
	}

	maxScroll := v.provider.LineCount() - v.height
	if maxScroll < 0 {
		maxScroll = 0
	}

	if v.scrollOffset > maxScroll {
		v.scrollOffset = maxScroll
	}
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
}

// Render returns the viewport content as a string
func (v *Viewport) Render() string {
	if v.provider == nil {
		return ""
	}

	lines, err := v.provider.GetLines(v.scrollOffset, v.height)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var builder strings.Builder
	lineCount := v.provider.LineCount()
	lineNumWidth := len(fmt.Sprintf("%d", lineCount))

	for i, line := range lines {
		if i > 0 {
			builder.WriteString("\n")
		}

		// Use original index if available (for filtered views), otherwise use position
		lineNum := v.scrollOffset + i + 1 // 1-based for display
		if line.OriginalIndex > 0 {
			lineNum = line.OriginalIndex + 1
		}

		// Check if this is the highlighted line
		originalIdx := line.OriginalIndex
		if originalIdx == 0 {
			// If OriginalIndex not set, use position-based index
			originalIdx = v.scrollOffset + i
		}
		isHighlighted := v.highlightedLine >= 0 && originalIdx == v.highlightedLine

		if v.showLineNumbers {
			numStr := fmt.Sprintf("%*d ", lineNumWidth, lineNum)
			if isHighlighted {
				// Highlight line number with marker
				builder.WriteString(v.highlightStyle.Render(numStr))
			} else {
				builder.WriteString(v.lineNumberStyle.Render(numStr))
			}
		}

		// Use renderer for content
		content := v.renderer.Render(line)

		// Truncate if needed (note: this is naive with ANSI codes)
		availableWidth := v.width
		if v.showLineNumbers {
			availableWidth -= lineNumWidth + 1
		}

		// For now, just write the styled content
		// TODO: proper truncation with ANSI awareness
		builder.WriteString(content)
	}

	// Pad with empty lines if needed
	for i := len(lines); i < v.height; i++ {
		if i > 0 || len(lines) > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("~")
	}

	return builder.String()
}

// PercentScrolled returns how far through the file we are
func (v *Viewport) PercentScrolled() float64 {
	if v.provider == nil || v.provider.LineCount() == 0 {
		return 0
	}

	total := v.provider.LineCount()
	if total <= v.height {
		return 100
	}

	return float64(v.scrollOffset) / float64(total-v.height) * 100
}

// SetShowLineNumbers toggles line numbers
func (v *Viewport) SetShowLineNumbers(show bool) {
	v.showLineNumbers = show
}

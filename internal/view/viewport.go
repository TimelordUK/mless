package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/TimelordUK/mless/internal/render"
	"github.com/TimelordUK/mless/internal/source"
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

	// Marks (original line number -> mark character)
	marks map[int]rune

	// Horizontal scroll offset
	horizontalOffset int
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

// SetMarks updates the marks to display (original line -> rune)
func (v *Viewport) SetMarks(marks map[int]rune) {
	v.marks = marks
}

// ScrollLeft scrolls horizontally left by n columns
func (v *Viewport) ScrollLeft(n int) {
	v.horizontalOffset -= n
	if v.horizontalOffset < 0 {
		v.horizontalOffset = 0
	}
}

// ScrollRight scrolls horizontally right by n columns
func (v *Viewport) ScrollRight(n int) {
	v.horizontalOffset += n
}

// ResetHorizontalScroll resets horizontal scroll to beginning
func (v *Viewport) ResetHorizontalScroll() {
	v.horizontalOffset = 0
}

// HorizontalOffset returns the current horizontal scroll offset
func (v *Viewport) HorizontalOffset() int {
	return v.horizontalOffset
}

// ToggleWrap toggles line wrapping
func (v *Viewport) ToggleWrap() bool {
	v.wrapLines = !v.wrapLines
	if v.wrapLines {
		v.horizontalOffset = 0 // Reset horizontal scroll when wrapping
	}
	return v.wrapLines
}

// IsWrapping returns whether line wrapping is enabled
func (v *Viewport) IsWrapping() bool {
	return v.wrapLines
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

		// Line number: use OriginalIndex (always set by source/filtered provider)
		lineNum := line.OriginalIndex + 1 // 1-based for display

		// Check if this is the highlighted line
		// Use OriginalIndex if set (> 0 or explicitly 0 for first line)
		// For filtered views, OriginalIndex is always set
		originalIdx := v.scrollOffset + i
		if line.OriginalIndex > 0 || (i == 0 && line.OriginalIndex == 0) {
			originalIdx = line.OriginalIndex
		}
		isHighlighted := v.highlightedLine >= 0 && originalIdx == v.highlightedLine

		if v.showLineNumbers {
			// Check if this line has a mark
			markChar := ' '
			if v.marks != nil {
				if m, ok := v.marks[originalIdx]; ok {
					markChar = m
				}
			}

			numStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)
			if isHighlighted {
				// Highlight line number with marker
				builder.WriteString(v.highlightStyle.Render(fmt.Sprintf("%c%s ", markChar, numStr)))
			} else {
				if markChar != ' ' {
					// Show mark character in highlight style
					builder.WriteString(v.highlightStyle.Render(string(markChar)))
					builder.WriteString(v.lineNumberStyle.Render(fmt.Sprintf("%s ", numStr)))
				} else {
					builder.WriteString(v.lineNumberStyle.Render(fmt.Sprintf(" %s ", numStr)))
				}
			}
		}

		// Use renderer for content
		content := v.renderer.Render(line)

		// Calculate available width
		availableWidth := v.width
		if v.showLineNumbers {
			availableWidth -= lineNumWidth + 2 // +2 for mark char and space
		}

		if v.wrapLines {
			// Wrap long lines
			builder.WriteString(v.wrapContent(content, availableWidth))
		} else {
			// Apply horizontal offset and truncation
			content = v.applyHorizontalScroll(content, availableWidth)
			builder.WriteString(content)
		}
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

// applyHorizontalScroll applies horizontal offset and truncates to width
func (v *Viewport) applyHorizontalScroll(content string, width int) string {
	if width <= 0 {
		return ""
	}

	// Skip horizontal offset characters (ANSI-aware)
	// Always preserve ANSI codes, only skip visible characters
	skipped := 0
	var result strings.Builder
	inEscape := false

	for _, r := range content {
		if r == '\x1b' {
			inEscape = true
			result.WriteRune(r) // Always keep escape sequences
			continue
		}
		if inEscape {
			result.WriteRune(r) // Always keep escape sequences
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		// Regular character - skip if within offset
		if skipped < v.horizontalOffset {
			skipped++
			continue
		}

		result.WriteRune(r)
	}

	// Now truncate to width
	output := result.String()
	visWidth := 0
	var truncated strings.Builder
	inEscape = false

	for _, r := range output {
		if r == '\x1b' {
			inEscape = true
			truncated.WriteRune(r)
			continue
		}
		if inEscape {
			truncated.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if visWidth >= width {
			break
		}
		truncated.WriteRune(r)
		visWidth++
	}

	// Reset ANSI at end
	truncated.WriteString("\x1b[0m")
	return truncated.String()
}

// wrapContent wraps content to fit within width (ANSI-aware)
func (v *Viewport) wrapContent(content string, width int) string {
	if width <= 0 {
		return ""
	}

	var result strings.Builder
	visWidth := 0
	inEscape := false

	for _, r := range content {
		if r == '\x1b' {
			inEscape = true
			result.WriteRune(r)
			continue
		}
		if inEscape {
			result.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		// Check if we need to wrap
		if visWidth >= width {
			result.WriteString("\x1b[0m\n") // Reset and newline
			// Pad for continuation (no line number)
			result.WriteString(strings.Repeat(" ", v.width-width))
			visWidth = 0
		}

		result.WriteRune(r)
		visWidth++
	}

	result.WriteString("\x1b[0m")
	return result.String()
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

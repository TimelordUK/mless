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

	// Visual selection range (original line indices, -1 means no selection)
	visualStart  int
	visualEnd    int
	visualCursor int // The cursor line in visual mode (original index, -1 for none)
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
		visualStart:     -1,
		visualEnd:       -1,
		visualCursor:    -1,
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

// SetVisualSelection sets the visual selection range and cursor (original line indices)
// Pass -1, -1, -1 to clear the selection
func (v *Viewport) SetVisualSelection(start, end, cursor int) {
	v.visualStart = start
	v.visualEnd = end
	v.visualCursor = cursor
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

// Render returns the viewport content as a string.
//
// The output is ALWAYS exactly v.height physical rows, regardless of whether
// line wrapping is enabled. When wrapping is on, a single logical line can
// expand into several physical rows; those extra rows are counted against the
// height budget so the viewport never overflows its allotted space. This is
// what keeps split panes independent: each pane renders into its own fixed-size
// block, so one pane wrapping cannot push the other pane's rows around.
func (v *Viewport) Render() string {
	if v.provider == nil {
		return ""
	}

	// At most v.height logical lines can be visible (each occupies >= 1 row),
	// so fetching v.height lines is always enough to fill the row budget.
	lines, err := v.provider.GetLines(v.scrollOffset, v.height)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	lineCount := v.provider.LineCount()
	lineNumWidth := len(fmt.Sprintf("%d", lineCount))

	// Calculate available content width (gutter excluded)
	availableWidth := v.width
	gutterWidth := 0
	if v.showLineNumbers {
		gutterWidth = lineNumWidth + 2 // +2 for mark char and space
		availableWidth -= gutterWidth
	}

	// Build the list of physical rows, stopping once the height budget is full.
	rows := make([]string, 0, v.height)
	for i, line := range lines {
		if len(rows) >= v.height {
			break
		}

		gutter := v.renderGutter(line, i, lineNumWidth)
		content := v.renderer.Render(line)

		if v.wrapLines {
			// Wrap long lines into multiple physical rows.
			segments := v.wrapContentRows(content, availableWidth)
			contPad := strings.Repeat(" ", gutterWidth)
			for j, seg := range segments {
				if len(rows) >= v.height {
					break
				}
				if j == 0 {
					rows = append(rows, gutter+seg)
				} else {
					// Continuation rows have no line number; pad the gutter.
					rows = append(rows, contPad+seg)
				}
			}
		} else {
			// Apply horizontal offset and truncation to a single row.
			content = v.applyHorizontalScroll(content, availableWidth)
			rows = append(rows, gutter+content)
		}
	}

	// Assemble exactly v.height rows, padding the remainder with "~".
	var builder strings.Builder
	for i := 0; i < v.height; i++ {
		if i > 0 {
			builder.WriteString("\n")
		}
		if i < len(rows) {
			builder.WriteString(rows[i])
		} else {
			builder.WriteString("~")
		}
	}

	return builder.String()
}

// renderGutter builds the line-number / mark / visual-selection gutter for a
// logical line. Returns "" when line numbers are disabled. The index i is the
// line's position within the fetched window (used as a fallback for the
// original index of the first line).
func (v *Viewport) renderGutter(line *source.Line, i, lineNumWidth int) string {
	if !v.showLineNumbers {
		return ""
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

	// Check if this line has a mark
	markChar := ' '
	if v.marks != nil {
		if m, ok := v.marks[originalIdx]; ok {
			markChar = m
		}
	}

	// Check if this line is in visual selection
	inVisualSelection := v.visualStart >= 0 && v.visualEnd >= 0 &&
		originalIdx >= v.visualStart && originalIdx <= v.visualEnd
	isVisualCursor := v.visualCursor >= 0 && originalIdx == v.visualCursor

	numStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)
	switch {
	case isHighlighted:
		// Highlight line number with marker
		return v.highlightStyle.Render(fmt.Sprintf("%c%s ", markChar, numStr))
	case isVisualCursor:
		// Visual cursor: show █ marker in bright cyan to indicate cursor position
		cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // cyan
		return cursorStyle.Render("█") + v.lineNumberStyle.Render(fmt.Sprintf("%s ", numStr))
	case inVisualSelection:
		// Visual selection: show > marker in cyan
		visualStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // cyan
		return visualStyle.Render(">") + v.lineNumberStyle.Render(fmt.Sprintf("%s ", numStr))
	case markChar != ' ':
		// Show mark character in highlight style
		return v.highlightStyle.Render(string(markChar)) + v.lineNumberStyle.Render(fmt.Sprintf("%s ", numStr))
	default:
		return v.lineNumberStyle.Render(fmt.Sprintf(" %s ", numStr))
	}
}

// applyHorizontalScroll applies horizontal offset and truncates to width
func (v *Viewport) applyHorizontalScroll(content string, width int) string {
	if width <= 0 {
		return ""
	}

	// If no horizontal offset, just truncate to width and pad
	if v.horizontalOffset == 0 {
		visWidth := 0
		var truncated strings.Builder
		inEscape := false

		for _, r := range content {
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
		truncated.WriteString("\x1b[0m")
		return truncated.String()
	}

	// Skip horizontal offset characters (ANSI-aware)
	// Only keep ANSI codes that come AFTER we've finished skipping
	skipped := 0
	var result strings.Builder
	var escapeBuffer strings.Builder
	inEscape := false

	for _, r := range content {
		if r == '\x1b' {
			inEscape = true
			escapeBuffer.WriteRune(r)
			continue
		}
		if inEscape {
			escapeBuffer.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
				// Only keep escape sequence if we're past the skip zone
				if skipped >= v.horizontalOffset {
					result.WriteString(escapeBuffer.String())
				}
				escapeBuffer.Reset()
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

// wrapContentRows splits content into physical rows of at most `width` visible
// columns (ANSI-aware). Each returned segment is a single physical row's worth
// of content (without any gutter/continuation padding) and is terminated with a
// reset code. Always returns at least one segment so an empty/blank line still
// occupies one row.
func (v *Viewport) wrapContentRows(content string, width int) []string {
	if width <= 0 {
		return []string{""}
	}

	var rows []string
	var cur strings.Builder
	visWidth := 0
	inEscape := false

	for _, r := range content {
		if r == '\x1b' {
			inEscape = true
			cur.WriteRune(r)
			continue
		}
		if inEscape {
			cur.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		// Flush the current row once it is full.
		if visWidth >= width {
			cur.WriteString("\x1b[0m")
			rows = append(rows, cur.String())
			cur.Reset()
			visWidth = 0
		}

		cur.WriteRune(r)
		visWidth++
	}

	cur.WriteString("\x1b[0m")
	rows = append(rows, cur.String())
	return rows
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

// Height returns the viewport height in lines
func (v *Viewport) Height() int {
	return v.height
}

// CanScrollDown returns true if the viewport can scroll down further
func (v *Viewport) CanScrollDown() bool {
	if v.provider == nil {
		return false
	}
	maxScroll := v.provider.LineCount() - v.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	return v.scrollOffset < maxScroll
}

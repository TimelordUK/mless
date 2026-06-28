package ui

import (
	"strings"

	"github.com/TimelordUK/mless/internal/config"
	"github.com/TimelordUK/mless/internal/render"
	"github.com/TimelordUK/mless/internal/source"
	"github.com/TimelordUK/mless/internal/view"
	"github.com/TimelordUK/mless/pkg/logformat"
)

// Tab is a single workspace: one or two panes plus their split layout and zoom
// state. The Model owns a slice of Tabs and delegates window-management to the
// active one. Keeping these fields here (rather than on Model) makes layout,
// zoom, and split state naturally per-tab.
type Tab struct {
	panes      []*Pane
	activePane int
	splitDir   SplitDirection
	splitRatio float64 // 0.0 to 1.0, proportion for first pane (default 0.5)
	zoomed     bool    // tmux-style: render only the active pane full-screen

	config *config.Config

	// Content area this tab renders into (excludes the global status bar).
	width  int
	height int
}

// newTab builds a tab around an initial set of panes.
func newTab(panes []*Pane, splitDir SplitDirection, cfg *config.Config) *Tab {
	return &Tab{
		panes:      panes,
		activePane: 0,
		splitDir:   splitDir,
		splitRatio: 0.5,
		config:     cfg,
	}
}

// currentPane returns the currently active pane.
func (t *Tab) currentPane() *Pane {
	return t.panes[t.activePane]
}

// setSize updates the content area and re-lays out the panes.
func (t *Tab) setSize(width, height int) {
	t.width = width
	t.height = height
	t.calculatePaneSizes()
}

// setActivePane focuses pane idx, re-sizing if zoomed so the newly active pane
// fills the screen (the zoomed pane "follows" focus, tmux-style).
func (t *Tab) setActivePane(idx int) {
	if idx < 0 || idx >= len(t.panes) {
		return
	}
	t.activePane = idx
	if t.zoomed {
		t.calculatePaneSizes()
	}
}

// toggleZoom toggles full-screen focus on the active pane (only in a split).
func (t *Tab) toggleZoom() {
	if len(t.panes) > 1 {
		t.zoomed = !t.zoomed
		t.calculatePaneSizes()
	}
}

// toggleOrientation flips between vertical and horizontal split.
func (t *Tab) toggleOrientation() {
	if len(t.panes) <= 1 {
		return
	}
	if t.splitDir == SplitVertical {
		t.splitDir = SplitHorizontal
	} else {
		t.splitDir = SplitVertical
	}
	t.calculatePaneSizes()
}

// adjustRatio moves the splitter by delta, clamped to [0.1, 0.9].
func (t *Tab) adjustRatio(delta float64) {
	if len(t.panes) <= 1 {
		return
	}
	t.splitRatio += delta
	if t.splitRatio < 0.1 {
		t.splitRatio = 0.1
	}
	if t.splitRatio > 0.9 {
		t.splitRatio = 0.9
	}
	t.calculatePaneSizes()
}

// resetRatio restores a 50/50 split.
func (t *Tab) resetRatio() {
	if len(t.panes) > 1 {
		t.splitRatio = 0.5
		t.calculatePaneSizes()
	}
}

// splitVertical creates a vertical split (side-by-side panes)
func (t *Tab) splitVertical() {
	if len(t.panes) >= 2 {
		return // Already have max panes
	}

	current := t.currentPane()

	// Create new pane sharing the same source
	detector := logformat.NewLevelDetector(&t.config.LogLevels)
	newPane := &Pane{
		viewport:       view.NewViewport(80, 24),
		source:         current.source, // Shared source
		filteredSource: source.NewFilteredProvider(current.source, detector.Detect),
		config:         current.config,
		filename:       current.filename,
		sourcePath:     current.sourcePath,
		cachePath:      current.cachePath,
		isCached:       current.isCached,
		marks:          make(map[rune]int),
		visualAnchor:   -1,
	}
	newPane.viewport.SetProvider(newPane.filteredSource)
	newPane.viewport.SetRenderer(render.NewLogLevelRenderer(t.config))
	newPane.viewport.GotoLine(current.viewport.CurrentLine())

	t.panes = append(t.panes, newPane)
	t.splitDir = SplitVertical
	t.calculatePaneSizes()
}

// splitHorizontal creates a horizontal split (stacked panes)
func (t *Tab) splitHorizontal() {
	if len(t.panes) >= 2 {
		return
	}

	current := t.currentPane()

	detector := logformat.NewLevelDetector(&t.config.LogLevels)
	newPane := &Pane{
		viewport:       view.NewViewport(80, 24),
		source:         current.source,
		filteredSource: source.NewFilteredProvider(current.source, detector.Detect),
		config:         current.config,
		filename:       current.filename,
		sourcePath:     current.sourcePath,
		cachePath:      current.cachePath,
		isCached:       current.isCached,
		marks:          make(map[rune]int),
		visualAnchor:   -1,
	}
	newPane.viewport.SetProvider(newPane.filteredSource)
	newPane.viewport.SetRenderer(render.NewLogLevelRenderer(t.config))
	newPane.viewport.GotoLine(current.viewport.CurrentLine())

	t.panes = append(t.panes, newPane)
	t.splitDir = SplitHorizontal
	t.calculatePaneSizes()
}

// closeCurrentPane closes the active pane (cannot close the last one).
func (t *Tab) closeCurrentPane() {
	if len(t.panes) <= 1 {
		return // Can't close the last pane
	}

	// Don't close the source if other panes are using it
	closingPane := t.panes[t.activePane]
	sharedSource := false
	for i, p := range t.panes {
		if i != t.activePane && p.source == closingPane.source {
			sharedSource = true
			break
		}
	}

	// Remove the pane
	t.panes = append(t.panes[:t.activePane], t.panes[t.activePane+1:]...)

	// Adjust active pane index
	if t.activePane >= len(t.panes) {
		t.activePane = len(t.panes) - 1
	}

	// Reset split direction if only one pane left
	if len(t.panes) == 1 {
		t.splitDir = SplitNone
		t.zoomed = false
	}

	// Close the pane (but not the shared source)
	if !sharedSource {
		closingPane.Close()
	}

	t.calculatePaneSizes()
}

// Close releases the tab's panes, closing each distinct source only once
// (split panes within a tab share a source).
func (t *Tab) Close() error {
	var err error
	closed := make(map[*source.FileSource]bool)
	for _, p := range t.panes {
		// Skip panes whose source a sibling already closed; their cache path is
		// shared too, so there's nothing left to release.
		if p.source != nil {
			if closed[p.source] {
				continue
			}
			closed[p.source] = true
		}
		if e := p.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// calculatePaneSizes sets the dimensions for each pane within the content area.
func (t *Tab) calculatePaneSizes() {
	contentHeight := t.height

	if len(t.panes) == 1 {
		t.panes[0].SetSize(t.width, contentHeight)
		return
	}

	// Zoomed: the active pane fills the whole content area; the hidden pane
	// keeps whatever size it had (it isn't rendered until we unzoom).
	if t.zoomed {
		t.currentPane().SetSize(t.width, contentHeight)
		return
	}

	switch t.splitDir {
	case SplitVertical:
		// Side by side, leave 1 char for separator
		firstWidth := int(float64(t.width-1) * t.splitRatio)
		if firstWidth < 10 {
			firstWidth = 10
		}
		if firstWidth > t.width-11 {
			firstWidth = t.width - 11
		}
		t.panes[0].SetSize(firstWidth, contentHeight)
		t.panes[1].SetSize(t.width-firstWidth-1, contentHeight)

	case SplitHorizontal:
		// Stacked, leave 1 line for separator
		firstHeight := int(float64(contentHeight-1) * t.splitRatio)
		if firstHeight < 3 {
			firstHeight = 3
		}
		if firstHeight > contentHeight-4 {
			firstHeight = contentHeight - 4
		}
		t.panes[0].SetSize(t.width, firstHeight)
		t.panes[1].SetSize(t.width, contentHeight-firstHeight-1)
	}
}

// renderContent renders this tab's content area (single pane, zoomed pane, or
// split). The returned string includes its own trailing newline.
func (t *Tab) renderContent() string {
	if len(t.panes) == 1 {
		return t.panes[0].Render() + "\n"
	}
	if t.zoomed {
		return t.currentPane().Render() + "\n"
	}
	switch t.splitDir {
	case SplitVertical:
		return t.renderVerticalSplit()
	case SplitHorizontal:
		return t.renderHorizontalSplit()
	}
	return ""
}

// renderVerticalSplit renders two panes side by side
func (t *Tab) renderVerticalSplit() string {
	left := t.panes[0].Render()
	right := t.panes[1].Render()

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	var result strings.Builder

	// Choose separator based on active pane
	separator := "│"
	if t.activePane == 0 {
		separator = "┃"
	}

	// Get pane widths from ratio
	leftWidth := int(float64(t.width-1) * t.splitRatio)
	if leftWidth < 10 {
		leftWidth = 10
	}
	if leftWidth > t.width-11 {
		leftWidth = t.width - 11
	}
	rightWidth := t.width - leftWidth - 1

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	for i := 0; i < maxLines; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		rightLine := ""
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}

		// Truncate or pad left line to fit width
		leftLine = truncateOrPad(leftLine, leftWidth)
		// Truncate right line
		rightLine = truncateString(rightLine, rightWidth)

		result.WriteString(leftLine)
		result.WriteString(separator)
		result.WriteString(rightLine)
		result.WriteString("\n")
	}

	return result.String()
}

// renderHorizontalSplit renders two panes stacked
func (t *Tab) renderHorizontalSplit() string {
	top := t.panes[0].Render()
	bottom := t.panes[1].Render()

	// Choose separator based on active pane
	separator := strings.Repeat("─", t.width)
	if t.activePane == 1 {
		separator = strings.Repeat("━", t.width)
	}

	return top + "\n" + separator + "\n" + bottom + "\n"
}

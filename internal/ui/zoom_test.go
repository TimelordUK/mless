package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/TimelordUK/mless/internal/config"
)

func repeatedLines(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// newSplitModel builds a two-pane vertical-split Model from two temp files whose
// lines are filled with the given markers, ready to render.
func newSplitModel(t *testing.T, width, height int, markerA, markerB string) *Model {
	t.Helper()
	cfg := &config.Config{}
	paneA, err := NewPane(writeTempLog(t, repeatedLines(markerA, 50)), cfg, false)
	if err != nil {
		t.Fatalf("NewPane A: %v", err)
	}
	paneB, err := NewPane(writeTempLog(t, repeatedLines(markerB, 50)), cfg, false)
	if err != nil {
		t.Fatalf("NewPane B: %v", err)
	}
	m := &Model{
		tabs:        []*Tab{newTab([]*Pane{paneA, paneB}, SplitVertical, cfg)},
		activeTab:   0,
		searchInput: textinput.New(),
		config:      cfg,
		mode:        ModeNormal,
		width:       width,
		height:      height,
	}
	m.layoutTabs()
	return m
}

// TestZoomRendersOnlyActivePane covers the split-zoom feature: zoom shows just
// the active pane full-screen, zoom follows focus when you switch panes, the
// status bar shows [zoom], and unzoom restores the split.
func TestZoomRendersOnlyActivePane(t *testing.T) {
	const width, height = 60, 12
	const a, b = "AAAAAAAA", "BBBBBBBB"

	m := newSplitModel(t, width, height, a, b)
	defer func() {
		for _, p := range m.tab().panes {
			p.Close()
		}
	}()

	// Split view: both panes visible.
	if got := m.View(); !strings.Contains(got, a) || !strings.Contains(got, b) {
		t.Fatalf("split view should show both panes:\n%s", got)
	}

	// Zoom the active pane (A): only A shown.
	m.tab().zoomed = true
	m.tab().calculatePaneSizes()
	if got := m.View(); !strings.Contains(got, a) || strings.Contains(got, b) {
		t.Fatalf("zoom should show only active pane A:\n%s", got)
	}

	// Switch panes while zoomed: zoom follows focus to B.
	m.tab().setActivePane(1)
	got := m.View()
	if !strings.Contains(got, b) || strings.Contains(got, a) {
		t.Fatalf("zoom should follow focus to pane B:\n%s", got)
	}
	if !strings.Contains(got, "[zoom]") {
		t.Fatalf("expected [zoom] indicator in status bar:\n%s", got)
	}

	// Unzoom: split restored, both visible again.
	m.tab().zoomed = false
	m.tab().calculatePaneSizes()
	if got := m.View(); !strings.Contains(got, a) || !strings.Contains(got, b) {
		t.Fatalf("unzoom should restore the split:\n%s", got)
	}
}

// TestClosePaneResetsZoom verifies zoom is cleared when a split collapses back
// to a single pane, so the lone pane never renders in a stale zoomed state.
func TestClosePaneResetsZoom(t *testing.T) {
	const width, height = 60, 12

	m := newSplitModel(t, width, height, "AAAAAAAA", "BBBBBBBB")
	defer func() {
		for _, p := range m.tab().panes {
			p.Close()
		}
	}()

	m.tab().zoomed = true
	m.tab().calculatePaneSizes()

	m.tab().closeCurrentPane() // drops to one pane

	if m.tab().zoomed {
		t.Fatal("zoom should be reset after collapsing to a single pane")
	}
	if len(m.tab().panes) != 1 {
		t.Fatalf("expected 1 pane after close, got %d", len(m.tab().panes))
	}
}

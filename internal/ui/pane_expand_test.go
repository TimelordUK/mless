package ui

import (
	"strings"
	"testing"

	"github.com/TimelordUK/mless/internal/config"
)

// TestExpandCurrentLineInPlace verifies in-place single-line expansion: with
// global wrap off, the current (top) line still renders across multiple
// physical rows when expanded, and collapses back to one row when toggled off.
func TestExpandCurrentLineInPlace(t *testing.T) {
	const width, height = 40, 10
	const marker = "instanceId=3 --arg-tail-END"

	// One very long line followed by short ones; the long line carries a marker
	// at its tail so we can tell whether it was wrapped (visible) or truncated.
	lines := []string{
		strings.Repeat("x", 200) + " " + marker,
		"second line",
		"third line",
	}

	pane, err := NewPane(writeTempLog(t, lines), &config.Config{}, false)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	defer pane.Close()
	pane.SetSize(width, height)

	// Collapsed (default): the tail marker is off the right edge, not shown.
	if got := pane.Render(); strings.Contains(got, marker) {
		t.Fatalf("tail marker visible before expand (line should be truncated):\n%s", got)
	}

	// Expand the current (top) line: its full content wraps into view, and the
	// short lines below are still present (pushed down).
	if !pane.ToggleExpandCurrentLine() {
		t.Fatal("expected ToggleExpandCurrentLine to report expanded=true")
	}
	got := pane.Render()
	if !strings.Contains(got, marker) {
		t.Fatalf("tail marker not visible after expand:\n%s", got)
	}
	if !strings.Contains(got, "second line") {
		t.Fatalf("following line missing after expand:\n%s", got)
	}
	if strings.Count(got, "\n")+1 != height {
		t.Fatalf("render must still be exactly %d rows", height)
	}

	// Collapse again: back to truncated single row.
	if pane.ToggleExpandCurrentLine() {
		t.Fatal("expected ToggleExpandCurrentLine to report expanded=false")
	}
	if got := pane.Render(); strings.Contains(got, marker) {
		t.Fatalf("tail marker still visible after collapse:\n%s", got)
	}
}

// TestExpandSurvivesScrollAndClear checks that expansion is keyed by the
// original line (stays put when scrolled away and back) and that ClearExpanded
// collapses everything.
func TestExpandSurvivesScrollAndClear(t *testing.T) {
	const width, height = 40, 10
	const marker = "TAIL-MARKER-END"

	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "short"
	}
	lines[0] = strings.Repeat("y", 200) + " " + marker

	pane, err := NewPane(writeTempLog(t, lines), &config.Config{}, false)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	defer pane.Close()
	pane.SetSize(width, height)

	pane.ToggleExpandCurrentLine() // expand line 0

	pane.Viewport().ScrollDown(10)
	if got := pane.Render(); strings.Contains(got, marker) {
		t.Fatalf("expanded line should be off-screen after scrolling away:\n%s", got)
	}

	pane.Viewport().GotoTop()
	if got := pane.Render(); !strings.Contains(got, marker) {
		t.Fatalf("expansion did not persist after scrolling back:\n%s", got)
	}

	pane.ClearExpanded()
	if pane.HasExpanded() {
		t.Fatal("HasExpanded should be false after ClearExpanded")
	}
	if got := pane.Render(); strings.Contains(got, marker) {
		t.Fatalf("tail marker visible after ClearExpanded:\n%s", got)
	}
}

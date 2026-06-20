package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TimelordUK/mless/internal/config"
)

// writeTempLog writes lines to a temp file and returns its path.
func writeTempLog(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write temp log: %v", err)
	}
	return path
}

// TestToggleWrapReanchorsMatchNearEOF reproduces the original bug: a search
// match near end-of-file sits mid-screen (the scroll offset is clamped so it
// cannot reach the top). Turning wrap on expands the long lines above it into
// extra physical rows. Without re-anchoring the match is pushed off-screen;
// Pane.ToggleWrap must keep it visible.
func TestToggleWrapReanchorsMatchNearEOF(t *testing.T) {
	const width, height = 40, 10

	// Many wide lines so wrapping expands each into several rows; the unique
	// marker lives a few lines from the end, inside the clamp zone.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, strings.Repeat("x", 200)+" line")
	}
	// The match line itself is short so the marker is visible in both modes;
	// the bug is purely about vertical position, driven by the long lines above.
	const marker = "instanceId=3"
	matchLine := len(lines) - 3
	lines[matchLine] = marker

	pane, err := NewPane(writeTempLog(t, lines), &config.Config{}, false)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	defer pane.Close()
	pane.SetSize(width, height)

	pane.PerformSearch(marker)
	if got := pane.Render(); !strings.Contains(got, marker) {
		t.Fatalf("match not visible after search (non-wrapped):\n%s", got)
	}

	// The regression: toggle wrap and the match must remain on-screen.
	pane.ToggleWrap()
	if !pane.Viewport().IsWrapping() {
		t.Fatal("expected wrapping to be on after toggle")
	}
	if got := pane.Render(); !strings.Contains(got, marker) {
		t.Fatalf("match pushed off-screen after wrap toggle (the bug):\n%s", got)
	}

	// Toggling back off must still keep it visible.
	pane.ToggleWrap()
	if got := pane.Render(); !strings.Contains(got, marker) {
		t.Fatalf("match not visible after toggling wrap back off:\n%s", got)
	}
}

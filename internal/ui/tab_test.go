package ui

import (
	"strings"
	"testing"
)

// newTabModel builds a single-tab Model from one temp file, sized and laid out.
func newTabModel(t *testing.T, lines ...string) *Model {
	t.Helper()
	f := writeTempLog(t, lines)
	m, err := NewModelWithOptions(ModelOptions{Filepath: f})
	if err != nil {
		t.Fatalf("NewModelWithOptions: %v", err)
	}
	m.width, m.height = 80, 24
	m.layoutTabs()
	return m
}

// TestOpenTabSwitchesAndShowsBar covers opening a second tab: it becomes active,
// the tab bar appears (stealing one content row), and lists both tabs.
func TestOpenTabSwitchesAndShowsBar(t *testing.T) {
	m := newTabModel(t, "aaaa")
	defer m.Close()

	// One tab: no tab bar, full content height.
	singleHeight := m.tab().height
	if strings.Contains(m.View(), "2:") {
		t.Fatal("single tab should not render a tab bar")
	}

	f2 := writeTempLog(t, []string{"bbbb"})
	if err := m.openTab(f2); err != nil {
		t.Fatalf("openTab: %v", err)
	}
	if len(m.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(m.tabs))
	}
	if m.activeTab != 1 {
		t.Fatalf("new tab should be active, got activeTab=%d", m.activeTab)
	}

	// Tab bar now steals exactly one content row from every tab.
	if got := m.tab().height; got != singleHeight-1 {
		t.Fatalf("content height should drop by 1 for the tab bar: %d -> %d", singleHeight, got)
	}

	bar := m.renderTabBar()
	if !strings.Contains(bar, "1:") || !strings.Contains(bar, "2:") {
		t.Fatalf("tab bar should list both tabs: %q", bar)
	}
	if !strings.Contains(m.View(), "1:") {
		t.Fatal("View should include the tab bar with >1 tab")
	}
}

// TestTabNavigation covers gotoTab / nextTab / prevTab wraparound.
func TestTabNavigation(t *testing.T) {
	m := newTabModel(t, "aaaa")
	defer m.Close()
	if err := m.openTab(writeTempLog(t, []string{"bbbb"})); err != nil {
		t.Fatal(err)
	}
	if err := m.openTab(writeTempLog(t, []string{"cccc"})); err != nil {
		t.Fatal(err)
	}
	// Three tabs, active is the last (index 2).
	if m.activeTab != 2 {
		t.Fatalf("expected activeTab 2, got %d", m.activeTab)
	}
	m.nextTab() // wraps to 0
	if m.activeTab != 0 {
		t.Fatalf("nextTab should wrap to 0, got %d", m.activeTab)
	}
	m.prevTab() // wraps to 2
	if m.activeTab != 2 {
		t.Fatalf("prevTab should wrap to 2, got %d", m.activeTab)
	}
	m.gotoTab(1)
	if m.activeTab != 1 {
		t.Fatalf("gotoTab(1) failed, got %d", m.activeTab)
	}
	m.gotoTab(9) // out of range: no-op
	if m.activeTab != 1 {
		t.Fatalf("gotoTab out of range should be a no-op, got %d", m.activeTab)
	}
}

// TestCloseTab covers closing a tab, index adjustment, and the last-tab guard.
func TestCloseTab(t *testing.T) {
	m := newTabModel(t, "aaaa")
	defer m.Close()
	if err := m.openTab(writeTempLog(t, []string{"bbbb"})); err != nil {
		t.Fatal(err)
	}

	m.gotoTab(0)
	m.closeTab()
	if len(m.tabs) != 1 {
		t.Fatalf("expected 1 tab after close, got %d", len(m.tabs))
	}
	// Bar gone: content height restored.
	if m.tab().height != m.height-2 {
		t.Fatalf("content height should be restored after collapsing to one tab")
	}

	// Last tab can't be closed.
	m.message = ""
	m.closeTab()
	if len(m.tabs) != 1 {
		t.Fatalf("closing the last tab should be a no-op, got %d tabs", len(m.tabs))
	}
	if m.message == "" {
		t.Fatal("expected a message when refusing to close the last tab")
	}
}

// TestRunCommand covers the ":" command line: goto-line plus tab verbs.
func TestRunCommand(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	m := newTabModel(t, lines...)
	defer m.Close()

	m.runCommand("10")
	if got := m.currentPane().Viewport().CurrentLine(); got != 9 {
		t.Fatalf(":10 should go to line index 9, got %d", got)
	}

	m.runCommand("tabnew " + writeTempLog(t, []string{"other"}))
	if len(m.tabs) != 2 {
		t.Fatalf("tabnew should open a tab, got %d", len(m.tabs))
	}

	m.runCommand("tabclose")
	if len(m.tabs) != 1 {
		t.Fatalf("tabclose should remove a tab, got %d", len(m.tabs))
	}

	m.message = ""
	m.runCommand("tabnew") // missing path
	if m.message == "" {
		t.Fatal("tabnew without a path should report usage")
	}

	m.message = ""
	m.runCommand("tabnew /no/such/file/at/all.log")
	if m.message == "" {
		t.Fatal("tabnew on a missing file should surface an error message")
	}
	if len(m.tabs) != 1 {
		t.Fatalf("failed tabnew should not add a tab, got %d", len(m.tabs))
	}
}

// TestMaxTabs verifies the 9-tab cap.
func TestMaxTabs(t *testing.T) {
	m := newTabModel(t, "aaaa")
	defer m.Close()
	for len(m.tabs) < maxTabs {
		if err := m.openTab(writeTempLog(t, []string{"x"})); err != nil {
			t.Fatalf("openTab below cap failed: %v", err)
		}
	}
	if len(m.tabs) != maxTabs {
		t.Fatalf("expected %d tabs, got %d", maxTabs, len(m.tabs))
	}
	if err := m.openTab(writeTempLog(t, []string{"x"})); err == nil {
		t.Fatal("opening past the cap should error")
	}
	if len(m.tabs) != maxTabs {
		t.Fatalf("cap exceeded: %d tabs", len(m.tabs))
	}
}

package view

import (
	"strings"
	"testing"

	"github.com/TimelordUK/mless/internal/source"
)

// fakeProvider serves a fixed set of lines for viewport tests.
type fakeProvider struct {
	lines []string
}

func (f *fakeProvider) LineCount() int { return len(f.lines) }

func (f *fakeProvider) GetLine(index int) (*source.Line, error) {
	return &source.Line{Content: []byte(f.lines[index]), OriginalIndex: index}, nil
}

func (f *fakeProvider) GetLines(start, count int) ([]*source.Line, error) {
	var out []*source.Line
	for i := 0; i < count && start+i < len(f.lines); i++ {
		idx := start + i
		out = append(out, &source.Line{Content: []byte(f.lines[idx]), OriginalIndex: idx})
	}
	return out, nil
}

func countRows(s string) int {
	return strings.Count(s, "\n") + 1
}

// long returns a line wider than any reasonable viewport so wrapping is forced.
func long(n int) string {
	return strings.Repeat("x", n)
}

// TestRenderRowCountInvariant verifies the core split-view fix: Render() must
// always emit exactly `height` physical rows, whether or not lines wrap. A
// wrapping pane that emitted extra rows was what corrupted the adjacent pane.
func TestRenderRowCountInvariant(t *testing.T) {
	const width, height = 40, 10

	cases := []struct {
		name  string
		lines []string
		wrap  bool
	}{
		{"short_nowrap", []string{"a", "b", "c"}, false},
		{"short_wrap", []string{"a", "b", "c"}, true},
		{"full_nowrap", manyLines(height, 10), false},
		{"full_wrap_long", manyLines(height, 200), true},
		{"overflow_wrap", manyLines(height*5, 500), true},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := NewViewport(width, height)
			v.SetProvider(&fakeProvider{lines: tc.lines})
			if tc.wrap {
				v.ToggleWrap()
			}
			got := countRows(v.Render())
			if got != height {
				t.Fatalf("Render() produced %d rows, want exactly %d", got, height)
			}
		})
	}
}

// TestWrapStateIndependentPerViewport verifies two viewports (one per pane) keep
// independent wrap state and each still honors its own height budget.
func TestWrapStateIndependentPerViewport(t *testing.T) {
	lines := manyLines(50, 300)

	left := NewViewport(40, 10)
	left.SetProvider(&fakeProvider{lines: lines})
	right := NewViewport(40, 10)
	right.SetProvider(&fakeProvider{lines: lines})

	left.ToggleWrap() // wrap only the left pane

	if !left.IsWrapping() {
		t.Fatal("left should be wrapping")
	}
	if right.IsWrapping() {
		t.Fatal("right must not be affected by left's wrap toggle")
	}
	if n := countRows(left.Render()); n != 10 {
		t.Fatalf("wrapped pane produced %d rows, want 10", n)
	}
	if n := countRows(right.Render()); n != 10 {
		t.Fatalf("unwrapped pane produced %d rows, want 10", n)
	}
}

// TestWrapCarriesParentColor verifies that when a colored line (e.g. a red
// error) wraps across multiple physical rows, every continuation row re-opens
// the parent's color instead of falling back to the default foreground.
func TestWrapCarriesParentColor(t *testing.T) {
	const color = "\x1b[38;5;196m" // red foreground, as emitted by the renderer
	const width = 10

	// 35 visible chars at width 10 => 4 physical rows.
	content := color + strings.Repeat("x", 35) + "\x1b[0m"

	v := NewViewport(80, 24)
	rows := v.wrapContentRows(content, width)

	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	for i, row := range rows {
		if !strings.Contains(row, color) {
			t.Errorf("row %d does not re-open parent color %q: %q", i, color, row)
		}
		if !strings.HasSuffix(row, "\x1b[0m") {
			t.Errorf("row %d is not reset-terminated: %q", i, row)
		}
	}
}

// TestWrapClearsColorAfterReset verifies a mid-content reset stops the color
// from bleeding onto subsequent wrapped rows.
func TestWrapClearsColorAfterReset(t *testing.T) {
	const color = "\x1b[38;5;196m"
	const width = 5

	// Colored for the first 5 chars, reset, then 10 plain chars => rows 2+ plain.
	content := color + strings.Repeat("x", 5) + "\x1b[0m" + strings.Repeat("y", 10)

	v := NewViewport(80, 24)
	rows := v.wrapContentRows(content, width)

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		if strings.Contains(rows[i], color) {
			t.Errorf("row %d re-opened color after a reset: %q", i, rows[i])
		}
	}
}

func manyLines(count, width int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = long(width)
	}
	return out
}

// TestGotoBottomReachesEOFWhenWrapping verifies the wrap-aware scroll bound:
// when long lines wrap, the final screen holds fewer logical lines, so the max
// scroll offset must be higher than LineCount-height. Otherwise GotoBottom
// stops short and the last line is never rendered.
func TestGotoBottomReachesEOFWhenWrapping(t *testing.T) {
	const width, height = 40, 10

	lines := manyLines(30, 200) // each wraps to several rows
	lines[29] = "LAST-LINE-MARKER"

	v := NewViewport(width, height)
	v.SetProvider(&fakeProvider{lines: lines})
	v.ToggleWrap()
	v.GotoBottom()

	if got := v.Render(); !strings.Contains(got, "LAST-LINE-MARKER") {
		t.Fatalf("GotoBottom did not reach EOF in wrap mode:\n%s", got)
	}
	if countRows(v.Render()) != height {
		t.Fatalf("expected %d rows, got %d", height, countRows(v.Render()))
	}
}

// TestScrollLastLineToTop verifies the vim-style relaxed scroll bound: the
// final screenful is reachable, i.e. the last line can be scrolled all the way
// to the top row. Classic less pins it to the bottom and forbids going further,
// which made near-EOF lines unreachable as the current (top) line.
func TestScrollLastLineToTop(t *testing.T) {
	const width, height = 40, 10

	lines := manyLines(30, 10)
	lines[29] = "LAST-LINE-MARKER"

	v := NewViewport(width, height)
	v.SetProvider(&fakeProvider{lines: lines})

	// Scroll down well past the classic LineCount-height bound (20).
	v.ScrollDown(1000)

	if v.CurrentLine() != len(lines)-1 {
		t.Fatalf("expected top line %d (last line at top), got %d", len(lines)-1, v.CurrentLine())
	}
	first := strings.SplitN(v.Render(), "\n", 2)[0]
	if !strings.Contains(first, "LAST-LINE-MARKER") {
		t.Fatalf("last line not at top row:\n%s", first)
	}
	if v.PercentScrolled() != 100 {
		t.Fatalf("expected 100%%, got %v", v.PercentScrolled())
	}
}

// TestFileFittingOnScreenDoesNotScroll guards the relaxed bound: a file shorter
// than the viewport must not scroll its top lines off into "~".
func TestFileFittingOnScreenDoesNotScroll(t *testing.T) {
	const width, height = 40, 10

	v := NewViewport(width, height)
	v.SetProvider(&fakeProvider{lines: []string{"a", "b", "c"}})

	v.ScrollDown(1000)

	if v.CurrentLine() != 0 {
		t.Fatalf("short file scrolled to line %d, expected to stay at 0", v.CurrentLine())
	}
}

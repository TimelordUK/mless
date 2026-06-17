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

func manyLines(count, width int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = long(width)
	}
	return out
}

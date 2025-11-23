# Split View Design

## Overview

This document outlines the refactoring needed to support split views in mless, enabling side-by-side file viewing with eventual time-synchronized scrolling.

---

## Current Architecture Issues

The `Model` in `app.go` is monolithic - it owns everything directly:

```go
type Model struct {
    viewport       *view.Viewport
    source         *source.FileSource
    filteredSource *source.FilteredProvider
    searchInput    textinput.Model
    // ... marks, search state, slice stack, etc.
}
```

This structure doesn't support multiple file views.

---

## Target Architecture

### Pane Struct

Extract a `Pane` that encapsulates everything for a single file view:

```go
// internal/ui/pane.go
type Pane struct {
    viewport       *view.Viewport
    source         *source.FileSource
    filteredSource *source.FilteredProvider

    // File state
    filename   string
    sourcePath string
    cachePath  string
    isCached   bool

    // Search state
    searchTerm    string
    searchResults []int
    searchIndex   int

    // Marks
    marks map[rune]int

    // Slice state
    slicer     *slice.Slicer
    sliceStack []*slice.Info

    // Follow mode (per-pane)
    following bool
}
```

### Methods to Move to Pane

From `Model` to `Pane`:
- `performSearch()`, `nextSearchResult()`, `prevSearchResult()`
- `nextMark()`, `prevMark()`
- `parseAndSlice()`, `performSlice()`, `revertSlice()`, `sliceFromCurrent()`
- `parseLineRef()`
- `checkForNewLines()`, `resyncFromSource()`
- Mark set/jump handling
- Level filter toggles

### Model as Orchestrator

```go
type Model struct {
    panes       []*Pane
    activePane  int
    splitDir    SplitDirection

    // Shared UI state
    searchInput textinput.Model
    config      *config.Config
    mode        Mode
    width       int
    height      int

    // Time sync (Phase 4)
    timeSynced bool
}

type SplitDirection int

const (
    SplitNone SplitDirection = iota
    SplitVertical   // side-by-side |
    SplitHorizontal // stacked -
)
```

---

## Layout Management

### Dimension Calculation

```go
func (m *Model) calculatePaneSizes() {
    statusHeight := 2 // status bar + help line
    contentHeight := m.height - statusHeight

    if len(m.panes) == 1 {
        m.panes[0].viewport.SetSize(m.width, contentHeight)
        return
    }

    switch m.splitDir {
    case SplitVertical:
        // Side by side, leave 1 char for separator
        halfWidth := (m.width - 1) / 2
        m.panes[0].viewport.SetSize(halfWidth, contentHeight)
        m.panes[1].viewport.SetSize(m.width-halfWidth-1, contentHeight)

    case SplitHorizontal:
        // Stacked, leave 1 line for separator
        halfHeight := (contentHeight - 1) / 2
        m.panes[0].viewport.SetSize(m.width, halfHeight)
        m.panes[1].viewport.SetSize(m.width, contentHeight-halfHeight-1)
    }
}
```

### Rendering Split Views

```go
func (m *Model) renderVerticalSplit() string {
    left := m.panes[0].Render()
    right := m.panes[1].Render()

    leftLines := strings.Split(left, "\n")
    rightLines := strings.Split(right, "\n")

    var result strings.Builder
    separator := "│"
    if m.activePane == 0 {
        separator = "┃" // bold for active side
    }

    for i := 0; i < len(leftLines); i++ {
        result.WriteString(leftLines[i])
        result.WriteString(separator)
        if i < len(rightLines) {
            result.WriteString(rightLines[i])
        }
        result.WriteString("\n")
    }

    return result.String()
}

func (m *Model) renderHorizontalSplit() string {
    top := m.panes[0].Render()
    bottom := m.panes[1].Render()

    separator := strings.Repeat("─", m.width)
    if m.activePane == 1 {
        separator = strings.Repeat("━", m.width) // bold for active
    }

    return top + "\n" + separator + "\n" + bottom
}
```

---

## Input Routing

### Global vs Pane-Specific Commands

```go
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    // Handle input modes first (search, goto, etc.)
    if m.mode != ModeNormal {
        return m.handleModeInput(msg)
    }

    // Global split commands
    switch msg.String() {
    case "ctrl+w":
        m.mode = ModeSplitCmd
        return m, nil

    case "tab":
        if len(m.panes) > 1 {
            m.activePane = (m.activePane + 1) % len(m.panes)
        }
        return m, nil
    }

    // Route to active pane
    return m.panes[m.activePane].HandleKey(msg, m)
}

func (m *Model) handleSplitCmd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    m.mode = ModeNormal

    switch msg.String() {
    case "v": // Vertical split
        m.splitVertical()
    case "s": // Horizontal split
        m.splitHorizontal()
    case "w": // Switch pane
        m.activePane = (m.activePane + 1) % len(m.panes)
    case "q": // Close current pane
        m.closeCurrentPane()
    case "o": // Open file in new pane
        m.mode = ModeOpenFile
    }

    return m, nil
}
```

---

## Keybindings

### Split Management

| Key | Action |
|-----|--------|
| `ctrl+w v` | Vertical split (duplicate current view) |
| `ctrl+w s` | Horizontal split (duplicate current view) |
| `ctrl+w 'a` | Split with second pane at mark 'a |
| `ctrl+w \|range` | Split with second pane showing range (e.g., `\|1000-5000`) |
| `ctrl+w w` | Switch to next pane |
| `tab` | Switch to next pane (quick) |
| `ctrl+w q` | Close current pane |
| `ctrl+w =` | Equalize pane sizes |

### Split Range Examples

- `ctrl+w |1000-5000` - split showing lines 1000-5000
- `ctrl+w |'a-'b` - split showing between marks
- `ctrl+w |.-$` - split showing current line to end

### Time Sync (Phase 4)

| Key | Action |
|-----|--------|
| `ctrl+y` | Toggle time sync between panes |

---

## Implementation Phases

### Phase 3a: Extract Pane (No New Features)

**Goal**: Refactor without changing behavior

1. Create `internal/ui/pane.go` with `Pane` struct
2. Move state fields from Model to Pane
3. Move methods from Model to Pane
4. Model creates single pane on init
5. Model delegates to `m.panes[0]` for everything
6. **All tests pass, behavior unchanged**

**Files changed**:
- New: `internal/ui/pane.go`
- Modified: `internal/ui/app.go`

### Phase 3b: Split Commands

**Goal**: Basic split functionality

1. Add `SplitDirection` and `activePane` to Model
2. Implement `calculatePaneSizes()`
3. Add `ModeSplitCmd` for `ctrl+w` prefix
4. Implement split creation (duplicate current pane)
5. Implement pane switching
6. Implement pane closing
7. Render split views

**New keybindings**: `ctrl+w v/s/w/q`, `tab`

### Phase 3c: Open File in Split

**Goal**: Different files in each pane

1. Add file picker mode or path input
2. Create new pane with different file
3. Handle errors (file not found, etc.)

### Phase 3d: Independent Pane State

**Goal**: Each pane fully independent

1. Each pane has own filters
2. Each pane has own marks
3. Each pane has own search
4. Status bar shows active pane info

---

## Phase 4: Time-Synced Views

### Sync Mechanism

```go
func (m *Model) syncPaneToTime(sourcePane, targetPane int) {
    if !m.timeSynced {
        return
    }

    // Get current line's timestamp from source pane
    src := m.panes[sourcePane]
    currentLine := src.viewport.CurrentLine()
    originalLine := src.filteredSource.OriginalLineNumber(currentLine)

    ts := src.source.GetTimestamp(originalLine)
    if ts == nil {
        return
    }

    // Find matching line in target pane
    tgt := m.panes[targetPane]
    targetOriginal := tgt.source.FindLineAtTime(*ts)
    if targetOriginal < 0 {
        return
    }

    // Map to filtered index and scroll
    targetFiltered := tgt.filteredSource.FilteredIndexFor(targetOriginal)
    if targetFiltered >= 0 {
        tgt.viewport.GotoLine(targetFiltered)
    }
}
```

### Sync Triggers

1. After scroll in either pane
2. After `ctrl+t` goto time
3. After jumping to mark
4. Toggle with `ctrl+y`

### Visual Indicator

Status bar shows `[synced]` when time sync is active.

---

## Status Bar Design

### Single Pane (Current)

```
filename.log [slice:100-500]  L50/400 12:30:45  50%  [INF,WRN,ERR]
```

### Split View

```
[1] file1.log L100/5000 12:30:45 | [2*] file2.log L200/3000 12:30:47 [synced]
```

- `*` indicates active pane
- `[synced]` shows time sync is on

---

## Same-File Splits

When splitting the same file into two panes:

- Both panes share the same `source` (memory efficient, single mmap)
- Each pane has its own `filteredSource` (independent filters)
- Each pane has its own viewport, marks, search, slice stack
- Changes to the underlying file are reflected in both panes

```go
// Create second pane sharing source
func (m *Model) splitWithSharedSource(direction SplitDirection) {
    current := m.currentPane()

    // New pane shares source but has own filtered provider
    newPane := &Pane{
        source:         current.source,  // shared
        filteredSource: source.NewFilteredProvider(current.source, detector.Detect),
        viewport:       view.NewViewport(width, height),
        // ... own state
    }

    m.panes = append(m.panes, newPane)
}
```

---

## Edge Cases

### Unequal Timestamps

Files may have different time ranges or gaps:
- If target time not found, find nearest
- Show indicator when times don't match exactly
- Consider configurable tolerance (e.g., within 1 second)

### Different Filters

Each pane can have different level filters:
- Time sync uses original (unfiltered) line numbers
- If synced line is filtered out in target, find nearest visible

### Slice Navigation

When in slice:
- Sync uses slice's line numbers
- May need to track original file timestamps separately

---

## Testing Strategy

### Phase 3a Tests
- Create model, verify single pane works
- All existing functionality unchanged

### Phase 3b Tests
- Split creates two panes
- Dimensions calculated correctly
- Pane switching works
- Close pane works

### Phase 4 Tests
- Sync scrolls both panes
- Sync finds nearest time
- Sync respects filters
- Toggle sync on/off

---

## Open Questions

1. **Multiple splits?** - Start with 2 panes max, expand later?
2. **Resize ratio?** - Fixed 50/50 or draggable?
3. **Per-pane follow mode?** - Probably yes
4. **Copy between panes?** - Select in one, paste to other?

---

## Dependencies

- Phase 3a: None (refactor only)
- Phase 3b: Phase 3a
- Phase 3c: Phase 3b
- Phase 4: Phase 3b + timestamp indexing (already exists)

---

## Current Status

- [ ] Phase 3a: Extract Pane struct
- [ ] Phase 3b: Split commands
- [ ] Phase 3c: Open file in split
- [ ] Phase 3d: Independent pane state
- [ ] Phase 4: Time-synced views

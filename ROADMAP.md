# mless Feature Roadmap

## Vision
Extend mless from a log viewer into a tool for slicing, caching, and comparing log files - especially useful for large network files where extracting relevant sections locally improves performance and allows focused analysis.

---

## Phase 1: File Slicing to Cache

Core ability to extract regions from a file into local cache, then work with the slice as a standalone file.

### Slice Operations
- From current line to end of file
- From line N to line M (range)
- From time T1 to time T2
- Between two marked positions
- Current filtered view (level/text filter) to file

### Proposed Commands
- `s` - enter slice mode, prompt for range type
- `:slice 100-500` - slice line range
- `:slice 10:30:00-10:45:00` - slice time range
- `ctrl+s` - quick slice from current position to end
- `m` - mark current line (for start/end of slice region)

### Implementation Steps
1. Add `SliceMode` to UI modes
2. Create `internal/slice` package:
   - `Slicer` struct to extract line ranges
   - Write to temp file (similar to cache behavior)
   - Track slice metadata (source file, range, parent slice)
3. Extend `Model` to track slice stack (for revert)
4. Add slice input handling in `handleSliceKey()`
5. Switch viewport to sliced file after creation
6. Update status bar to show slice info

### Status Bar Example
```
file.log > [slice:100-500]  L50/400  50%
```

---

## Phase 2: Slice Management

### Features
- Revert to original/parent file (`R` or new key)
- Re-slice from current slice (nested slicing)
- `:write <path>` to save slice permanently
- Slice history/breadcrumb navigation

### Implementation Steps
1. Add slice stack to Model (parent tracking)
2. Modify `R` to pop slice stack (revert)
3. Add `:write` command to save current view to file
4. Show slice breadcrumb in status bar
5. Consider slice metadata file for complex workflows

---

## Phase 3: Split View

Ability to view two files side-by-side.

### Features
- Vertical split (`|` or `ctrl+w v`)
- Horizontal split (`-` or `ctrl+w s`)
- Switch focus between panes (`ctrl+w w` or `tab`)
- Close split (`ctrl+w q`)
- Independent scrolling by default

### Implementation Steps
1. Refactor `Model` to support multiple viewports
2. Create `SplitManager` to handle pane layout
3. Track active pane for input routing
4. Render multiple viewports side-by-side
5. Handle resize for split dimensions

---

## Phase 4: Time-Synced Views

Keep two log files synchronized by timestamp - essential for comparing logs from different services/machines.

### Features
- Toggle time-sync between splits (`ctrl+y`)
- When synced:
  - `ctrl+t` goto time applies to both panes
  - `j/k` scrolling finds nearest timestamp in other pane
  - Scroll one file, other jumps to same time
- Visual sync indicator in status bar
- Configurable time tolerance for "same time" matching

### Implementation Steps
1. Add sync state to SplitManager
2. Create time-matching algorithm:
   - Binary search for nearest timestamp
   - Handle files with different time granularity
3. Hook scroll events to trigger sync
4. Add sync toggle command
5. Visual indicator: `[synced]` or link icon in status

### Time Matching Algorithm
```go
// Find line in file B closest to timestamp from file A
func (s *SyncedView) FindNearestTime(target time.Time) int {
    // Binary search through file B's timestamp index
    // Return line number with closest timestamp
}
```

---

## Phase 5: Virtual Merged View

Interleave two log files into a single time-ordered stream.

### Features
- **Merge by timestamp** - combine logs into unified timeline
- **Source indicators** - color or prefix to show origin file
- **Toggle view** - switch between split and merged (`ctrl+m`)
- **Filter by source** - show only lines from one file

### Implementation Steps
1. Create `MergedProvider` that wraps two sources
2. Build unified timestamp index across both files
3. Interleave lines in time order
4. Add source field to `Line` struct
5. Color-code or prefix lines by source
6. Allow filtering to single source

---

## Phase 6: Time Delta Features

Show elapsed time from a reference point.

### Features
- **Mark reference time** - `mt` to set T=0 at current line
- **Delta display** - show +/-HH:MM:SS.mmm from mark
- **Gutter mode** - show delta in line number area
- **Status bar mode** - show delta for current line
- **Duration between marks** - select range, show elapsed

### Proposed Commands
- `mt` - mark current line as T=0
- `ctrl+d` - toggle delta display mode
- `'t` - jump back to T=0 mark

### Implementation Steps
1. Add `referenceTime` to Model/Pane
2. Calculate delta for each visible line
3. Format delta as +/-HH:MM:SS
4. Option to show in gutter vs status bar
5. Handle lines without timestamps gracefully

---

## Phase 7: Multi-Tab Support

> Superseded — see **Workspace, Tabs & Window Management (planning)** below for
> the current approach (extract a `Tab` struct from `Model` rather than a
> `TabManager` of Models, plus zoom and the recommended sequencing).

Open multiple files in tabs, switch between them.

### Features
- **Tab bar** - show open files at top
- **Switch tabs** - `gt`/`gT` or `1-9` direct jump
- **New tab** - `:tabnew <file>` or `ctrl+t`
- **Close tab** - `:tabclose` or `ctrl+w`
- **Promote split** - move pane to own tab

### Implementation Steps
1. Create `TabManager` to hold multiple Models
2. Render tab bar above content
3. Route input to active tab
4. Handle tab switching animations
5. Persist tab state for session restore

---

## Wrap-Aware Viewport (goto-line + wrap focus)  — Phase A DONE

**Problem.** The viewport scrolls in *logical-line* units (`scrollOffset` = top
logical line), but with wrap on the screen budget is *physical rows*. This
mismatch causes three issues:

1. The scroll anchor is the *top* line, not the line you care about. Search does
   `GotoLine(match)` (match at top), but near EOF `clampScroll()` caps
   `scrollOffset` at `LineCount - height`, so the match lands mid-screen; toggling
   wrap (`Z`) then expands the lines above it and pushes it off the bottom — you
   "lose focus" on the match.
2. `clampScroll` uses logical math (`LineCount - height`), which is wrong under
   wrap — a wrapped screenful holds *fewer* logical lines.
3. You can't anchor to the middle of a wrapped line, so any wrap toggle re-flows
   around the top line instead of your point of interest.

Real-world workflow that hurts: search `instanceId=3`, find it, then need the
full (very long) line → press `Z` to wrap → match scrolls away.

### Phase A — cheap wins ✅ DONE (shipped)
- **Re-anchor on `Z`** ✅: `Pane.ToggleWrap` re-pins the highlighted line to the
  top in the new mode (`Viewport.HighlightedLine` accessor). Match stays put
  across the toggle.
- **Wrap-aware scroll bounds** ✅: `bottomAnchorOffset` (EOF pinned to the bottom
  for `G`) and a relaxed `maxScrollOffset` (last line can reach the top, vim
  style) replace the old `LineCount - height` clamp. Fixes the near-EOF "lose the
  match" case *and* a latent bug where wrap mode couldn't scroll to true EOF.
  Files that fit on screen still don't scroll; `PercentScrolled` caps at 100%.
- **In-place single-line expand (`z`)** ✅: instead of the planned context-peek
  *overlay*, we landed a simpler in-place model — `z` expands/collapses the
  current (top) line, wrapping just that line even with global wrap off. Keyed by
  original line (survives filter/scroll like a mark); several can be expanded at
  once; `esc` collapses all. This is the "read this one long line's full args on
  one screen while n-nexting" case, with no overlay and no scroll-model change.
  See `internal/ui/pane.go` (`ToggleExpandCurrentLine`) and
  `internal/view/viewport.go` (per-line wrap in `Render`).

### Phase B — real fix: physical-row anchor (still future)
The one case Phase A deliberately does **not** solve: you can't land the viewport
*partway into* a line taller than the whole screen — scrolling is per logical
line, so such a line shows from its first row or is scrolled off the top, tail
unreachable. Fixing that needs the physical-row anchor below. Not yet needed.
Replace `scrollOffset int` with `{topLine int, topSubRow int}` (which logical
line is at top, and which wrapped sub-row of it).
- `displayRowsFor(line, width)` cached per (line, width).
- `ScrollDown/Up`, `PageDown/Up` advance in display rows, recompute anchor.
- `GotoLine(N)` = `{N, 0}` → target pinned at top, fully visible, wrap on or off.
- `ToggleWrap` keeps the same anchor logical line (subRow→0) → focus preserved by
  construction.
- Clamp becomes wrap-aware.
- **Cost control:** keep `PercentScrolled` and coarse clamp in *logical* space;
  only do wrap-width math for the visible window + active scroll. Never index the
  whole file (keeps it fast on huge logs). Also fixes `j/k` granularity and makes
  goto-line exact.

### Notes from the field
- Split-view wrap corruption is already fixed: `Viewport.Render` now always emits
  exactly `height` physical rows, so a wrapping pane can't overflow into the
  adjacent pane (`internal/view/viewport.go`, test in `viewport_test.go`).
- Pane-switch chords get trapped upstream: Windows Terminal eats `ctrl+w`;
  vim-tmux-navigator's root-table `ctrl+h/j/k/l` switch tmux panes. Working paths:
  `tab` (tmux-safe) and the leader chords `ctrl+x`/`ctrl+w` then `h/j/k/l`.

---

## Workspace, Tabs & Window Management (planning)

Supersedes the older Phase 7 sketch ("TabManager holding multiple Models") with a
cleaner structural move. The guiding idea is to keep three concepts **orthogonal**
so complexity never compounds:

- **Tabs** = "more files open" (independent full workspaces)
- **Split** = "compare two side by side" — **cap at 2 panes per tab**; resist
  nested/recursive splits (that's where tmux-style complexity detonates — open a
  tab instead)
- **Zoom** = "focus one pane temporarily"

### Sequencing (cheapest, highest-leverage first)

**1. Split zoom — ✅ DONE.**
`zoomed bool` on the Model: when set, `calculatePaneSizes` gives the active pane
the full content area and `View` renders only that pane. Toggle with
`<leader> z` (tmux muscle memory). Zoom *follows focus* — switching panes while
zoomed re-sizes and shows the newly active one (`setActivePane` helper). Status
bar shows `[zoom]`; collapsing back to one pane clears it. No refactor needed.
See `internal/ui/app.go`, tests in `internal/ui/zoom_test.go`.

**2. Time-synced scroll (the old Phase 4) — self-contained, high value.**
NOTE: greenfield — there is currently **no sync code in the tree**, despite the
Phase 4 entry. It's a property of a 2-pane split, needs no tab work: add
`synced bool` + reuse the existing timestamp index so scrolling pane A
binary-searches pane B for the nearest timestamp and repositions it. Very
testable; the original "compare two services' logs" use-case.

**3. Extract a `Tab`/`Workspace` struct — ✅ DONE.**
`{panes, activePane, splitDir, splitRatio, zoomed}` now live on a `Tab`
(`internal/ui/tab.go`), which also carries its `config` + content `width/height`
and owns all window-management (`splitVertical/Horizontal`, `closeCurrentPane`,
`calculatePaneSizes`, `setActivePane`, `toggleZoom/Orientation`, `adjustRatio`,
`renderContent`). `Model` holds `tabs []*Tab` + `activeTab` and stays the global
shell (config, mode, status, dimensions); a thin `tab()` accessor + a
`currentPane()` delegate kept the in-file key handlers untouched, and resize flows
through `Model.layoutTabs()`. Pure structural move, no behavior change — zoom +
constructor smoke tests green. This unlocks tabs and makes zoom/layout per-tab.

**4. Tabs + cross-tab follow — once the Tab struct exists.**
- Cap at **9 tabs** — gives `1`–`9` as direct jump keys (vim/tmux window-number
  model). Resource cost per file (mmap + line index + timestamp cache) is bounded;
  the real cost is follow polling + redraw, not memory.
- One **global ticker** iterates every follow-enabled pane across all tabs and
  refreshes its source; only visible panes re-render. Show a "● new data" marker
  on inactive tab labels.

### Configurable keymaps — defer the engine, fix the real pain now

The real pain isn't "rebind every key", it's **leader/chord collisions** with
tmux / zellij / Windows Terminal trapping chords before mless sees them.

- **Near term (cheap):** make the **leader key configurable**; keep the alias
  approach already in use (`ctrl+x` for `ctrl+w`). Document which chords clash and
  how to remap the leader. ~80% of the pain for ~5% of the effort.
- **Later (only on real demand):** refactor the normal-mode `switch` into an
  `Action` enum + `map[string]Action` keymap loaded from config (with defaults).
  Side benefit: the help screen can be generated from the keymap instead of
  hand-maintained. Real work — don't pay for it until someone wants per-key
  rebinding.
- **Principle:** one configurable leader for *window-management* verbs (split,
  zoom, tab-next, close); single letters reserved for *in-file* navigation.
  Minimizes collisions by construction.

---

## Scratch Pane (yank hunks into a working buffer)

Field idea (from real log-triage at work): split vertical, **left = the open log
(read-only source)**, **right = a scratch pane** you *pull hunks of interest
into*. Read down the log, yank the sections that matter over to the right, and
end up with a curated buffer of just-the-good-bits — then optionally write it to
disk. Turns mless from "view + slice one file" into "harvest evidence from a
file into a keepable artifact" without leaving the pager.

### How it differs from what we already have
- **Yank-to-clipboard** (`yy`/`Y`/visual-yank/`y'a`, shipped) sends text *out* of
  the app to the system clipboard — one selection at a time, no accumulation.
- **Slice + `:write`** (Phases 1–2, shipped) carves a *contiguous* range into a
  file and *switches the view to it*.
- **Scratch** is the missing middle: an **in-app, append-only buffer** you push
  *many non-contiguous* hunks into while still reading the source, building up a
  collection. Same extraction code as yank, different sink.

### Phase 1 — in-memory scratch (MVP)
- New `ScratchSource` in `internal/source` implementing `LineProvider` but
  **mutable**: an append-only `[][]byte` (plus per-hunk provenance). A pane whose
  provider is a `ScratchSource` is a "scratch pane".
- **Open scratch** as the right pane: e.g. `ctrl+w n` (new empty scratch) — sits
  in the existing 2-pane split, no tab work needed.
- **Send hunk to scratch** by reusing the yank extraction (factor the visual /
  mark-range / count-prefix grab so it can target either clipboard *or* the
  scratch sink): in visual mode `A` = append selection; normal `"s yy`-style
  register, or a leader verb — pick one, keep it one keystroke from a yank.
- Scratch pane reuses the normal viewport (scroll, search, level coloring all
  work for free since it's just a `LineProvider`).

### Phase 2 — provenance & persistence
- **Provenance separators**: each appended hunk gets an optional header line
  (`── file.log:1240–1268 @ 13:42:01 ──`) so you know where every block came
  from. Toggleable for clean exports.
- **Write to disk**: reuse the existing `:write <path>` plumbing to dump the
  scratch buffer; optionally back it with a temp file from the start (like the
  cache/slice temp files) so it survives a crash and can be re-opened.
- **Light editing**: delete the hunk under the cursor, clear all. Resist
  full-editor scope — append + delete-hunk + write covers the workflow.

### Notes / dependencies
- Builds entirely on **Phase 3 split (done)** + **existing yank (done)**;
  independent of the Tab/Workspace refactor but composes with it (a scratch could
  later be "promoted to a tab", or be one global scratch shared across tabs).
- Keep the source pane strictly read-only; the scratch is the only writable
  surface, which keeps the mental model (and undo story) simple.

---

## Future Ideas

- **Diff view**: highlight differences between synced files
- **Regex filter mode**: in addition to literal text filter
- **Time range filter**: show only lines within time window
- **Export filtered view**: save current filter as new file
- **Search across splits**: find term in all open files
- **Session save/restore**: remember open files, splits, positions
- **Tail multiple files**: follow mode across splits
- **Time offset**: shift timestamps for files from different timezones
- **Bookmarks file**: persist marks across sessions

---

## Technical Notes

### File Tracking Structure
```go
type SliceInfo struct {
    SourcePath   string      // Original file
    CachePath    string      // Local temp file
    StartLine    int         // Slice start (0-based)
    EndLine      int         // Slice end (exclusive)
    StartTime    *time.Time  // If time-based slice
    EndTime      *time.Time
    Parent       *SliceInfo  // For nested slices
}
```

### Dependencies
- Phase 1: None (builds on existing cache infrastructure)
- Phase 2: Requires Phase 1
- Phase 3: Independent (can parallel with Phase 1-2)
- Phase 4: Requires Phase 3 + timestamp indexing from Phase 1

---

## Current Status

- [x] Basic log viewing with level coloring
- [x] Level filtering (t/d/i/w/e toggles, T/D/I/W/E for level+above)
- [x] Text filtering (fzf-style live filter)
- [x] File caching (`-c` flag)
- [x] Follow mode (`F`)
- [x] Time navigation (`ctrl+t` goto time)
- [x] Timestamp detection and display
- [x] **Phase 1: File slicing** (ctrl+s quick slice, S for range input)
- [x] Phase 2: Slice management (nested slices, R to revert, depth indicator)
- [x] Phase 3: Split view (ctrl+w v/s/w/q, H/L resize, ctrl+o toggle orientation)
- [x] Marks (ma-mz set, 'a-'z jump, ]'/[' next/prev)
- [x] Yank to clipboard (yy/Y, y'a to mark, count prefixes)
- [x] Horizontal scrolling (</>, ^, Z wrap toggle)
- [x] Syntax highlighting for source files (chroma-based)
- [x] Split-view wrap fix (Render emits exactly `height` rows; panes independent)
- [x] Wrap-Aware Viewport Phase A: re-anchor on `Z`, wrap-aware scroll bounds,
      reachable last screenful, in-place single-line expand (`z`)
- [x] Split zoom (`<leader> z`, follows focus, `[zoom]` indicator)
- [ ] **Time-synced scroll** (old Phase 4 — greenfield, no code yet) ← Next
- [x] Extract `Tab`/`Workspace` struct (`internal/ui/tab.go`) — enables tabs +
      per-tab zoom/layout
- [ ] Tabs (cap 9, `1`-`9` jump) + cross-tab follow ticker
- [ ] Configurable leader key (then full keymap engine only on demand)
- [ ] Scratch pane: yank non-contiguous hunks into an append-only buffer, then
      `:write` to disk (Phase 1 in-memory; Phase 2 provenance + persistence)
- [ ] Wrap-Aware Viewport Phase B: physical-row anchor (partial-top-line)
- [ ] Phase 5: Virtual merged view
- [ ] Phase 6: Time delta features

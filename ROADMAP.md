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
- [ ] **Phase 4: Time-synced views** ← Next
- [ ] Phase 5: Virtual merged view
- [ ] Phase 6: Time delta features
- [ ] Phase 7: Multi-tab support

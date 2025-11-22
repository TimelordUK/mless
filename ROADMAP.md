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

## Future Ideas

- **Diff view**: highlight differences between synced files
- **Bookmark lines**: mark interesting lines, jump between them
- **Export filtered view**: save current filter as new file
- **Search across splits**: find term in all open files
- **Session save/restore**: remember open files, splits, positions
- **Tail multiple files**: follow mode across splits
- **Time offset**: shift timestamps for files from different timezones

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
- [ ] **Phase 1: File slicing** ← Next
- [ ] Phase 2: Slice management
- [ ] Phase 3: Split view
- [ ] Phase 4: Time-synced views

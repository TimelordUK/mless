# mless - Enhanced Log Viewer

An enhanced `less` for log files with level filtering, time navigation, file slicing, and split views.

## Architecture

```
cmd/mless/main.go          - Entry point, CLI argument parsing
internal/
  ui/
    app.go                  - Main bubbletea Model, key handling, rendering
    pane.go                 - Single file view with its own state
  view/
    viewport.go             - Scrolling viewport, renders lines
  source/
    file.go                 - Memory-mapped file source
    filtered.go             - Level/text filtering layer
    provider.go             - LineProvider interface
  index/
    lines.go                - Line offsets and timestamp indexing
  slice/
    slicer.go               - Extract file portions to cache
  render/
    renderer.go             - Log level coloring
  config/
    config.go               - Configuration loading
pkg/
  logformat/
    level.go                - Log level detection
    timestamp.go            - Timestamp parsing
```

## Key Concepts

### Model / Pane Separation
- `Model` is the top-level bubbletea model handling global state (mode, split layout)
- `Pane` represents a single file view with its own viewport, filters, marks, search state
- Split views use multiple panes sharing or independent sources

### Filtering Architecture
- `FilteredProvider` wraps `FileSource` and maintains `filteredIndices []int`
- Maps filtered view positions to original line numbers
- `FilteredIndexFor(originalLine)` does reverse mapping for time/mark jumps
- Filter is a view layer - slicing operates on original lines

### Time Navigation
- Timestamps parsed lazily and cached in `LineIndex.timestamps`
- `FindLineAtTime()` returns original line number
- Combined with `FilteredIndexFor()` for filtered view navigation

### Slicing
- Extracts line ranges to temp files in `/tmp`
- `sliceStack` allows nested slices with revert (`R`)
- Range syntax supports: `100-500`, `.-$-1000`, `'a-'b`, `13:00-14:00`

### Marks
- Stored as `map[rune]int` (char → original line number)
- Work correctly across filter changes
- Displayed in gutter

## Key Bindings

### Navigation
- `j/k` - scroll line by line
- `f/b` - page down/up
- `g/G` - top/bottom
- `:N` - go to line N
- `ctrl+t` - go to time

### Filtering
- `t/d/i/w/e` - toggle trace/debug/info/warn/error
- `T/D/I/W/E` - show level and above
- `?pattern` - fzf-style text filter
- `0` - clear all filters

### Search
- `/pattern` - search
- `n/N` - next/prev result

### Marks
- `ma`-`mz` - set mark
- `'a`-`'z` - jump to mark
- `]['` - next/prev mark
- `M` - clear marks

### Slicing
- `S` - slice range (`100-500`, `'a-'b`, `13:00-14:00`)
- `ctrl+s` - slice from current to end
- `R` - revert slice

### Split Views
- `ctrl+w v` - vertical split
- `ctrl+w s` - horizontal split
- `ctrl+w w` / `tab` - switch pane
- `ctrl+w q` - close pane

### Other
- `F` - follow mode
- `h` - help
- `ctrl+g` - file info
- `q` - quit

## Command Line

```bash
mless [-c] [-S range] [-t time] [file]
command | mless [-S range] [-t time]
```

- `-c` - cache file locally (for network files)
- `-S` - initial slice range
- `-t` - go to time on open

## Implementation Notes

### Adding New Features

1. **New Mode**: Add to `Mode` const, handle in `handleKey()`, add handler function
2. **New Pane Method**: Add to `Pane` struct in `pane.go`, expose via accessor
3. **New Key Binding**: Add case in normal mode switch in `handleKey()`

### Filter/View Coordinate Systems
- Viewport works in filtered indices (0 to filteredSource.LineCount()-1)
- Marks/time navigation store original line numbers
- Always use `FilteredIndexFor()` when jumping from original to filtered
- Always use `OriginalLineNumber()` when going from filtered to original

### Split View Rendering
- `calculatePaneSizes()` divides space between panes
- Vertical split: side-by-side with `│` separator
- Horizontal split: stacked with `─` separator
- Active pane shown with bold separator (`┃`/`━`)

## Future Plans (ROADMAP.md)

- Time-synced split views (scroll together by timestamp)
- Multiple file command line support
- Resizable splits
- Syntax highlighting mode for code files

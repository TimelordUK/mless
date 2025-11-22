# mless Architecture

## Vision

A modern, fast log viewer that is a superset of `less` with deep log awareness, multi-file correlation, and time-series intelligence.

---

## Core Design Principles

1. **Layered Architecture** - Each concern is isolated and composable
2. **Content Agnostic Core** - The viewport knows nothing about logs
3. **Plugin-style Processors** - Filters, parsers, renderers are swappable
4. **Time as First-Class Citizen** - Timestamps are indexed, not searched

---

## Architecture Layers

```
┌─────────────────────────────────────────────────┐
│                   UI Layer                      │
│  (tabs, splits, keybindings, themes)            │
├─────────────────────────────────────────────────┤
│                 View Layer                      │
│  (viewport, scrolling, selection, markers)      │
├─────────────────────────────────────────────────┤
│               Render Pipeline                   │
│  (syntax highlighting, log level colors)        │
├─────────────────────────────────────────────────┤
│              Filter Pipeline                    │
│  (include/exclude, log levels, regex, time)     │
├─────────────────────────────────────────────────┤
│              Source Abstraction                 │
│  (single file, merged files, virtual streams)   │
├─────────────────────────────────────────────────┤
│               Index Layer                       │
│  (line offsets, timestamps, log levels)         │
├─────────────────────────────────────────────────┤
│                 I/O Layer                       │
│  (mmap, chunked reading, file watching)         │
└─────────────────────────────────────────────────┘
```

---

## Layer Responsibilities

### I/O Layer
- Memory-mapped file access
- Chunk management for huge files
- File change detection (tail -f behavior)
- **Knows nothing about content meaning**

### Index Layer
- Line offset index (byte positions)
- Timestamp index (line → time mapping)
- Log level index (line → level mapping)
- Background indexing with progress
- **Parses but doesn't filter or render**

### Source Abstraction
- `SingleFile` - one file
- `MergedSource` - multiple files sorted by time
- `FilteredSource` - wraps another source with exclusions
- **Provides unified line iteration interface**

### Filter Pipeline
- Chain of filters, each can pass/reject lines
- `LevelFilter` - exclude DEBUG, show only ERROR
- `RegexFilter` - include/exclude patterns
- `TimeRangeFilter` - show only 14:00-15:00
- **Filters are composable and hot-swappable**

### Render Pipeline
- Transforms lines into styled output
- `LogLevelHighlighter` - colors by level
- `TimestampHighlighter` - dims or highlights times
- `SearchHighlighter` - highlights search matches
- **Renderers stack, each adds styling**

### View Layer
- Viewport state (scroll position, dimensions)
- Line wrapping decisions
- Selection and markers
- **Completely agnostic to content type**

### UI Layer
- Tab management
- Split screen layouts
- Keybinding dispatch
- Command palette
- **Orchestrates views, doesn't know about logs**

---

## Key Abstractions

### `LineProvider` Interface
```go
type LineProvider interface {
    // Total lines (may be estimated during indexing)
    LineCount() int

    // Get line content by index (post-filter index)
    GetLine(index int) (Line, error)

    // Get range of lines efficiently
    GetLines(start, count int) ([]Line, error)

    // Map filtered index to original file position
    OriginalPosition(index int) FilePosition
}
```

### `Line` Structure
```go
type Line struct {
    Content   []byte
    Timestamp *time.Time    // nil if not detected
    Level     *LogLevel     // nil if not detected
    Source    *SourceInfo   // which file, for merged views
}
```

### `Filter` Interface
```go
type Filter interface {
    // Returns true if line should be included
    Include(line *Line) bool

    // Human-readable description for status bar
    Description() string
}
```

### `Renderer` Interface
```go
type Renderer interface {
    // Apply styling to line, returns styled segments
    Render(line *Line, styles []StyledSegment) []StyledSegment
}
```

---

## Evolution Roadmap

### Phase 1: Foundation
**Goal**: Basic `less` replacement that's fast and correct

- [ ] I/O layer with mmap
- [ ] Line index building
- [ ] Basic viewport with scrolling
- [ ] Simple keybindings (j/k/g/G/q//)
- [ ] Search with highlighting

**Key decisions**:
- Establish `LineProvider` interface
- Viewport only talks to `LineProvider`
- No log awareness yet

### Phase 2: Log Awareness
**Goal**: Parse and understand log formats

- [ ] Timestamp detection (multiple formats)
- [ ] Log level detection (INFO, [INF], etc.)
- [ ] Background indexing of timestamps/levels
- [ ] Log level color themes
- [ ] Jump to time (`:goto 14:30:00`)

**Key decisions**:
- Index layer builds timestamp/level indices
- Renderers for log highlighting
- Keep detection patterns configurable

### Phase 3: Filtering
**Goal**: Dynamic content filtering

- [ ] Log level filtering (hide DEBUG)
- [ ] Regex include/exclude
- [ ] Time range filtering
- [ ] Filter indicators in status bar
- [ ] Quick toggles for common filters

**Key decisions**:
- FilteredSource wraps underlying source
- Filters are a pipeline, not hardcoded
- Maintain mapping from filtered→original line

### Phase 4: Multi-file
**Goal**: View multiple files together

- [ ] Tab support
- [ ] Split screen (horizontal/vertical)
- [ ] Time-synced scrolling between splits
- [ ] Merged view (virtual log from N files)

**Key decisions**:
- MergedSource implementation
- Sync mechanism based on timestamp index
- Color coding by source file

### Phase 5: Advanced Features
**Goal**: Power user features

- [ ] Bookmarks/markers
- [ ] Persistent sessions
- [ ] Custom log format definitions
- [ ] Export filtered view
- [ ] Tail mode with auto-scroll
- [ ] Command palette

---

## Critical Patterns

### Viewport Isolation

The viewport must NEVER know about:
- Log levels
- Timestamps
- File sources
- Filters applied

It only knows:
- How many lines exist (from LineProvider)
- Current scroll position
- Visible dimensions
- Which lines to request

This isolation means we can:
- Swap filtered/unfiltered sources
- Change renderers without viewport changes
- Test viewport independently

### Index Independence

Indices are built once, used by many:
- Timestamp index → used by TimeRangeFilter, time sync, goto
- Level index → used by LevelFilter, LevelHighlighter
- Line offset index → used by everyone

Never re-parse files. Index once, query the index.

### Filter Stacking

```go
source := NewFileSource("app.log")
filtered := source
filtered = NewLevelFilter(filtered, ExcludeLevels(DEBUG, TRACE))
filtered = NewTimeFilter(filtered, After(startTime))
filtered = NewRegexFilter(filtered, Exclude("healthcheck"))

// Viewport sees filtered as just another LineProvider
viewport.SetSource(filtered)
```

Each filter:
- Implements LineProvider
- Wraps another LineProvider
- Maintains its own index mapping

### Render Stacking

```go
renderers := []Renderer{
    NewLogLevelRenderer(theme),
    NewTimestampRenderer(dimTimestamps),
    NewSearchRenderer(searchTerm),
}

// Apply in order
segments := []StyledSegment{{Text: line.Content}}
for _, r := range renderers {
    segments = r.Render(line, segments)
}
```

---

## File Structure (Proposed)

```
mless/
├── cmd/
│   └── mless/
│       └── main.go
├── internal/
│   ├── io/
│   │   ├── mmap.go
│   │   ├── chunk.go
│   │   └── watcher.go
│   ├── index/
│   │   ├── lines.go
│   │   ├── timestamps.go
│   │   └── levels.go
│   ├── source/
│   │   ├── provider.go      # LineProvider interface
│   │   ├── file.go          # SingleFile implementation
│   │   ├── merged.go        # MergedSource
│   │   └── filtered.go      # FilteredSource wrapper
│   ├── filter/
│   │   ├── filter.go        # Filter interface
│   │   ├── level.go
│   │   ├── regex.go
│   │   └── timerange.go
│   ├── render/
│   │   ├── renderer.go      # Renderer interface
│   │   ├── level.go
│   │   ├── timestamp.go
│   │   └── search.go
│   ├── view/
│   │   ├── viewport.go
│   │   ├── selection.go
│   │   └── markers.go
│   └── ui/
│       ├── app.go
│       ├── tabs.go
│       ├── splits.go
│       ├── keys.go
│       └── theme.go
├── pkg/
│   └── logformat/
│       ├── detect.go        # Auto-detect log format
│       ├── timestamp.go     # Timestamp patterns
│       └── level.go         # Level patterns
└── go.mod
```

---

## Anti-Patterns to Avoid

### 1. Viewport knowing about filters
❌ `viewport.SetLevelFilter(DEBUG, false)`
✅ `viewport.SetSource(NewLevelFilter(source, ...))`

### 2. Hardcoded log formats
❌ `if strings.Contains(line, "[INFO]")`
✅ `levelDetector.Detect(line)` with configurable patterns

### 3. Re-parsing on scroll
❌ Parse timestamp every time line is displayed
✅ Build timestamp index once, query by line number

### 4. Monolithic state
❌ Single giant App struct with all state
✅ Each component owns its state, communicates via messages

### 5. Synchronous indexing
❌ Block UI while indexing 10GB file
✅ Background goroutine, progress updates, incremental results

---

## Testing Strategy

- **I/O Layer**: Test with real files, mock filesystem
- **Index Layer**: Unit test with known log samples
- **Filters**: Pure functions, easy to unit test
- **Renderers**: Snapshot tests for styled output
- **Viewport**: Test scroll math without any I/O
- **Integration**: Test full pipeline with sample logs

---

## Open Questions

1. **Config format** - TOML? YAML? Keep it simple
2. **Plugin system** - Do we need external plugins or just good defaults?
3. **Mouse support** - Worth the complexity?
4. **Sixel/images** - Some logs have embedded images, worth supporting?

---

## Next Steps

1. Initialize Go module with dependencies
2. Implement I/O layer with mmap
3. Build basic line index
4. Create minimal viewport with bubbletea
5. Get basic scrolling working
6. Then iterate through phases


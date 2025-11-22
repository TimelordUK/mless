package source

import "bytes"

// LevelDetectFunc detects log level from content
type LevelDetectFunc func(content []byte) LogLevel

// FilteredProvider wraps a LineProvider and filters by log level
type FilteredProvider struct {
	source   LineProvider
	detector LevelDetectFunc

	// Level filter: if set, only show lines with these levels
	levelFilter map[LogLevel]bool

	// Text filter: substring match
	textFilter []byte

	// Cached filtered indices (original line numbers that pass filter)
	filteredIndices []int
	dirty           bool
}

// NewFilteredProvider creates a filtered provider
func NewFilteredProvider(source LineProvider, detector LevelDetectFunc) *FilteredProvider {
	return &FilteredProvider{
		source:      source,
		detector:    detector,
		levelFilter: make(map[LogLevel]bool),
		dirty:       true,
	}
}

// SetLevelFilter sets which levels to show (empty = show all)
func (f *FilteredProvider) SetLevelFilter(levels map[LogLevel]bool) {
	f.levelFilter = levels
	f.dirty = true
}

// ToggleLevel toggles a level in the filter
func (f *FilteredProvider) ToggleLevel(level LogLevel) {
	if f.levelFilter[level] {
		delete(f.levelFilter, level)
	} else {
		f.levelFilter[level] = true
	}
	f.dirty = true
}

// SetOnlyLevel sets filter to show only this level
func (f *FilteredProvider) SetOnlyLevel(level LogLevel) {
	f.levelFilter = map[LogLevel]bool{level: true}
	f.dirty = true
}

// SetLevelAndAbove sets filter to show this level and all higher severity
func (f *FilteredProvider) SetLevelAndAbove(level LogLevel) {
	f.levelFilter = make(map[LogLevel]bool)
	// Levels are ordered: Unknown=0, Trace=1, Debug=2, Info=3, Warn=4, Error=5, Fatal=6
	allLevels := []LogLevel{LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal}
	for _, l := range allLevels {
		if l >= level {
			f.levelFilter[l] = true
		}
	}
	f.dirty = true
}

// ClearFilter removes all level filters
func (f *FilteredProvider) ClearFilter() {
	f.levelFilter = make(map[LogLevel]bool)
	f.dirty = true
}

// SetTextFilter sets the text substring filter
func (f *FilteredProvider) SetTextFilter(text string) {
	if text == "" {
		f.textFilter = nil
	} else {
		f.textFilter = []byte(text)
	}
	f.dirty = true
}

// ClearTextFilter removes the text filter
func (f *FilteredProvider) ClearTextFilter() {
	f.textFilter = nil
	f.dirty = true
}

// GetTextFilter returns the current text filter
func (f *FilteredProvider) GetTextFilter() string {
	return string(f.textFilter)
}

// HasTextFilter returns true if a text filter is active
func (f *FilteredProvider) HasTextFilter() bool {
	return len(f.textFilter) > 0
}

// MarkDirty marks the filter index as needing rebuild
func (f *FilteredProvider) MarkDirty() {
	f.dirty = true
}

// IsFiltered returns true if any filter is active
func (f *FilteredProvider) IsFiltered() bool {
	return len(f.levelFilter) > 0 || len(f.textFilter) > 0
}

// GetActiveFilters returns the active level filters
func (f *FilteredProvider) GetActiveFilters() map[LogLevel]bool {
	return f.levelFilter
}

// rebuildIndex rebuilds the filtered index if dirty
func (f *FilteredProvider) rebuildIndex() {
	if !f.dirty {
		return
	}

	f.filteredIndices = nil

	// If no filter, don't build index (use source directly)
	if len(f.levelFilter) == 0 && len(f.textFilter) == 0 {
		f.dirty = false
		return
	}

	// Build filtered index
	total := f.source.LineCount()
	for i := 0; i < total; i++ {
		line, err := f.source.GetLine(i)
		if err != nil {
			continue
		}

		// Check text filter first (most common case)
		if len(f.textFilter) > 0 {
			if !bytes.Contains(line.Content, f.textFilter) {
				continue
			}
		}

		// Check level filter if active
		if len(f.levelFilter) > 0 {
			// Detect level if not already set
			level := line.Level
			if level == LevelUnknown && f.detector != nil {
				level = f.detector(line.Content)
			}

			// Check if level passes filter
			if !f.levelFilter[level] {
				continue
			}
		}

		f.filteredIndices = append(f.filteredIndices, i)
	}

	f.dirty = false
}

// LineCount returns total number of filtered lines
func (f *FilteredProvider) LineCount() int {
	f.rebuildIndex()

	if len(f.levelFilter) == 0 && len(f.textFilter) == 0 {
		return f.source.LineCount()
	}
	return len(f.filteredIndices)
}

// GetLine returns line at filtered index
func (f *FilteredProvider) GetLine(index int) (*Line, error) {
	f.rebuildIndex()

	if len(f.levelFilter) == 0 && len(f.textFilter) == 0 {
		return f.source.GetLine(index)
	}

	if index < 0 || index >= len(f.filteredIndices) {
		return nil, nil
	}

	originalIndex := f.filteredIndices[index]
	line, err := f.source.GetLine(originalIndex)
	if err != nil {
		return nil, err
	}

	// Store original index for display
	line.OriginalIndex = originalIndex
	return line, nil
}

// GetLines returns a range of filtered lines
func (f *FilteredProvider) GetLines(start, count int) ([]*Line, error) {
	f.rebuildIndex()

	if len(f.levelFilter) == 0 && len(f.textFilter) == 0 {
		return f.source.GetLines(start, count)
	}

	var lines []*Line
	for i := start; i < start+count && i < len(f.filteredIndices); i++ {
		line, err := f.GetLine(i)
		if err != nil {
			return lines, err
		}
		if line != nil {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// OriginalLineNumber returns the original line number for a filtered index
func (f *FilteredProvider) OriginalLineNumber(filteredIndex int) int {
	f.rebuildIndex()

	if len(f.levelFilter) == 0 {
		return filteredIndex
	}

	if filteredIndex < 0 || filteredIndex >= len(f.filteredIndices) {
		return -1
	}
	return f.filteredIndices[filteredIndex]
}

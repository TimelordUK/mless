package ui

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TimelordUK/mless/internal/config"
	"github.com/TimelordUK/mless/internal/render"
	"github.com/TimelordUK/mless/internal/slice"
	"github.com/TimelordUK/mless/internal/source"
	"github.com/TimelordUK/mless/internal/view"
	"github.com/TimelordUK/mless/pkg/logformat"
)

// Pane represents a single file view with its own state
type Pane struct {
	viewport       *view.Viewport
	source         *source.FileSource
	filteredSource *source.FilteredProvider
	config         *config.Config

	// File state
	filename   string
	sourcePath string
	cachePath  string
	isCached   bool

	// Follow mode
	following bool

	// Slice state
	slicer     *slice.Slicer
	sliceStack []*slice.Info

	// Marks (a-z) - stores original line numbers
	marks map[rune]int

	// Search state
	searchTerm    string
	searchResults []int
	searchIndex   int

	// Filter state
	filterTerm string
}

// NewPane creates a new pane for a file
func NewPane(filePath string, cfg *config.Config, cacheFile bool) (*Pane, error) {
	var actualPath string
	var cachePath string
	var isCached bool

	if cacheFile {
		// Create cache directory
		cacheDir := os.TempDir()

		// Generate cache filename from source path hash
		hash := md5Sum([]byte(filePath))
		baseName := filepath.Base(filePath)
		cachePath = filepath.Join(cacheDir, fmt.Sprintf("mless-%x-%s", hash[:8], baseName))

		// Copy file to cache
		if err := copyFile(filePath, cachePath); err != nil {
			return nil, fmt.Errorf("failed to cache file: %w", err)
		}

		actualPath = cachePath
		isCached = true
	} else {
		actualPath = filePath
	}

	src, err := source.NewFileSource(actualPath)
	if err != nil {
		// Clean up cache file if we created one
		if cachePath != "" {
			os.Remove(cachePath)
		}
		return nil, err
	}

	// Set up level detector and filtered provider
	detector := logformat.NewLevelDetector(&cfg.LogLevels)
	filtered := source.NewFilteredProvider(src, detector.Detect)

	viewport := view.NewViewport(80, 24)
	viewport.SetProvider(filtered)
	viewport.SetShowLineNumbers(cfg.Display.ShowLineNumbers)

	// Set up log level renderer
	renderer := render.NewLogLevelRenderer(cfg)
	viewport.SetRenderer(renderer)

	return &Pane{
		viewport:       viewport,
		source:         src,
		filteredSource: filtered,
		config:         cfg,
		filename:       filepath.Base(filePath),
		sourcePath:     filePath,
		cachePath:      cachePath,
		isCached:       isCached,
		slicer:         slice.NewSlicer(),
		marks:          make(map[rune]int),
	}, nil
}

// SetSize sets the viewport size
func (p *Pane) SetSize(width, height int) {
	p.viewport.SetSize(width, height)
}

// Render returns the rendered viewport content
func (p *Pane) Render() string {
	// Update viewport with current marks
	if len(p.marks) > 0 {
		reverseMarks := make(map[int]rune)
		for char, line := range p.marks {
			reverseMarks[line] = char
		}
		p.viewport.SetMarks(reverseMarks)
	} else {
		p.viewport.SetMarks(nil)
	}

	return p.viewport.Render()
}

// Close cleans up pane resources
func (p *Pane) Close() error {
	var err error
	if p.source != nil {
		err = p.source.Close()
	}

	// Delete cached file
	if p.cachePath != "" {
		os.Remove(p.cachePath)
	}

	return err
}

// Viewport returns the pane's viewport
func (p *Pane) Viewport() *view.Viewport {
	return p.viewport
}

// Source returns the pane's file source
func (p *Pane) Source() *source.FileSource {
	return p.source
}

// FilteredSource returns the pane's filtered provider
func (p *Pane) FilteredSource() *source.FilteredProvider {
	return p.filteredSource
}

// Filename returns the display filename
func (p *Pane) Filename() string {
	return p.filename
}

// IsFollowing returns whether follow mode is active
func (p *Pane) IsFollowing() bool {
	return p.following
}

// SetFollowing sets follow mode
func (p *Pane) SetFollowing(following bool) {
	p.following = following
}

// ToggleFollowing toggles follow mode
func (p *Pane) ToggleFollowing() bool {
	p.following = !p.following
	return p.following
}

// SearchTerm returns the current search term
func (p *Pane) SearchTerm() string {
	return p.searchTerm
}

// SearchResults returns the search results
func (p *Pane) SearchResults() []int {
	return p.searchResults
}

// HasSlice returns whether the pane has an active slice
func (p *Pane) HasSlice() bool {
	return len(p.sliceStack) > 0
}

// CurrentSlice returns the current slice info
func (p *Pane) CurrentSlice() *slice.Info {
	if len(p.sliceStack) == 0 {
		return nil
	}
	return p.sliceStack[len(p.sliceStack)-1]
}

// IsCached returns whether the file is cached
func (p *Pane) IsCached() bool {
	return p.isCached
}

// PerformSearch executes a search
func (p *Pane) PerformSearch(term string) {
	p.searchTerm = term
	if term == "" {
		p.searchResults = nil
		return
	}

	// Simple search - find all lines containing the term
	p.searchResults = nil
	for i := 0; i < p.source.LineCount(); i++ {
		line, err := p.source.GetLine(i)
		if err != nil {
			continue
		}
		if strings.Contains(string(line.Content), term) {
			p.searchResults = append(p.searchResults, i)
		}
	}

	// Jump to first result
	if len(p.searchResults) > 0 {
		p.searchIndex = 0
		p.viewport.GotoLine(p.searchResults[0])
		p.viewport.SetHighlightedLine(p.searchResults[0])
	} else {
		p.viewport.ClearHighlight()
	}
}

// NextSearchResult jumps to next search result
func (p *Pane) NextSearchResult() {
	if len(p.searchResults) == 0 {
		return
	}
	p.searchIndex = (p.searchIndex + 1) % len(p.searchResults)
	p.viewport.GotoLine(p.searchResults[p.searchIndex])
	p.viewport.SetHighlightedLine(p.searchResults[p.searchIndex])
}

// PrevSearchResult jumps to previous search result
func (p *Pane) PrevSearchResult() {
	if len(p.searchResults) == 0 {
		return
	}
	p.searchIndex--
	if p.searchIndex < 0 {
		p.searchIndex = len(p.searchResults) - 1
	}
	p.viewport.GotoLine(p.searchResults[p.searchIndex])
	p.viewport.SetHighlightedLine(p.searchResults[p.searchIndex])
}

// ClearSearch clears search state
func (p *Pane) ClearSearch() {
	p.searchTerm = ""
	p.searchResults = nil
	p.searchIndex = 0
	p.viewport.ClearHighlight()
}

// SetMark sets a mark at the current line
func (p *Pane) SetMark(char rune) {
	currentFiltered := p.viewport.CurrentLine()
	originalLine := p.filteredSource.OriginalLineNumber(currentFiltered)
	if originalLine >= 0 {
		p.marks[char] = originalLine
	}
}

// JumpToMark jumps to a mark
func (p *Pane) JumpToMark(char rune) bool {
	originalLine, ok := p.marks[char]
	if !ok {
		return false
	}

	filteredIndex := p.filteredSource.FilteredIndexFor(originalLine)
	if filteredIndex >= 0 {
		p.viewport.GotoLine(filteredIndex)
		actualOriginal := p.filteredSource.OriginalLineNumber(filteredIndex)
		if actualOriginal >= 0 {
			p.viewport.SetHighlightedLine(actualOriginal)
		}
	}
	return true
}

// ClearMarks clears all marks
func (p *Pane) ClearMarks() {
	p.marks = make(map[rune]int)
	p.viewport.ClearHighlight()
}

// NextMark jumps to the next mark by line order
func (p *Pane) NextMark() {
	if len(p.marks) == 0 {
		return
	}

	currentFiltered := p.viewport.CurrentLine()
	currentOriginal := p.filteredSource.OriginalLineNumber(currentFiltered)

	var nextLine int = -1
	var firstLine int = -1

	for _, line := range p.marks {
		if firstLine == -1 || line < firstLine {
			firstLine = line
		}
		if line > currentOriginal {
			if nextLine == -1 || line < nextLine {
				nextLine = line
			}
		}
	}

	if nextLine == -1 {
		nextLine = firstLine
	}

	if nextLine >= 0 {
		filteredIndex := p.filteredSource.FilteredIndexFor(nextLine)
		if filteredIndex >= 0 {
			p.viewport.GotoLine(filteredIndex)
			actualOriginal := p.filteredSource.OriginalLineNumber(filteredIndex)
			if actualOriginal >= 0 {
				p.viewport.SetHighlightedLine(actualOriginal)
			}
		}
	}
}

// PrevMark jumps to the previous mark by line order
func (p *Pane) PrevMark() {
	if len(p.marks) == 0 {
		return
	}

	currentFiltered := p.viewport.CurrentLine()
	currentOriginal := p.filteredSource.OriginalLineNumber(currentFiltered)

	var prevLine int = -1
	var lastLine int = -1

	for _, line := range p.marks {
		if line > lastLine {
			lastLine = line
		}
		if line < currentOriginal {
			if line > prevLine {
				prevLine = line
			}
		}
	}

	if prevLine == -1 {
		prevLine = lastLine
	}

	if prevLine >= 0 {
		filteredIndex := p.filteredSource.FilteredIndexFor(prevLine)
		if filteredIndex >= 0 {
			p.viewport.GotoLine(filteredIndex)
			actualOriginal := p.filteredSource.OriginalLineNumber(filteredIndex)
			if actualOriginal >= 0 {
				p.viewport.SetHighlightedLine(actualOriginal)
			}
		}
	}
}

// CheckForNewLines checks if file has grown and updates view
func (p *Pane) CheckForNewLines() error {
	newLines, err := p.source.Refresh()
	if err != nil {
		return err
	}

	if newLines > 0 {
		p.filteredSource.MarkDirty()
		p.viewport.GotoBottom()
	}
	return nil
}

// ResyncFromSource re-copies the source file to cache and reloads
func (p *Pane) ResyncFromSource() error {
	if !p.isCached || p.sourcePath == "" || p.cachePath == "" {
		return nil
	}

	// Close current source
	p.source.Close()

	// Re-copy from source
	if err := copyFile(p.sourcePath, p.cachePath); err != nil {
		return err
	}

	// Reopen the cached file
	src, err := source.NewFileSource(p.cachePath)
	if err != nil {
		return err
	}

	// Update the source
	p.source = src

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&p.config.LogLevels)
	p.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	p.viewport.SetProvider(p.filteredSource)

	// Reset position
	p.viewport.GotoTop()

	// Clear search results
	p.ClearSearch()

	return nil
}

// ParseAndSlice parses a range string and performs the slice
func (p *Pane) ParseAndSlice(rangeStr string) error {
	currentFiltered := p.viewport.CurrentLine()
	currentLine := p.filteredSource.OriginalLineNumber(currentFiltered)
	if currentLine < 0 {
		currentLine = 0
	}
	totalLines := p.source.LineCount()

	var start, end int
	var startStr, endStr string

	// Find the separator dash (not one that's part of $-N or .-N)
	dashIdx := -1
	for i := 0; i < len(rangeStr); i++ {
		if rangeStr[i] == '-' {
			if i > 0 && (rangeStr[i-1] == '$' || rangeStr[i-1] == '.') {
				continue
			}
			dashIdx = i
			break
		}
	}

	if dashIdx >= 0 {
		startStr = rangeStr[:dashIdx]
		endStr = rangeStr[dashIdx+1:]
	} else {
		startStr = rangeStr
		endStr = "$"
	}

	start = p.parseLineRef(startStr, currentLine, totalLines)
	end = p.parseLineRef(endStr, currentLine, totalLines)

	if start < 0 {
		start = 0
	}
	if end > totalLines {
		end = totalLines
	}

	return p.PerformSlice(start, end)
}

// parseLineRef parses a line reference like ".", "$", "$-100", "'a", "13:00", or "500"
func (p *Pane) parseLineRef(ref string, current, total int) int {
	ref = strings.TrimSpace(ref)

	if ref == "" {
		return 0
	}

	if ref == "." {
		return current
	}

	if ref == "$" {
		return total
	}

	// Handle mark references like 'a
	if strings.HasPrefix(ref, "'") && len(ref) >= 2 {
		markChar := ref[1]
		if markChar >= 'a' && markChar <= 'z' {
			if line, ok := p.marks[rune(markChar)]; ok {
				return line
			}
		}
		return -1
	}

	// Handle time references like 13:00 or 13:00:00
	if strings.Contains(ref, ":") && !strings.HasPrefix(ref, "$") && !strings.HasPrefix(ref, ".") {
		if target := p.parseTimeInput(ref); target != nil {
			line := p.source.FindLineAtTime(*target)
			if line >= 0 {
				return line
			}
		}
		return -1
	}

	// Handle $-N or $+N
	if strings.HasPrefix(ref, "$") {
		offset := 0
		fmt.Sscanf(ref[1:], "%d", &offset)
		return total + offset
	}

	// Handle .-N or .+N
	if strings.HasPrefix(ref, ".") {
		offset := 0
		fmt.Sscanf(ref[1:], "%d", &offset)
		return current + offset
	}

	// Absolute line number (1-based input, convert to 0-based)
	var lineNum int
	fmt.Sscanf(ref, "%d", &lineNum)
	return lineNum - 1
}

// parseTimeInput parses user time input into a time.Time
func (p *Pane) parseTimeInput(input string) *time.Time {
	layouts := []string{
		"15:04:05",
		"15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, input); err == nil {
			if layout == "15:04:05" || layout == "15:04" {
				if firstTs := p.source.GetTimestamp(0); firstTs != nil {
					t = time.Date(firstTs.Year(), firstTs.Month(), firstTs.Day(),
						t.Hour(), t.Minute(), t.Second(), 0, firstTs.Location())
				} else {
					now := time.Now()
					t = time.Date(now.Year(), now.Month(), now.Day(),
						t.Hour(), t.Minute(), t.Second(), 0, time.Local)
				}
			}
			return &t
		}
	}

	return nil
}

// SliceFromCurrent slices from current viewport line to end
func (p *Pane) SliceFromCurrent() error {
	currentFiltered := p.viewport.CurrentLine()
	originalLine := p.filteredSource.OriginalLineNumber(currentFiltered)
	if originalLine < 0 {
		originalLine = 0
	}

	return p.PerformSlice(originalLine, p.source.LineCount())
}

// PerformSlice executes a slice operation and switches to the sliced file
func (p *Pane) PerformSlice(start, end int) error {
	info, cachePath, err := p.slicer.SliceRange(p.source, start, end)
	if err != nil {
		return err
	}

	// Track parent slice info
	if len(p.sliceStack) > 0 {
		info.Parent = p.sliceStack[len(p.sliceStack)-1]
	}
	p.sliceStack = append(p.sliceStack, info)

	// Close current source
	p.source.Close()

	// Open sliced file
	src, err := source.NewFileSource(cachePath)
	if err != nil {
		p.sliceStack = p.sliceStack[:len(p.sliceStack)-1]
		return err
	}

	// Update source
	p.source = src
	p.isCached = true

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&p.config.LogLevels)
	p.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	p.viewport.SetProvider(p.filteredSource)

	// Reset position and clear filters
	p.filteredSource.ClearFilter()
	p.viewport.GotoTop()

	// Clear search results
	p.ClearSearch()

	return nil
}

// RevertSlice returns to the parent file/slice
func (p *Pane) RevertSlice() error {
	if len(p.sliceStack) == 0 {
		return nil
	}

	// Get current slice info
	current := p.sliceStack[len(p.sliceStack)-1]
	p.sliceStack = p.sliceStack[:len(p.sliceStack)-1]

	// Cleanup current slice file
	p.slicer.Cleanup(current)

	// Close current source
	p.source.Close()

	// Determine which file to open
	var pathToOpen string
	if len(p.sliceStack) > 0 {
		pathToOpen = p.sliceStack[len(p.sliceStack)-1].CachePath
	} else {
		pathToOpen = p.sourcePath
		p.isCached = false
	}

	// Open the file
	src, err := source.NewFileSource(pathToOpen)
	if err != nil {
		return err
	}

	// Update source
	p.source = src

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&p.config.LogLevels)
	p.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	p.viewport.SetProvider(p.filteredSource)

	// Reset position
	p.viewport.GotoTop()

	// Clear search results
	p.ClearSearch()

	return nil
}

// GotoTime navigates to a specific time
func (p *Pane) GotoTime(timeStr string) bool {
	target := p.parseTimeInput(timeStr)
	if target == nil {
		return false
	}

	originalLine := p.source.FindLineAtTime(*target)
	if originalLine < 0 {
		return false
	}

	filteredIndex := p.filteredSource.FilteredIndexFor(originalLine)
	if filteredIndex >= 0 {
		p.viewport.GotoLine(filteredIndex)
		actualOriginal := p.filteredSource.OriginalLineNumber(filteredIndex)
		if actualOriginal >= 0 {
			p.viewport.SetHighlightedLine(actualOriginal)
		}
	}
	return true
}

// FilterTerm returns the current filter term
func (p *Pane) FilterTerm() string {
	return p.filterTerm
}

// SetFilterTerm sets the filter term
func (p *Pane) SetFilterTerm(term string) {
	p.filterTerm = term
}

// md5Sum helper for cache file naming
func md5Sum(data []byte) [16]byte {
	return md5.Sum(data)
}

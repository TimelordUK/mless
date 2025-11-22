package ui

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/config"
	"github.com/user/mless/internal/render"
	"github.com/user/mless/internal/slice"
	"github.com/user/mless/internal/source"
	"github.com/user/mless/internal/view"
	"github.com/user/mless/pkg/logformat"
)

// tickMsg is sent periodically in follow mode
type tickMsg time.Time

// ModelOptions contains options for creating a new model
type ModelOptions struct {
	Filepath  string
	CacheFile bool
}

// Mode represents the current UI mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeSearch
	ModeGoto
	ModeGotoTime
	ModeFilter
	ModeSlice
)

// Model is the main application model
type Model struct {
	viewport       *view.Viewport
	source         *source.FileSource
	filteredSource *source.FilteredProvider
	searchInput    textinput.Model
	config         *config.Config

	mode   Mode
	width  int
	height int

	// Search state
	searchTerm    string
	searchResults []int // line numbers with matches
	searchIndex   int   // current result index

	// Filter state (fzf-style)
	filterTerm string

	// Status
	filename string
	err      error

	// Cache state
	sourcePath string // Original file path
	cachePath  string // Cached file path (empty if not cached)
	isCached   bool

	// Follow mode
	following bool

	// Slice state
	slicer     *slice.Slicer
	sliceStack []*slice.Info // Stack for nested slices
}

// NewModel creates a new application model
func NewModel(filePath string) (*Model, error) {
	return NewModelWithOptions(ModelOptions{Filepath: filePath})
}

// NewModelWithOptions creates a new application model with options
func NewModelWithOptions(opts ModelOptions) (*Model, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	var actualPath string
	var cachePath string
	var isCached bool

	if opts.CacheFile {
		// Create cache directory
		cacheDir := os.TempDir()

		// Generate cache filename from source path hash
		hash := md5.Sum([]byte(opts.Filepath))
		baseName := filepath.Base(opts.Filepath)
		cachePath = filepath.Join(cacheDir, fmt.Sprintf("mless-%x-%s", hash[:8], baseName))

		// Copy file to cache
		if err := copyFile(opts.Filepath, cachePath); err != nil {
			return nil, fmt.Errorf("failed to cache file: %w", err)
		}

		actualPath = cachePath
		isCached = true
	} else {
		actualPath = opts.Filepath
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

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	return &Model{
		viewport:       viewport,
		source:         src,
		filteredSource: filtered,
		searchInput:    ti,
		config:         cfg,
		mode:           ModeNormal,
		filename:       filepath.Base(opts.Filepath),
		sourcePath:     opts.Filepath,
		cachePath:      cachePath,
		isCached:       isCached,
		slicer:         slice.NewSlicer(),
	}, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve 2 lines for status bar
		m.viewport.SetSize(msg.Width, msg.Height-2)
		return m, nil

	case tickMsg:
		if m.following {
			m.checkForNewLines()
			return m, m.tickCmd()
		}
		return m, nil
	}

	return m, nil
}

// tickCmd returns a command that sends a tick after a delay
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle mode-specific input
	if m.mode == ModeSearch {
		return m.handleSearchKey(msg)
	}
	if m.mode == ModeGoto {
		return m.handleGotoKey(msg)
	}
	if m.mode == ModeFilter {
		return m.handleFilterKey(msg)
	}
	if m.mode == ModeGotoTime {
		return m.handleGotoTimeKey(msg)
	}
	if m.mode == ModeSlice {
		return m.handleSliceKey(msg)
	}

	// Normal mode
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Clear all active modes/filters
		if m.following {
			m.following = false
		}
		if m.filteredSource.HasTextFilter() {
			m.filteredSource.ClearTextFilter()
			m.filterTerm = ""
		}
		if m.searchTerm != "" {
			m.searchTerm = ""
			m.searchResults = nil
			m.searchIndex = 0
			m.viewport.ClearHighlight()
		}

	case "j", "down":
		m.viewport.ScrollDown(1)
	case "k", "up":
		m.viewport.ScrollUp(1)

	case "ctrl+d", "ctrl+f":
		m.viewport.PageDown()
	case "ctrl+u", "ctrl+b":
		m.viewport.PageUp()

	case "f", "pgdown", " ":
		m.viewport.PageDown()
	case "b", "pgup":
		m.viewport.PageUp()

	case "g", "home":
		m.viewport.GotoTop()
	case "G", "end":
		// Refresh file to pick up any new content, then go to bottom
		m.source.Refresh()
		m.filteredSource.MarkDirty()
		m.viewport.GotoBottom()

	case "/":
		m.mode = ModeSearch
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, textinput.Blink

	case ":":
		m.mode = ModeGoto
		m.searchInput.SetValue("")
		m.searchInput.Placeholder = "Line number..."
		m.searchInput.Focus()
		return m, textinput.Blink

	case "ctrl+t":
		m.mode = ModeGotoTime
		m.searchInput.SetValue("")
		m.searchInput.Placeholder = "Time (HH:MM:SS or HH:MM)..."
		m.searchInput.Focus()
		return m, textinput.Blink

	case "?":
		m.mode = ModeFilter
		m.searchInput.SetValue("")
		m.searchInput.Placeholder = "Filter..."
		m.searchInput.Focus()
		return m, textinput.Blink

	case "n":
		m.nextSearchResult()
	case "N":
		m.prevSearchResult()

	case "l":
		// Toggle line numbers
		m.viewport.SetShowLineNumbers(true)

	// Level filtering: letters toggle levels
	case "t": // Trace
		m.filteredSource.ToggleLevel(source.LevelTrace)
		m.viewport.GotoTop()
	case "d": // Debug
		m.filteredSource.ToggleLevel(source.LevelDebug)
		m.viewport.GotoTop()
	case "i": // Info
		m.filteredSource.ToggleLevel(source.LevelInfo)
		m.viewport.GotoTop()
	case "w": // Warn
		m.filteredSource.ToggleLevel(source.LevelWarn)
		m.viewport.GotoTop()
	case "e": // Error
		m.filteredSource.ToggleLevel(source.LevelError)
		m.viewport.GotoTop()
	case "alt+f": // Fatal (use alt+f since F is for follow mode)
		m.filteredSource.ToggleLevel(source.LevelFatal)
		m.viewport.GotoTop()

	case "F": // Follow mode
		m.following = !m.following
		if m.following {
			m.viewport.GotoBottom()
			return m, m.tickCmd()
		}

	// Shift+letter: show this level and above
	case "T": // Trace and above (all)
		m.filteredSource.SetLevelAndAbove(source.LevelTrace)
		m.viewport.GotoTop()
	case "D": // Debug and above
		m.filteredSource.SetLevelAndAbove(source.LevelDebug)
		m.viewport.GotoTop()
	case "I": // Info and above
		m.filteredSource.SetLevelAndAbove(source.LevelInfo)
		m.viewport.GotoTop()
	case "W": // Warn and above
		m.filteredSource.SetLevelAndAbove(source.LevelWarn)
		m.viewport.GotoTop()
	case "E": // Error and above
		m.filteredSource.SetLevelAndAbove(source.LevelError)
		m.viewport.GotoTop()
	// Note: F is already used for fatal toggle, use ctrl+f for fatal-only if needed

	case "0": // Clear all filters
		m.filteredSource.ClearFilter()
		m.viewport.GotoTop()

	case "R": // Revert slice or resync from source
		if len(m.sliceStack) > 0 {
			m.revertSlice()
		} else if m.isCached {
			m.resyncFromSource()
		}

	case "ctrl+s": // Quick slice from current line to end
		m.sliceFromCurrent()

	case "S": // Enter slice mode for range input
		m.mode = ModeSlice
		m.searchInput.SetValue("")
		m.searchInput.Placeholder = "Range (e.g., 100-500 or -500 or 100-)..."
		m.searchInput.Focus()
		return m, textinput.Blink
	}

	return m, nil
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searchTerm = m.searchInput.Value()
		m.performSearch()
		m.mode = ModeNormal
		m.searchInput.Blur()
		return m, nil

	case "esc":
		m.mode = ModeNormal
		m.searchInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m *Model) handleGotoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		var lineNum int
		fmt.Sscanf(m.searchInput.Value(), "%d", &lineNum)
		if lineNum > 0 {
			m.viewport.GotoLine(lineNum - 1) // Convert to 0-based
		}
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil

	case "esc":
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m *Model) handleGotoTimeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		timeStr := m.searchInput.Value()
		if target := m.parseTimeInput(timeStr); target != nil {
			originalLine := m.source.FindLineAtTime(*target)
			if originalLine >= 0 {
				// Map original line to filtered index
				filteredIndex := m.filteredSource.FilteredIndexFor(originalLine)
				if filteredIndex >= 0 {
					m.viewport.GotoLine(filteredIndex)
					// Highlight using the actual original line at that filtered position
					actualOriginal := m.filteredSource.OriginalLineNumber(filteredIndex)
					if actualOriginal >= 0 {
						m.viewport.SetHighlightedLine(actualOriginal)
					}
				}
			}
		}
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil

	case "esc":
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// parseTimeInput parses user time input into a time.Time
func (m *Model) parseTimeInput(input string) *time.Time {
	// Try various formats
	layouts := []string{
		"15:04:05",
		"15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, input); err == nil {
			// For time-only formats, use the date from the first line's timestamp
			if layout == "15:04:05" || layout == "15:04" {
				// Get reference date from first line
				if firstTs := m.source.GetTimestamp(0); firstTs != nil {
					t = time.Date(firstTs.Year(), firstTs.Month(), firstTs.Day(),
						t.Hour(), t.Minute(), t.Second(), 0, firstTs.Location())
				} else {
					// Fallback to today
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

func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Keep filter and return to normal mode
		m.filterTerm = m.searchInput.Value()
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil

	case "esc":
		// Cancel filter and clear
		m.filteredSource.ClearTextFilter()
		m.filterTerm = ""
		m.viewport.GotoTop()
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil
	}

	// Update input and apply filter live
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)

	// Apply filter immediately (live filtering)
	m.filteredSource.SetTextFilter(m.searchInput.Value())
	m.viewport.GotoTop()

	return m, cmd
}

func (m *Model) performSearch() {
	if m.searchTerm == "" {
		m.searchResults = nil
		return
	}

	// Simple search - find all lines containing the term
	m.searchResults = nil
	for i := 0; i < m.source.LineCount(); i++ {
		line, err := m.source.GetLine(i)
		if err != nil {
			continue
		}
		if strings.Contains(string(line.Content), m.searchTerm) {
			m.searchResults = append(m.searchResults, i)
		}
	}

	// Jump to first result
	if len(m.searchResults) > 0 {
		m.searchIndex = 0
		m.viewport.GotoLine(m.searchResults[0])
		m.viewport.SetHighlightedLine(m.searchResults[0])
	} else {
		m.viewport.ClearHighlight()
	}
}

func (m *Model) nextSearchResult() {
	if len(m.searchResults) == 0 {
		return
	}
	m.searchIndex = (m.searchIndex + 1) % len(m.searchResults)
	m.viewport.GotoLine(m.searchResults[m.searchIndex])
	m.viewport.SetHighlightedLine(m.searchResults[m.searchIndex])
}

func (m *Model) prevSearchResult() {
	if len(m.searchResults) == 0 {
		return
	}
	m.searchIndex--
	if m.searchIndex < 0 {
		m.searchIndex = len(m.searchResults) - 1
	}
	m.viewport.GotoLine(m.searchResults[m.searchIndex])
	m.viewport.SetHighlightedLine(m.searchResults[m.searchIndex])
}

// checkForNewLines checks if file has grown and updates view
func (m *Model) checkForNewLines() {
	newLines, err := m.source.Refresh()
	if err != nil {
		m.err = err
		return
	}

	if newLines > 0 {
		// Mark filter as dirty to rebuild index with new lines
		m.filteredSource.MarkDirty()

		// Auto-scroll to bottom in follow mode
		m.viewport.GotoBottom()
	}
}

// resyncFromSource re-copies the source file to cache and reloads
func (m *Model) resyncFromSource() {
	if !m.isCached || m.sourcePath == "" || m.cachePath == "" {
		return
	}

	// Close current source
	m.source.Close()

	// Re-copy from source
	if err := copyFile(m.sourcePath, m.cachePath); err != nil {
		m.err = err
		return
	}

	// Reopen the cached file
	src, err := source.NewFileSource(m.cachePath)
	if err != nil {
		m.err = err
		return
	}

	// Update the source
	m.source = src

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&m.config.LogLevels)
	m.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	m.viewport.SetProvider(m.filteredSource)

	// Reset position
	m.viewport.GotoTop()

	// Clear search results (line numbers may have changed)
	m.searchResults = nil
	m.searchTerm = ""
	m.viewport.ClearHighlight()
}

func (m *Model) handleSliceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		rangeStr := m.searchInput.Value()
		m.parseAndSlice(rangeStr)
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil

	case "esc":
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// parseAndSlice parses a range string and performs the slice
func (m *Model) parseAndSlice(rangeStr string) {
	// Get current position (original line number)
	currentFiltered := m.viewport.CurrentLine()
	currentLine := m.filteredSource.OriginalLineNumber(currentFiltered)
	if currentLine < 0 {
		currentLine = 0
	}
	totalLines := m.source.LineCount()

	// Parse range - split on first dash that's not part of an offset
	// Format: start-end where start/end can be:
	//   . = current position
	//   $ = end of file
	//   $-N = end minus N
	//   N = absolute line number

	var start, end int
	var startStr, endStr string

	// Find the separator dash (not one that's part of $-N)
	dashIdx := -1
	for i := 0; i < len(rangeStr); i++ {
		if rangeStr[i] == '-' {
			// Check if this dash is part of $-N
			if i > 0 && rangeStr[i-1] == '$' {
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
		// Single value - from that point to end
		startStr = rangeStr
		endStr = "$"
	}

	start = m.parseLineRef(startStr, currentLine, totalLines)
	end = m.parseLineRef(endStr, currentLine, totalLines)

	if start < 0 {
		start = 0
	}
	if end > totalLines {
		end = totalLines
	}

	m.performSlice(start, end)
}

// parseLineRef parses a line reference like ".", "$", "$-100", or "500"
func (m *Model) parseLineRef(ref string, current, total int) int {
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

	// Handle $-N or $+N
	if strings.HasPrefix(ref, "$") {
		offset := 0
		fmt.Sscanf(ref[1:], "%d", &offset)
		return total + offset // offset is negative for $-100
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

// sliceFromCurrent slices from current viewport line to end
func (m *Model) sliceFromCurrent() {
	// Get original line number for current position
	currentFiltered := m.viewport.CurrentLine()
	originalLine := m.filteredSource.OriginalLineNumber(currentFiltered)
	if originalLine < 0 {
		originalLine = 0
	}

	m.performSlice(originalLine, m.source.LineCount())
}

// performSlice executes a slice operation and switches to the sliced file
func (m *Model) performSlice(start, end int) {
	info, cachePath, err := m.slicer.SliceRange(m.source, start, end)
	if err != nil {
		m.err = err
		return
	}

	// Track parent slice info
	if len(m.sliceStack) > 0 {
		info.Parent = m.sliceStack[len(m.sliceStack)-1]
	}
	m.sliceStack = append(m.sliceStack, info)

	// Close current source
	m.source.Close()

	// Open sliced file
	src, err := source.NewFileSource(cachePath)
	if err != nil {
		m.err = err
		// Pop the failed slice
		m.sliceStack = m.sliceStack[:len(m.sliceStack)-1]
		return
	}

	// Update source
	m.source = src
	m.isCached = true

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&m.config.LogLevels)
	m.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	m.viewport.SetProvider(m.filteredSource)

	// Reset position and clear filters
	m.filteredSource.ClearFilter()
	m.viewport.GotoTop()

	// Clear search results
	m.searchResults = nil
	m.searchTerm = ""
	m.viewport.ClearHighlight()
}

// revertSlice returns to the parent file/slice
func (m *Model) revertSlice() {
	if len(m.sliceStack) == 0 {
		return
	}

	// Get current slice info
	current := m.sliceStack[len(m.sliceStack)-1]
	m.sliceStack = m.sliceStack[:len(m.sliceStack)-1]

	// Cleanup current slice file
	m.slicer.Cleanup(current)

	// Close current source
	m.source.Close()

	// Determine which file to open
	var pathToOpen string
	if len(m.sliceStack) > 0 {
		// Open parent slice
		pathToOpen = m.sliceStack[len(m.sliceStack)-1].CachePath
	} else {
		// Open original file
		pathToOpen = m.sourcePath
		m.isCached = false
	}

	// Open the file
	src, err := source.NewFileSource(pathToOpen)
	if err != nil {
		m.err = err
		return
	}

	// Update source
	m.source = src

	// Recreate filtered provider
	detector := logformat.NewLevelDetector(&m.config.LogLevels)
	m.filteredSource = source.NewFilteredProvider(src, detector.Detect)
	m.viewport.SetProvider(m.filteredSource)

	// Reset position
	m.viewport.GotoTop()

	// Clear search results
	m.searchResults = nil
	m.searchTerm = ""
	m.viewport.ClearHighlight()
}

// View implements tea.Model
func (m *Model) View() string {
	var builder strings.Builder

	// Main content
	builder.WriteString(m.viewport.Render())
	builder.WriteString("\n")

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("255")).
		Width(m.width)

	var status string
	switch m.mode {
	case ModeSearch:
		status = "/" + m.searchInput.View()
	case ModeGoto:
		status = ":" + m.searchInput.View()
	case ModeGotoTime:
		status = "t:" + m.searchInput.View()
	case ModeFilter:
		status = "?" + m.searchInput.View()
	case ModeSlice:
		status = "S:" + m.searchInput.View()
	default:
		// Show filtered count vs total if filter is active
		var lineInfo string
		if m.filteredSource.IsFiltered() {
			lineInfo = fmt.Sprintf("L%d/%d (of %d)",
				m.viewport.CurrentLine()+1,
				m.filteredSource.LineCount(),
				m.source.LineCount())
		} else {
			lineInfo = fmt.Sprintf("L%d/%d",
				m.viewport.CurrentLine()+1,
				m.source.LineCount())
		}

		percent := fmt.Sprintf("%.0f%%", m.viewport.PercentScrolled())

		searchInfo := ""
		if m.searchTerm != "" {
			searchInfo = fmt.Sprintf(" [%d matches]", len(m.searchResults))
		}

		// Show active filters
		filterInfo := ""
		if m.filteredSource.IsFiltered() {
			var parts []string

			// Level filters
			filters := m.filteredSource.GetActiveFilters()
			levelNames := map[source.LogLevel]string{
				source.LevelTrace: "TRC",
				source.LevelDebug: "DBG",
				source.LevelInfo:  "INF",
				source.LevelWarn:  "WRN",
				source.LevelError: "ERR",
				source.LevelFatal: "FTL",
			}
			var levels []string
			for level, active := range filters {
				if active {
					levels = append(levels, levelNames[level])
				}
			}
			if len(levels) > 0 {
				parts = append(parts, strings.Join(levels, ","))
			}

			// Text filter
			if m.filteredSource.HasTextFilter() {
				text := m.filteredSource.GetTextFilter()
				if len(text) > 15 {
					text = text[:15] + "..."
				}
				parts = append(parts, "\""+text+"\"")
			}

			if len(parts) > 0 {
				filterInfo = fmt.Sprintf(" [%s]", strings.Join(parts, " "))
			}
		}

		// Slice/cached indicator
		sliceInfo := ""
		if len(m.sliceStack) > 0 {
			current := m.sliceStack[len(m.sliceStack)-1]
			sliceInfo = fmt.Sprintf(" [slice:%d-%d]", current.StartLine+1, current.EndLine)
		} else if m.isCached {
			sliceInfo = " [cached]"
		}

		// Follow indicator
		followInfo := ""
		if m.following {
			followInfo = " [following]"
		}

		// Get timestamp for current line
		timeInfo := ""
		currentLine := m.viewport.CurrentLine()
		if ts := m.source.GetTimestamp(currentLine); ts != nil {
			timeInfo = fmt.Sprintf(" %s", ts.Format("15:04:05"))
		}

		status = fmt.Sprintf(" %s%s%s  %s%s  %s%s%s",
			m.filename, sliceInfo, followInfo, lineInfo, timeInfo, percent, searchInfo, filterInfo)
	}

	builder.WriteString(statusStyle.Render(status))
	builder.WriteString("\n")

	// Help line
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "j/k:scroll  /:search  ?:filter  t/d/i/w/e:level  T/D/I/W/E:lvl+  0:clear  q:quit"
	builder.WriteString(helpStyle.Render(help))

	return builder.String()
}

// Close cleans up resources
func (m *Model) Close() error {
	var err error
	if m.source != nil {
		err = m.source.Close()
	}

	// Delete cached file
	if m.cachePath != "" {
		os.Remove(m.cachePath)
	}

	return err
}

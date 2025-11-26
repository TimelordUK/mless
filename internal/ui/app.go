package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/TimelordUK/mless/internal/config"
	"github.com/TimelordUK/mless/internal/render"
	"github.com/TimelordUK/mless/internal/source"
	"github.com/TimelordUK/mless/internal/view"
	"github.com/TimelordUK/mless/pkg/logformat"
)

// tickMsg is sent periodically in follow mode
type tickMsg time.Time

// ModelOptions contains options for creating a new model
type ModelOptions struct {
	Filepath   string
	Filepaths  []string // Multiple files for split view
	CacheFile  bool
	SliceRange string // e.g., "1000-5000"
	GotoTime   string // e.g., "14:00"
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
	ModeMarkSet  // Waiting for mark character (ma-mz)
	ModeMarkJump // Waiting for mark character ('a-'z)
	ModeHelp
	ModeFileInfo // Showing file info (ctrl+g)
	ModeSplitCmd // Waiting for split command (v, s, w, q, etc.)
	ModeYank     // Waiting for yank target (y for line, number, or 'a for mark)
	ModeVisual   // Visual selection mode
)

// SplitDirection represents the split layout direction
type SplitDirection int

const (
	SplitNone SplitDirection = iota
	SplitVertical   // side-by-side |
	SplitHorizontal // stacked -
)

// Model is the main application model
type Model struct {
	panes      []*Pane
	activePane int
	splitDir   SplitDirection
	splitRatio float64 // 0.0 to 1.0, proportion for first pane (default 0.5)

	searchInput textinput.Model
	config      *config.Config

	mode   Mode
	width  int
	height int

	// Command count prefix (e.g., 5j, 10yy)
	countPrefix int

	// Status
	err     error
	message string // Temporary status message (e.g., "5 lines yanked")
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

	// Build list of files to open
	var files []string
	if len(opts.Filepaths) > 0 {
		files = opts.Filepaths
	} else if opts.Filepath != "" {
		files = []string{opts.Filepath}
	} else {
		return nil, fmt.Errorf("no file specified")
	}

	// Create panes for each file
	var panes []*Pane
	for _, filePath := range files {
		pane, err := NewPane(filePath, cfg, opts.CacheFile)
		if err != nil {
			// Clean up already created panes
			for _, p := range panes {
				p.Close()
			}
			return nil, err
		}
		panes = append(panes, pane)
	}

	// Apply initial slice to first pane if specified
	if opts.SliceRange != "" && len(panes) > 0 {
		if err := panes[0].ParseAndSlice(opts.SliceRange); err != nil {
			for _, p := range panes {
				p.Close()
			}
			return nil, fmt.Errorf("invalid slice range: %w", err)
		}
	}

	// Apply initial time navigation to first pane if specified
	if opts.GotoTime != "" && len(panes) > 0 {
		panes[0].GotoTime(opts.GotoTime)
	}

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	// Determine split direction based on number of files
	splitDir := SplitNone
	if len(panes) > 1 {
		splitDir = SplitVertical
	}

	return &Model{
		panes:       panes,
		activePane:  0,
		splitDir:    splitDir,
		splitRatio:  0.5,
		searchInput: ti,
		config:      cfg,
		mode:        ModeNormal,
	}, nil
}

// activePane returns the currently active pane
func (m *Model) currentPane() *Pane {
	return m.panes[m.activePane]
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
		m.calculatePaneSizes()
		return m, nil

	case tickMsg:
		if m.currentPane().IsFollowing() {
			m.currentPane().CheckForNewLines()
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
	// Clear any temporary message
	m.message = ""

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
	if m.mode == ModeMarkSet {
		return m.handleMarkSetKey(msg)
	}
	if m.mode == ModeMarkJump {
		return m.handleMarkJumpKey(msg)
	}
	if m.mode == ModeHelp {
		// Any key exits help
		m.mode = ModeNormal
		return m, nil
	}
	if m.mode == ModeFileInfo {
		// Any key exits file info
		m.mode = ModeNormal
		return m, nil
	}
	if m.mode == ModeSplitCmd {
		return m.handleSplitCmd(msg)
	}
	if m.mode == ModeYank {
		return m.handleYankKey(msg)
	}
	if m.mode == ModeVisual {
		return m.handleVisualKey(msg)
	}

	// Normal mode
	pane := m.currentPane()
	key := msg.String()

	// Handle digit prefix for counts (1-9 to start, 0-9 to continue)
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		digit := int(key[0] - '0')
		if m.countPrefix > 0 || digit > 0 { // Don't start with 0
			m.countPrefix = m.countPrefix*10 + digit
			return m, nil
		}
	}

	// Get count and reset
	count := m.countPrefix
	if count == 0 {
		count = 1
	}
	m.countPrefix = 0

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Clear all active modes/filters
		if pane.IsFollowing() {
			pane.SetFollowing(false)
		}
		if pane.FilteredSource().HasTextFilter() {
			pane.FilteredSource().ClearTextFilter()
			pane.SetFilterTerm("")
		}
		if pane.SearchTerm() != "" {
			pane.ClearSearch()
		}
		// Clear highlighted line
		pane.Viewport().SetHighlightedLine(-1)

	case "j", "down":
		pane.Viewport().ScrollDown(count)
	case "k", "up":
		pane.Viewport().ScrollUp(count)

	case "left", "<":
		pane.Viewport().ScrollLeft(10)
	case "right", ">":
		pane.Viewport().ScrollRight(10)
	case "^": // Reset horizontal scroll
		pane.Viewport().ResetHorizontalScroll()
	case "Z": // Toggle line wrap
		pane.Viewport().ToggleWrap()

	case "ctrl+d", "ctrl+f":
		pane.Viewport().PageDown()
	case "ctrl+u", "ctrl+b":
		pane.Viewport().PageUp()

	case "f", "pgdown", " ":
		pane.Viewport().PageDown()
	case "b", "pgup":
		pane.Viewport().PageUp()

	case "g", "home":
		pane.Viewport().GotoTop()
	case "G", "end":
		// Refresh file to pick up any new content, then go to bottom
		pane.Source().Refresh()
		pane.FilteredSource().MarkDirty()
		pane.Viewport().GotoBottom()

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
		pane.NextSearchResult()
	case "N":
		pane.PrevSearchResult()

	case "l":
		// Toggle line numbers
		pane.Viewport().SetShowLineNumbers(true)

	// Level filtering: letters toggle levels
	case "t": // Trace
		pane.FilteredSource().ToggleLevel(source.LevelTrace)
		pane.Viewport().GotoTop()
	case "d": // Debug
		pane.FilteredSource().ToggleLevel(source.LevelDebug)
		pane.Viewport().GotoTop()
	case "i": // Info
		pane.FilteredSource().ToggleLevel(source.LevelInfo)
		pane.Viewport().GotoTop()
	case "w": // Warn
		pane.FilteredSource().ToggleLevel(source.LevelWarn)
		pane.Viewport().GotoTop()
	case "e": // Error
		pane.FilteredSource().ToggleLevel(source.LevelError)
		pane.Viewport().GotoTop()
	case "alt+f": // Fatal (use alt+f since F is for follow mode)
		pane.FilteredSource().ToggleLevel(source.LevelFatal)
		pane.Viewport().GotoTop()

	case "F": // Follow mode
		if pane.ToggleFollowing() {
			pane.Viewport().GotoBottom()
			return m, m.tickCmd()
		}

	// Shift+letter: show this level and above
	case "T": // Trace and above (all)
		pane.FilteredSource().SetLevelAndAbove(source.LevelTrace)
		pane.Viewport().GotoTop()
	case "D": // Debug and above
		pane.FilteredSource().SetLevelAndAbove(source.LevelDebug)
		pane.Viewport().GotoTop()
	case "I": // Info and above
		pane.FilteredSource().SetLevelAndAbove(source.LevelInfo)
		pane.Viewport().GotoTop()
	case "W": // Warn and above
		pane.FilteredSource().SetLevelAndAbove(source.LevelWarn)
		pane.Viewport().GotoTop()
	case "E": // Error and above
		pane.FilteredSource().SetLevelAndAbove(source.LevelError)
		pane.Viewport().GotoTop()
	// Note: F is already used for fatal toggle, use ctrl+f for fatal-only if needed

	case "0": // Clear all filters, preserve position
		// Remember current original line before clearing
		currentFiltered := pane.Viewport().CurrentLine()
		originalLine := pane.FilteredSource().OriginalLineNumber(currentFiltered)

		pane.FilteredSource().ClearFilter()

		// Jump back to the same original line in unfiltered view
		if originalLine >= 0 {
			filteredIdx := pane.FilteredSource().FilteredIndexFor(originalLine)
			if filteredIdx >= 0 {
				pane.Viewport().GotoLine(filteredIdx)
			}
		}

	case "R": // Revert slice or resync from source
		if pane.HasSlice() {
			pane.RevertSlice()
		} else if pane.IsCached() {
			pane.ResyncFromSource()
		}

	case "ctrl+s": // Quick slice from current line to end
		pane.SliceFromCurrent()

	case "S": // Enter slice mode for range input
		m.mode = ModeSlice
		m.searchInput.SetValue("")
		m.searchInput.Placeholder = "Range (e.g., 'a-'b, 13:00-14:00, 100-500)..."
		m.searchInput.Focus()
		return m, textinput.Blink

	case "m": // Enter mark set mode
		m.mode = ModeMarkSet

	case "M": // Clear all marks
		pane.ClearMarks()

	case "'": // Enter mark jump mode
		m.mode = ModeMarkJump

	case "]'": // Next mark
		pane.NextMark()

	case "['": // Previous mark
		pane.PrevMark()

	case "h": // Show help
		m.mode = ModeHelp

	case "ctrl+g": // Show file info
		m.mode = ModeFileInfo

	case "ctrl+w": // Enter split command mode
		m.mode = ModeSplitCmd

	case "tab": // Quick pane switch
		if len(m.panes) > 1 {
			m.activePane = (m.activePane + 1) % len(m.panes)
		}

	// Split resizing
	case "H": // Shrink first pane (move splitter left/up)
		if len(m.panes) > 1 {
			m.splitRatio -= 0.05
			if m.splitRatio < 0.1 {
				m.splitRatio = 0.1
			}
			m.calculatePaneSizes()
		}
	case "L": // Grow first pane (move splitter right/down)
		if len(m.panes) > 1 {
			m.splitRatio += 0.05
			if m.splitRatio > 0.9 {
				m.splitRatio = 0.9
			}
			m.calculatePaneSizes()
		}
	case "=": // Reset split to 50/50
		if len(m.panes) > 1 {
			m.splitRatio = 0.5
			m.calculatePaneSizes()
		}

	case "ctrl+o": // Toggle split orientation
		if len(m.panes) > 1 {
			if m.splitDir == SplitVertical {
				m.splitDir = SplitHorizontal
			} else {
				m.splitDir = SplitVertical
			}
			m.calculatePaneSizes()
		}

	case "y": // Enter yank mode (count already captured)
		m.mode = ModeYank
		// Store count for yank mode to use
		m.countPrefix = count

	case "Y": // Quick yank current line (with count)
		m.yankLines(count)

	case "v": // Enter visual mode
		pane.StartVisualSelection()
		m.mode = ModeVisual
	}

	return m, nil
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.currentPane().PerformSearch(m.searchInput.Value())
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
			m.currentPane().Viewport().GotoLine(lineNum - 1) // Convert to 0-based
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
		result := m.currentPane().GotoTime(m.searchInput.Value())
		if result.Found && result.Actual != nil {
			m.message = fmt.Sprintf("Target %s -> %s",
				result.Target.Format("15:04:05"),
				result.Actual.Format("2006-01-02 15:04:05"))
		} else if result.Target != nil {
			m.message = fmt.Sprintf("No line found near %s", result.Target.Format("2006-01-02 15:04:05"))
		} else {
			m.message = "Invalid time format"
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


func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pane := m.currentPane()
	switch msg.String() {
	case "enter":
		// Keep filter and return to normal mode
		pane.SetFilterTerm(m.searchInput.Value())
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil

	case "esc":
		// Cancel filter and clear
		pane.FilteredSource().ClearTextFilter()
		pane.SetFilterTerm("")
		pane.Viewport().GotoTop()
		m.mode = ModeNormal
		m.searchInput.Blur()
		m.searchInput.Placeholder = "Search..."
		return m, nil
	}

	// Update input and apply filter live
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)

	// Apply filter immediately (live filtering)
	pane.FilteredSource().SetTextFilter(m.searchInput.Value())
	pane.Viewport().GotoTop()

	return m, cmd
}


func (m *Model) handleSliceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if err := m.currentPane().ParseAndSlice(m.searchInput.Value()); err != nil {
			m.err = err
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

func (m *Model) handleMarkSetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.mode = ModeNormal

	// Check if it's a valid mark character (a-z)
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		m.currentPane().SetMark(rune(key[0]))
	}

	return m, nil
}

func (m *Model) handleMarkJumpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	m.mode = ModeNormal

	// Check if it's a valid mark character (a-z)
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		m.currentPane().JumpToMark(rune(key[0]))
	}

	return m, nil
}

func (m *Model) handleSplitCmd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = ModeNormal

	switch msg.String() {
	case "v": // Vertical split (side-by-side)
		m.splitVertical()
	case "s": // Horizontal split (stacked)
		m.splitHorizontal()
	case "w": // Switch pane
		if len(m.panes) > 1 {
			m.activePane = (m.activePane + 1) % len(m.panes)
		}
	case "q": // Close current pane
		m.closeCurrentPane()
	case "esc": // Cancel
		// Just return to normal mode
	}

	return m, nil
}

func (m *Model) handleYankKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Get count from normal mode prefix
	count := m.countPrefix
	if count == 0 {
		count = 1
	}

	switch key {
	case "y": // yy or Nyy - yank lines
		m.mode = ModeNormal
		m.yankLines(count)
		m.countPrefix = 0

	case "'": // y'a - yank to mark
		// Stay in yank mode for mark character
		m.countPrefix = -1 // Signal waiting for mark

	case "esc":
		m.mode = ModeNormal
		m.countPrefix = 0

	default:
		// Check if it's a mark character after y'
		if m.countPrefix == -1 && len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			m.mode = ModeNormal
			m.yankToMark(rune(key[0]))
			m.countPrefix = 0
		} else {
			m.mode = ModeNormal
			m.countPrefix = 0
		}
	}

	return m, nil
}

// yankLines yanks N lines from current position to clipboard
func (m *Model) yankLines(count int) {
	pane := m.currentPane()
	startFiltered := pane.Viewport().CurrentLine()

	var lines []string
	for i := 0; i < count; i++ {
		line, err := pane.FilteredSource().GetLine(startFiltered + i)
		if err != nil || line == nil {
			break
		}
		lines = append(lines, string(line.Content))
	}

	if len(lines) > 0 {
		text := strings.Join(lines, "\n")
		m.copyToClipboard(text)
		if len(lines) == 1 {
			m.message = "1 line yanked"
		} else {
			m.message = fmt.Sprintf("%d lines yanked", len(lines))
		}
	}
}

// yankToMark yanks from current line to mark
func (m *Model) yankToMark(markChar rune) {
	pane := m.currentPane()

	markLine, ok := pane.marks[markChar]
	if !ok {
		return
	}

	// Get current original line
	currentFiltered := pane.Viewport().CurrentLine()
	currentOriginal := pane.FilteredSource().OriginalLineNumber(currentFiltered)

	// Determine range
	startOriginal := currentOriginal
	endOriginal := markLine
	if startOriginal > endOriginal {
		startOriginal, endOriginal = endOriginal, startOriginal
	}

	// Collect lines from filtered view that fall in this range
	var lines []string
	for i := 0; i < pane.FilteredSource().LineCount(); i++ {
		line, err := pane.FilteredSource().GetLine(i)
		if err != nil || line == nil {
			continue
		}
		origIdx := pane.FilteredSource().OriginalLineNumber(i)
		if origIdx >= startOriginal && origIdx <= endOriginal {
			lines = append(lines, string(line.Content))
		}
	}

	if len(lines) > 0 {
		text := strings.Join(lines, "\n")
		m.copyToClipboard(text)
		if len(lines) == 1 {
			m.message = "1 line yanked"
		} else {
			m.message = fmt.Sprintf("%d lines yanked", len(lines))
		}
	}
}

// copyToClipboard copies text to system clipboard
func (m *Model) copyToClipboard(text string) {
	// Try different clipboard commands based on OS
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Check for WSL first (clip.exe works in WSL to access Windows clipboard)
		if _, err := exec.LookPath("clip.exe"); err == nil {
			cmd = exec.Command("clip.exe")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			// Fallback: try wl-copy for Wayland
			cmd = exec.Command("wl-copy")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return
	}

	if cmd == nil {
		return
	}

	cmd.Stdin = strings.NewReader(text)
	cmd.Run()
}

func (m *Model) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pane := m.currentPane()
	key := msg.String()

	// Handle digit prefix for counts (1-9 to start, 0-9 to continue)
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		digit := int(key[0] - '0')
		if m.countPrefix > 0 || digit > 0 {
			m.countPrefix = m.countPrefix*10 + digit
			return m, nil
		}
	}

	// Get count and reset
	count := m.countPrefix
	if count == 0 {
		count = 1
	}
	m.countPrefix = 0

	switch key {
	case "j", "down":
		m.visualMoveDown(pane, count)

	case "k", "up":
		m.visualMoveUp(pane, count)

	case "g": // Go to top
		pane.ResetCursorOffset()
		pane.Viewport().GotoTop()

	case "G": // Go to bottom
		pane.Viewport().GotoBottom()
		// Set cursor to last visible line
		maxOffset := pane.Viewport().Height() - 1
		lineCount := pane.FilteredSource().LineCount()
		topLine := pane.Viewport().CurrentLine()
		visibleLines := lineCount - topLine
		if visibleLines < maxOffset+1 {
			maxOffset = visibleLines - 1
		}
		if maxOffset < 0 {
			maxOffset = 0
		}
		pane.SetCursorOffset(maxOffset)

	case "f", "ctrl+d", "ctrl+f": // Page down
		pane.ResetCursorOffset()
		pane.Viewport().PageDown()

	case "b", "ctrl+u", "ctrl+b": // Page up
		pane.ResetCursorOffset()
		pane.Viewport().PageUp()

	case "y": // Yank visual selection
		m.yankVisualSelection()
		pane.ClearVisualSelection()
		pane.ResetCursorOffset()
		m.mode = ModeNormal

	case "v", "esc": // Exit visual mode
		pane.ClearVisualSelection()
		pane.ResetCursorOffset()
		m.mode = ModeNormal
	}

	return m, nil
}

// visualMoveDown handles j/down in visual mode with boundary awareness
func (m *Model) visualMoveDown(pane *Pane, count int) {
	viewport := pane.Viewport()
	lineCount := pane.FilteredSource().LineCount()
	maxOffset := viewport.Height() - 1

	for i := 0; i < count; i++ {
		// Can we scroll the viewport?
		if viewport.CanScrollDown() {
			viewport.ScrollDown(1)
		} else {
			// At scroll boundary - increase cursor offset instead
			cursorFiltered := viewport.CurrentLine() + pane.CursorOffset()
			if cursorFiltered < lineCount-1 && pane.CursorOffset() < maxOffset {
				pane.SetCursorOffset(pane.CursorOffset() + 1)
			}
		}
	}
}

// visualMoveUp handles k/up in visual mode with boundary awareness
func (m *Model) visualMoveUp(pane *Pane, count int) {
	viewport := pane.Viewport()

	for i := 0; i < count; i++ {
		// If we have a cursor offset, reduce it first
		if pane.CursorOffset() > 0 {
			pane.SetCursorOffset(pane.CursorOffset() - 1)
		} else {
			// No cursor offset - scroll the viewport
			viewport.ScrollUp(1)
		}
	}
}

// yankVisualSelection yanks the lines in the visual selection to clipboard
func (m *Model) yankVisualSelection() {
	pane := m.currentPane()
	startOrig, endOrig := pane.GetVisualSelectionRange()
	if startOrig < 0 || endOrig < 0 {
		return
	}

	// Collect lines from filtered view that fall in this range
	var lines []string
	for i := 0; i < pane.FilteredSource().LineCount(); i++ {
		line, err := pane.FilteredSource().GetLine(i)
		if err != nil || line == nil {
			continue
		}
		origIdx := pane.FilteredSource().OriginalLineNumber(i)
		if origIdx >= startOrig && origIdx <= endOrig {
			lines = append(lines, string(line.Content))
		}
	}

	if len(lines) > 0 {
		text := strings.Join(lines, "\n")
		m.copyToClipboard(text)
		if len(lines) == 1 {
			m.message = "1 line yanked"
		} else {
			m.message = fmt.Sprintf("%d lines yanked", len(lines))
		}
	}
}

// splitVertical creates a vertical split (side-by-side panes)
func (m *Model) splitVertical() {
	if len(m.panes) >= 2 {
		return // Already have max panes
	}

	current := m.currentPane()

	// Create new pane sharing the same source
	detector := logformat.NewLevelDetector(&m.config.LogLevels)
	newPane := &Pane{
		viewport:       view.NewViewport(80, 24),
		source:         current.source, // Shared source
		filteredSource: source.NewFilteredProvider(current.source, detector.Detect),
		config:         current.config,
		filename:       current.filename,
		sourcePath:     current.sourcePath,
		cachePath:      current.cachePath,
		isCached:       current.isCached,
		marks:          make(map[rune]int),
		visualAnchor:   -1,
	}
	newPane.viewport.SetProvider(newPane.filteredSource)
	newPane.viewport.SetRenderer(render.NewLogLevelRenderer(m.config))
	newPane.viewport.GotoLine(current.viewport.CurrentLine())

	m.panes = append(m.panes, newPane)
	m.splitDir = SplitVertical
	m.calculatePaneSizes()
}

// splitHorizontal creates a horizontal split (stacked panes)
func (m *Model) splitHorizontal() {
	if len(m.panes) >= 2 {
		return
	}

	current := m.currentPane()

	detector := logformat.NewLevelDetector(&m.config.LogLevels)
	newPane := &Pane{
		viewport:       view.NewViewport(80, 24),
		source:         current.source,
		filteredSource: source.NewFilteredProvider(current.source, detector.Detect),
		config:         current.config,
		filename:       current.filename,
		sourcePath:     current.sourcePath,
		cachePath:      current.cachePath,
		isCached:       current.isCached,
		marks:          make(map[rune]int),
		visualAnchor:   -1,
	}
	newPane.viewport.SetProvider(newPane.filteredSource)
	newPane.viewport.SetRenderer(render.NewLogLevelRenderer(m.config))
	newPane.viewport.GotoLine(current.viewport.CurrentLine())

	m.panes = append(m.panes, newPane)
	m.splitDir = SplitHorizontal
	m.calculatePaneSizes()
}

// closeCurrentPane closes the active pane
func (m *Model) closeCurrentPane() {
	if len(m.panes) <= 1 {
		return // Can't close the last pane
	}

	// Don't close the source if other panes are using it
	closingPane := m.panes[m.activePane]
	sharedSource := false
	for i, p := range m.panes {
		if i != m.activePane && p.source == closingPane.source {
			sharedSource = true
			break
		}
	}

	// Remove the pane
	m.panes = append(m.panes[:m.activePane], m.panes[m.activePane+1:]...)

	// Adjust active pane index
	if m.activePane >= len(m.panes) {
		m.activePane = len(m.panes) - 1
	}

	// Reset split direction if only one pane left
	if len(m.panes) == 1 {
		m.splitDir = SplitNone
	}

	// Close the pane (but not the shared source)
	if !sharedSource {
		closingPane.Close()
	}

	m.calculatePaneSizes()
}

// calculatePaneSizes sets the dimensions for each pane
func (m *Model) calculatePaneSizes() {
	statusHeight := 2 // status bar + help line
	contentHeight := m.height - statusHeight

	if len(m.panes) == 1 {
		m.panes[0].SetSize(m.width, contentHeight)
		return
	}

	switch m.splitDir {
	case SplitVertical:
		// Side by side, leave 1 char for separator
		firstWidth := int(float64(m.width-1) * m.splitRatio)
		if firstWidth < 10 {
			firstWidth = 10
		}
		if firstWidth > m.width-11 {
			firstWidth = m.width - 11
		}
		m.panes[0].SetSize(firstWidth, contentHeight)
		m.panes[1].SetSize(m.width-firstWidth-1, contentHeight)

	case SplitHorizontal:
		// Stacked, leave 1 line for separator
		firstHeight := int(float64(contentHeight-1) * m.splitRatio)
		if firstHeight < 3 {
			firstHeight = 3
		}
		if firstHeight > contentHeight-4 {
			firstHeight = contentHeight - 4
		}
		m.panes[0].SetSize(m.width, firstHeight)
		m.panes[1].SetSize(m.width, contentHeight-firstHeight-1)
	}
}

// renderVerticalSplit renders two panes side by side
func (m *Model) renderVerticalSplit() string {
	left := m.panes[0].Render()
	right := m.panes[1].Render()

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	var result strings.Builder

	// Choose separator based on active pane
	separator := "│"
	if m.activePane == 0 {
		separator = "┃"
	}

	// Get pane widths from ratio
	leftWidth := int(float64(m.width-1) * m.splitRatio)
	if leftWidth < 10 {
		leftWidth = 10
	}
	if leftWidth > m.width-11 {
		leftWidth = m.width - 11
	}
	rightWidth := m.width - leftWidth - 1

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	for i := 0; i < maxLines; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		rightLine := ""
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}

		// Truncate or pad left line to fit width
		leftLine = truncateOrPad(leftLine, leftWidth)
		// Truncate right line
		rightLine = truncateString(rightLine, rightWidth)

		result.WriteString(leftLine)
		result.WriteString(separator)
		result.WriteString(rightLine)
		result.WriteString("\n")
	}

	return result.String()
}

// truncateOrPad ensures a string is exactly the given visible width (ANSI-aware)
func truncateOrPad(s string, width int) string {
	visWidth := visibleWidth(s)
	if visWidth > width {
		return truncateToWidth(s, width)
	}
	// Pad with spaces
	return s + strings.Repeat(" ", width-visWidth)
}

// truncateString truncates a string to max visible width (ANSI-aware)
func truncateString(s string, width int) string {
	if visibleWidth(s) > width {
		return truncateToWidth(s, width)
	}
	return s
}

// visibleWidth calculates the visible width of a string, ignoring ANSI escape codes
func visibleWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		width++
	}
	return width
}

// truncateToWidth truncates a string to a visible width, preserving ANSI codes
func truncateToWidth(s string, width int) string {
	var result strings.Builder
	visWidth := 0
	inEscape := false

	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			result.WriteRune(r)
			continue
		}
		if inEscape {
			result.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		if visWidth >= width {
			break
		}
		result.WriteRune(r)
		visWidth++
	}
	// Reset any open ANSI codes
	result.WriteString("\x1b[0m")
	return result.String()
}

// renderHorizontalSplit renders two panes stacked
func (m *Model) renderHorizontalSplit() string {
	top := m.panes[0].Render()
	bottom := m.panes[1].Render()

	// Choose separator based on active pane
	separator := strings.Repeat("─", m.width)
	if m.activePane == 1 {
		separator = strings.Repeat("━", m.width)
	}

	return top + "\n" + separator + "\n" + bottom + "\n"
}


// View implements tea.Model
func (m *Model) View() string {
	var builder strings.Builder

	// Show help screen
	if m.mode == ModeHelp {
		return m.renderHelp()
	}

	// Show file info
	if m.mode == ModeFileInfo {
		return m.renderFileInfo()
	}

	// Render pane(s)
	if len(m.panes) == 1 {
		builder.WriteString(m.panes[0].Render())
		builder.WriteString("\n")
	} else {
		switch m.splitDir {
		case SplitVertical:
			builder.WriteString(m.renderVerticalSplit())
		case SplitHorizontal:
			builder.WriteString(m.renderHorizontalSplit())
		}
	}

	pane := m.currentPane()

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
	case ModeVisual:
		start, end := pane.GetVisualSelectionRange()
		lineCount := 0
		if start >= 0 && end >= 0 {
			lineCount = end - start + 1
		}
		status = fmt.Sprintf(" -- VISUAL -- %d lines selected (L%d-L%d)  y:yank  v/esc:cancel",
			lineCount, start+1, end+1)
	default:
		// Show filtered count vs total if filter is active
		var lineInfo string
		if pane.FilteredSource().IsFiltered() {
			lineInfo = fmt.Sprintf("L%d/%d (of %d)",
				pane.Viewport().CurrentLine()+1,
				pane.FilteredSource().LineCount(),
				pane.Source().LineCount())
		} else {
			lineInfo = fmt.Sprintf("L%d/%d",
				pane.Viewport().CurrentLine()+1,
				pane.Source().LineCount())
		}

		percent := fmt.Sprintf("%.0f%%", pane.Viewport().PercentScrolled())

		searchInfo := ""
		if pane.SearchTerm() != "" {
			searchInfo = fmt.Sprintf(" [%d matches]", len(pane.SearchResults()))
		}

		// Show active filters
		filterInfo := ""
		if pane.FilteredSource().IsFiltered() {
			var parts []string

			// Level filters
			filters := pane.FilteredSource().GetActiveFilters()
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
			if pane.FilteredSource().HasTextFilter() {
				text := pane.FilteredSource().GetTextFilter()
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
		if pane.HasSlice() {
			current := pane.CurrentSlice()
			depth := pane.SliceDepth()
			if depth > 1 {
				sliceInfo = fmt.Sprintf(" [slice×%d:%d-%d]", depth, current.StartLine+1, current.EndLine)
			} else {
				sliceInfo = fmt.Sprintf(" [slice:%d-%d]", current.StartLine+1, current.EndLine)
			}
		} else if pane.IsCached() {
			sliceInfo = " [cached]"
		}

		// Follow indicator
		followInfo := ""
		if pane.IsFollowing() {
			followInfo = " [following]"
		}

		// Get timestamp for current line
		timeInfo := ""
		currentLine := pane.Viewport().CurrentLine()
		if ts := pane.Source().GetTimestamp(currentLine); ts != nil {
			timeInfo = fmt.Sprintf(" %s", ts.Format("15:04:05"))
		}

		// Add temporary message if present
		msgInfo := ""
		if m.message != "" {
			msgInfo = fmt.Sprintf(" [%s]", m.message)
		}

		status = fmt.Sprintf(" %s%s%s  %s%s  %s%s%s%s",
			pane.Filename(), sliceInfo, followInfo, lineInfo, timeInfo, percent, searchInfo, filterInfo, msgInfo)
	}

	builder.WriteString(statusStyle.Render(status))
	builder.WriteString("\n")

	// Help line
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "j/k:scroll  /:search  ?:filter  t/d/i/w/e:level  T/D/I/W/E:lvl+  0:clear  q:quit"
	builder.WriteString(helpStyle.Render(help))

	return builder.String()
}

// renderFileInfo renders file information (ctrl+g)
func (m *Model) renderFileInfo() string {
	pane := m.currentPane()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("File Information"))
	b.WriteString("\n\n")

	// File path
	b.WriteString(labelStyle.Render("  File:      "))
	b.WriteString(valueStyle.Render(pane.sourcePath))
	b.WriteString("\n")

	// Line count
	b.WriteString(labelStyle.Render("  Lines:     "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d", pane.Source().LineCount())))
	b.WriteString("\n")

	// Current position
	currentLine := pane.Viewport().CurrentLine() + 1
	totalLines := pane.Source().LineCount()
	percent := float64(currentLine) / float64(totalLines) * 100
	b.WriteString(labelStyle.Render("  Position:  "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("line %d of %d (%.0f%%)", currentLine, totalLines, percent)))
	b.WriteString("\n")

	// Filtered info
	if pane.FilteredSource().IsFiltered() {
		b.WriteString(labelStyle.Render("  Filtered:  "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%d lines visible", pane.FilteredSource().LineCount())))
		b.WriteString("\n")
	}

	// Slice info
	if pane.HasSlice() {
		slice := pane.CurrentSlice()
		b.WriteString(labelStyle.Render("  Slice:     "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("lines %d-%d", slice.StartLine+1, slice.EndLine)))
		b.WriteString("\n")
	}

	// Cache info
	if pane.IsCached() {
		b.WriteString(labelStyle.Render("  Cached:    "))
		b.WriteString(valueStyle.Render(pane.cachePath))
		b.WriteString("\n")
	}

	// Marks
	if len(pane.marks) > 0 {
		var marks []string
		for char, line := range pane.marks {
			marks = append(marks, fmt.Sprintf("'%c:%d", char, line+1))
		}
		b.WriteString(labelStyle.Render("  Marks:     "))
		b.WriteString(valueStyle.Render(strings.Join(marks, " ")))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(valueStyle.Render("Press any key to close"))

	return b.String()
}

// renderHelp renders the help screen
func (m *Model) renderHelp() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("mless - Help"))
	b.WriteString("\n\n")

	sections := []struct {
		title string
		items []string
	}{
		{"Navigation", []string{
			"j/k, up/down    Scroll line by line",
			"f/b, pgdn/pgup  Page down/up",
			"ctrl+d/u        Half page down/up",
			"g/G             Go to top/bottom",
			":N              Go to line N",
			"ctrl+t          Go to time (HH:MM:SS)",
		}},
		{"Search & Filter", []string{
			"/pattern        Search for pattern",
			"n/N             Next/prev search result",
			"?pattern        Filter lines (fzf-style)",
			"esc             Clear search/filter",
		}},
		{"Log Levels", []string{
			"t/d/i/w/e       Toggle trace/debug/info/warn/error",
			"alt+f           Toggle fatal",
			"T/D/I/W/E       Show level and above",
			"0               Clear all level filters",
		}},
		{"Marks", []string{
			"ma-mz           Set mark a-z at current line",
			"'a-'z           Jump to mark a-z",
			"]['             Next/prev mark",
			"M               Clear all marks",
		}},
		{"Slicing", []string{
			"S               Slice range (e.g., 'a-'b, 13:00-14:00, 100-$)",
			"ctrl+s          Slice from current to end",
			"R               Revert slice / resync cache",
		}},
		{"Yank (Copy)", []string{
			"yy / Y          Yank current line to clipboard",
			"5yy             Yank 5 lines",
			"y'a             Yank from current to mark 'a",
			"v               Enter visual mode for selection",
			"  j/k           Extend selection (in visual mode)",
			"  y             Yank selection (in visual mode)",
			"  v/esc         Cancel visual mode",
		}},
		{"Long Lines", []string{
			"< / >           Scroll horizontally",
			"^               Reset horizontal scroll",
			"Z               Toggle line wrap",
		}},
		{"Split Views", []string{
			"ctrl+w v        Vertical split (side-by-side)",
			"ctrl+w s        Horizontal split (stacked)",
			"ctrl+w w / tab  Switch pane",
			"ctrl+w q        Close current pane",
			"ctrl+o          Toggle split orientation",
			"H / L           Resize split",
			"=               Reset split to 50/50",
		}},
		{"Other", []string{
			"F               Toggle follow mode",
			"l               Show line numbers",
			"ctrl+g          Show file info",
			"h               Show this help",
			"q               Quit",
		}},
	}

	for _, section := range sections {
		b.WriteString(titleStyle.Render(section.title))
		b.WriteString("\n")
		for _, item := range section.items {
			// Split on first multiple spaces to separate key from description
			parts := strings.SplitN(item, "  ", 2)
			if len(parts) == 2 {
				b.WriteString("  ")
				b.WriteString(keyStyle.Render(fmt.Sprintf("%-16s", strings.TrimSpace(parts[0]))))
				b.WriteString(helpStyle.Render(strings.TrimSpace(parts[1])))
			} else {
				b.WriteString("  ")
				b.WriteString(helpStyle.Render(item))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("Press any key to close help"))

	return b.String()
}

// Close cleans up resources
func (m *Model) Close() error {
	var err error
	for _, pane := range m.panes {
		if paneErr := pane.Close(); paneErr != nil && err == nil {
			err = paneErr
		}
	}
	return err
}

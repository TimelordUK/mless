package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/config"
	"github.com/user/mless/internal/source"
)

// tickMsg is sent periodically in follow mode
type tickMsg time.Time

// ModelOptions contains options for creating a new model
type ModelOptions struct {
	Filepath   string
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
)

// Model is the main application model
type Model struct {
	panes      []*Pane
	activePane int

	searchInput textinput.Model
	config      *config.Config

	mode   Mode
	width  int
	height int

	// Status
	err error
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

	pane, err := NewPane(opts.Filepath, cfg, opts.CacheFile)
	if err != nil {
		return nil, err
	}

	// Apply initial slice if specified
	if opts.SliceRange != "" {
		if err := pane.ParseAndSlice(opts.SliceRange); err != nil {
			pane.Close()
			return nil, fmt.Errorf("invalid slice range: %w", err)
		}
	}

	// Apply initial time navigation if specified
	if opts.GotoTime != "" {
		pane.GotoTime(opts.GotoTime)
	}

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	return &Model{
		panes:       []*Pane{pane},
		activePane:  0,
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
		// Reserve 2 lines for status bar
		m.currentPane().SetSize(msg.Width, msg.Height-2)
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

	// Normal mode
	pane := m.currentPane()
	switch msg.String() {
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

	case "j", "down":
		pane.Viewport().ScrollDown(1)
	case "k", "up":
		pane.Viewport().ScrollUp(1)

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

	case "0": // Clear all filters
		pane.FilteredSource().ClearFilter()
		pane.Viewport().GotoTop()

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
		m.currentPane().GotoTime(m.searchInput.Value())
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


// View implements tea.Model
func (m *Model) View() string {
	var builder strings.Builder

	// Show help screen
	if m.mode == ModeHelp {
		return m.renderHelp()
	}

	pane := m.currentPane()

	// Main content (pane.Render() handles marks)
	builder.WriteString(pane.Render())
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
			sliceInfo = fmt.Sprintf(" [slice:%d-%d]", current.StartLine+1, current.EndLine)
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

		status = fmt.Sprintf(" %s%s%s  %s%s  %s%s%s",
			pane.Filename(), sliceInfo, followInfo, lineInfo, timeInfo, percent, searchInfo, filterInfo)
	}

	builder.WriteString(statusStyle.Render(status))
	builder.WriteString("\n")

	// Help line
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "j/k:scroll  /:search  ?:filter  t/d/i/w/e:level  T/D/I/W/E:lvl+  0:clear  q:quit"
	builder.WriteString(helpStyle.Render(help))

	return builder.String()
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
		{"Other", []string{
			"F               Toggle follow mode",
			"l               Show line numbers",
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

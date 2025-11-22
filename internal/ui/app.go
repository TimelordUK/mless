package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/config"
	"github.com/user/mless/internal/render"
	"github.com/user/mless/internal/source"
	"github.com/user/mless/internal/view"
	"github.com/user/mless/pkg/logformat"
)

// Mode represents the current UI mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeSearch
	ModeGoto
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

	// Status
	filename string
	err      error
}

// NewModel creates a new application model
func NewModel(filepath string) (*Model, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	src, err := source.NewFileSource(filepath)
	if err != nil {
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
		filename:       filepath,
	}, nil
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
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle mode-specific input
	if m.mode == ModeSearch {
		return m.handleSearchKey(msg)
	}
	if m.mode == ModeGoto {
		return m.handleGotoKey(msg)
	}

	// Normal mode
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

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
	case "d": // Debug
		m.filteredSource.ToggleLevel(source.LevelDebug)
	case "i": // Info
		m.filteredSource.ToggleLevel(source.LevelInfo)
	case "w": // Warn
		m.filteredSource.ToggleLevel(source.LevelWarn)
	case "e": // Error
		m.filteredSource.ToggleLevel(source.LevelError)
	case "F": // Fatal (capital to avoid conflict with page-down f)
		m.filteredSource.ToggleLevel(source.LevelFatal)

	// Shift+letter: show this level and above
	case "T": // Trace and above (all)
		m.filteredSource.SetLevelAndAbove(source.LevelTrace)
	case "D": // Debug and above
		m.filteredSource.SetLevelAndAbove(source.LevelDebug)
	case "I": // Info and above
		m.filteredSource.SetLevelAndAbove(source.LevelInfo)
	case "W": // Warn and above
		m.filteredSource.SetLevelAndAbove(source.LevelWarn)
	case "E": // Error and above
		m.filteredSource.SetLevelAndAbove(source.LevelError)
	// Note: F is already used for fatal toggle, use ctrl+f for fatal-only if needed

	case "0": // Clear all filters
		m.filteredSource.ClearFilter()
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
			filters := m.filteredSource.GetActiveFilters()
			var levels []string
			levelNames := map[source.LogLevel]string{
				source.LevelTrace: "TRC",
				source.LevelDebug: "DBG",
				source.LevelInfo:  "INF",
				source.LevelWarn:  "WRN",
				source.LevelError: "ERR",
				source.LevelFatal: "FTL",
			}
			for level, active := range filters {
				if active {
					levels = append(levels, levelNames[level])
				}
			}
			if len(levels) > 0 {
				filterInfo = fmt.Sprintf(" [%s]", strings.Join(levels, ","))
			}
		}

		status = fmt.Sprintf(" %s  %s  %s%s%s",
			m.filename, lineInfo, percent, searchInfo, filterInfo)
	}

	builder.WriteString(statusStyle.Render(status))
	builder.WriteString("\n")

	// Help line
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "j/k:scroll  f/b:page  /:search  t/d/i/w/e:filter  T/D/I/W/E:lvl+  0:clear  q:quit"
	builder.WriteString(helpStyle.Render(help))

	return builder.String()
}

// Close cleans up resources
func (m *Model) Close() error {
	if m.source != nil {
		return m.source.Close()
	}
	return nil
}

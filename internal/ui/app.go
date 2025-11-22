package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/mless/internal/source"
	"github.com/user/mless/internal/view"
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
	viewport   *view.Viewport
	source     *source.FileSource
	searchInput textinput.Model

	mode       Mode
	width      int
	height     int

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
	src, err := source.NewFileSource(filepath)
	if err != nil {
		return nil, err
	}

	viewport := view.NewViewport(80, 24)
	viewport.SetProvider(src)

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	return &Model{
		viewport:    viewport,
		source:      src,
		searchInput: ti,
		mode:        ModeNormal,
		filename:    filepath,
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

	case "d", "ctrl+d":
		m.viewport.PageDown()
	case "u", "ctrl+u":
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
	}
}

func (m *Model) nextSearchResult() {
	if len(m.searchResults) == 0 {
		return
	}
	m.searchIndex = (m.searchIndex + 1) % len(m.searchResults)
	m.viewport.GotoLine(m.searchResults[m.searchIndex])
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
		lineInfo := fmt.Sprintf("L%d/%d",
			m.viewport.CurrentLine()+1,
			m.source.LineCount())

		percent := fmt.Sprintf("%.0f%%", m.viewport.PercentScrolled())

		searchInfo := ""
		if m.searchTerm != "" {
			searchInfo = fmt.Sprintf(" [%d matches]", len(m.searchResults))
		}

		status = fmt.Sprintf(" %s  %s  %s%s",
			m.filename, lineInfo, percent, searchInfo)
	}

	builder.WriteString(statusStyle.Render(status))
	builder.WriteString("\n")

	// Help line
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := "j/k:scroll  f/b:page  g/G:top/bottom  /:search  n/N:next/prev  q:quit"
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

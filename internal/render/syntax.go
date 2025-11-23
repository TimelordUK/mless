package render

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/lipgloss"
	"github.com/TimelordUK/mless/internal/source"
)

// SyntaxRenderer applies syntax highlighting based on file type
type SyntaxRenderer struct {
	filename    string
	lexerName   string
	syntaxTheme string
}

// NewSyntaxRenderer creates a syntax highlighting renderer for the given filename
func NewSyntaxRenderer(filename string) *SyntaxRenderer {
	// Get lexer by filename extension
	lexer := lexers.Match(filename)
	lexerName := "plaintext"
	if lexer != nil {
		lexerName = lexer.Config().Name
	}

	return &SyntaxRenderer{
		filename:    filename,
		lexerName:   lexerName,
		syntaxTheme: "monokai",
	}
}

// Render applies syntax highlighting to a line
func (r *SyntaxRenderer) Render(line *source.Line) string {
	content := string(line.Content)
	if content == "" {
		return ""
	}

	var buf bytes.Buffer
	err := quick.Highlight(&buf, content, r.lexerName, "terminal16m", r.syntaxTheme)
	if err != nil {
		return content
	}

	// Remove any newlines that quick.Highlight adds
	highlighted := buf.String()
	highlighted = strings.ReplaceAll(highlighted, "\n", "")
	highlighted = strings.ReplaceAll(highlighted, "\r", "")

	// Use lipgloss to ensure proper rendering
	style := lipgloss.NewStyle()
	return style.Render(highlighted)
}

// IsSyntaxHighlightable returns true if the file type supports syntax highlighting
func IsSyntaxHighlightable(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	// Common source code extensions
	syntaxExts := map[string]bool{
		".go": true, ".rs": true, ".py": true, ".js": true, ".ts": true,
		".jsx": true, ".tsx": true, ".c": true, ".cpp": true, ".h": true,
		".hpp": true, ".java": true, ".rb": true, ".php": true, ".swift": true,
		".kt": true, ".scala": true, ".cs": true, ".fs": true, ".lua": true,
		".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".yaml": true, ".yml": true, ".json": true, ".toml": true, ".xml": true,
		".html": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
		".sql": true, ".md": true, ".markdown": true, ".vim": true,
		".dockerfile": true, ".makefile": true, ".cmake": true,
		".zig": true, ".nim": true, ".v": true, ".d": true, ".r": true,
		".ex": true, ".exs": true, ".erl": true, ".hrl": true, ".clj": true,
		".hs": true, ".ml": true, ".mli": true, ".pl": true, ".pm": true,
	}

	if syntaxExts[ext] {
		return true
	}

	// Check for special filenames
	base := strings.ToLower(filepath.Base(filename))
	specialFiles := map[string]bool{
		"makefile": true, "dockerfile": true, "cmakelists.txt": true,
		"gemfile": true, "rakefile": true, "vagrantfile": true,
	}

	return specialFiles[base]
}

package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/mless/internal/ui"
)

func main() {
	cacheFlag := flag.Bool("c", false, "Cache file locally for better performance")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mless [-c] <file>\n")
		fmt.Fprintf(os.Stderr, "  -c\tCache file locally (useful for network files)\n")
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	filepath := flag.Arg(0)

	opts := ui.ModelOptions{
		Filepath:  filepath,
		CacheFile: *cacheFlag,
	}

	model, err := ui.NewModelWithOptions(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer model.Close()

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

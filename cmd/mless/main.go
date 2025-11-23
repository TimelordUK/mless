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
	sliceFlag := flag.String("S", "", "Slice range (e.g., 1000-5000, 100-$, .-500)")
	timeFlag := flag.String("t", "", "Go to time (e.g., 14:00, 14:30:00)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mless [-c] [-S range] [-t time] <file>\n")
		fmt.Fprintf(os.Stderr, "  -c\tCache file locally (useful for network files)\n")
		fmt.Fprintf(os.Stderr, "  -S\tSlice range (e.g., 1000-5000, 100-$)\n")
		fmt.Fprintf(os.Stderr, "  -t\tGo to time (e.g., 14:00, 14:30:00)\n")
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	filepath := flag.Arg(0)

	opts := ui.ModelOptions{
		Filepath:   filepath,
		CacheFile:  *cacheFlag,
		SliceRange: *sliceFlag,
		GotoTime:   *timeFlag,
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

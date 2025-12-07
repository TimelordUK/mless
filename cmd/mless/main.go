package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/TimelordUK/mless/internal/ui"
)

// Version info - set via ldflags at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// isPiped checks if stdin has piped input
func isPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// readStdinToTemp reads stdin to a temporary file and returns its path
func readStdinToTemp() (string, error) {
	tmpFile, err := os.CreateTemp("", "mless-stdin-*.log")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, os.Stdin)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}

	return tmpFile.Name(), nil
}

func main() {
	versionFlag := flag.Bool("v", false, "Print version and exit")
	flag.BoolVar(versionFlag, "version", false, "Print version and exit")
	cacheFlag := flag.Bool("c", false, "Cache file locally for better performance")
	sliceFlag := flag.String("S", "", "Slice range (e.g., 1000-5000, 100-$, .-500)")
	timeFlag := flag.String("t", "", "Go to time (e.g., 14:00, 14:30:00)")
	consolidateFlag := flag.Bool("C", false, "Consolidate multiple files into single view")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mless [-c] [-C] [-S range] [-t time] [file...]\n")
		fmt.Fprintf(os.Stderr, "       command | mless [-S range] [-t time]\n")
		fmt.Fprintf(os.Stderr, "  -v\tPrint version and exit\n")
		fmt.Fprintf(os.Stderr, "  -c\tCache file locally (useful for network files)\n")
		fmt.Fprintf(os.Stderr, "  -C\tConsolidate multiple files into single view\n")
		fmt.Fprintf(os.Stderr, "  -S\tSlice range (e.g., 1000-5000, 100-$)\n")
		fmt.Fprintf(os.Stderr, "  -t\tGo to time (e.g., 14:00, 14:30:00)\n")
		fmt.Fprintf(os.Stderr, "\nMultiple files: split view (max 2) or consolidated (-C)\n")
	}
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mless %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	var filePaths []string
	var stdinTempFile string

	if flag.NArg() >= 1 {
		// Get absolute paths for all files
		// Consolidated mode: no limit; split view: max 2 files
		maxFiles := 2
		if *consolidateFlag {
			maxFiles = flag.NArg()
		}
		for i := 0; i < flag.NArg() && i < maxFiles; i++ {
			filePath := flag.Arg(i)
			absPath, err := filepath.Abs(filePath)
			if err == nil {
				filePath = absPath
			}
			filePaths = append(filePaths, filePath)
		}
	} else if isPiped() {
		// Read from stdin
		var err error
		stdinTempFile, err = readStdinToTemp()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		filePaths = []string{stdinTempFile}
		// Clean up temp file on exit
		defer os.Remove(stdinTempFile)
	} else {
		flag.Usage()
		os.Exit(1)
	}

	var consolidatePaths []string
	if *consolidateFlag && len(filePaths) > 0 {
		consolidatePaths = filePaths
	}

	opts := ui.ModelOptions{
		Filepaths:        filePaths,
		CacheFile:        *cacheFlag,
		SliceRange:       *sliceFlag,
		GotoTime:         *timeFlag,
		ConsolidatePaths: consolidatePaths,
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

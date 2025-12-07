package consolidate

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TimelordUK/mless/internal/source"
)

// SourceWatcher tracks a single file source for the consolidated writer
type SourceWatcher struct {
	source   *source.FileSource
	name     string // Display name (basename)
	position int    // Next line to write (starts at EOF for tail-only)
	enabled  bool   // Include in output
}

// Writer merges multiple log files into a single consolidated output file
type Writer struct {
	sources    []*SourceWatcher
	outputPath string
	output     *os.File
	pollMs     int  // Poll interval in milliseconds
	prefix     bool // Add "[source:line] " prefix to each line

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWriter creates a new consolidated writer
// Primes with last N lines from each source, then tails for new content
func NewWriter(paths []string) (*Writer, error) {
	return NewWriterWithPrime(paths, 100) // Default: prime with last 100 lines per source
}

// NewWriterWithPrime creates a consolidated writer with configurable priming
// primeLines=0 means tail-only (no initial content)
func NewWriterWithPrime(paths []string, primeLines int) (*Writer, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no source files provided")
	}

	// Generate output path
	hash := md5.Sum([]byte(time.Now().String()))
	outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("mless-consolidated-%x.log", hash[:8]))

	// Create output file
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	// Create source watchers
	var sources []*SourceWatcher
	for _, path := range paths {
		src, err := source.NewFileSource(path)
		if err != nil {
			// Clean up already opened sources
			for _, sw := range sources {
				sw.source.Close()
			}
			output.Close()
			os.Remove(outputPath)
			return nil, fmt.Errorf("failed to open source %s: %w", path, err)
		}

		sources = append(sources, &SourceWatcher{
			source:   src,
			name:     filepath.Base(path),
			position: 0, // Will be set after priming
			enabled:  true,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Writer{
		sources:    sources,
		outputPath: outputPath,
		output:     output,
		pollMs:     250, // Default poll interval
		prefix:     true,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Prime with last N lines from each source
	if primeLines > 0 {
		w.primeOutput(primeLines)
	} else {
		// Tail-only: start at EOF
		for _, sw := range w.sources {
			sw.position = sw.source.LineCount()
		}
	}

	return w, nil
}

// primeOutput writes the last N lines from each source to the output
func (w *Writer) primeOutput(n int) {
	for _, sw := range w.sources {
		lineCount := sw.source.LineCount()
		start := lineCount - n
		if start < 0 {
			start = 0
		}

		// Write lines from start to end
		for i := start; i < lineCount; i++ {
			line, err := sw.source.GetLine(i)
			if err != nil || line == nil {
				continue
			}

			if w.prefix {
				fmt.Fprintf(w.output, "[%s:%d] ", sw.name, i+1)
			}
			w.output.Write(line.Content)
			w.output.WriteString("\n")
		}

		// Set position to EOF for tailing
		sw.position = lineCount
	}

	// Sync to disk
	w.output.Sync()
}

// Run starts the polling loop - should be called in a goroutine
func (w *Writer) Run() {
	w.wg.Add(1)
	defer w.wg.Done()

	ticker := time.NewTicker(time.Duration(w.pollMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll checks all sources for new lines and writes them to output
func (w *Writer) poll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, sw := range w.sources {
		if !sw.enabled {
			continue
		}

		// Check for new lines
		newLines, err := sw.source.Refresh()
		if err != nil {
			continue
		}

		if newLines > 0 {
			w.writeNewLines(sw)
		}
	}
}

// writeNewLines writes new lines from a source to the output
func (w *Writer) writeNewLines(sw *SourceWatcher) {
	lineCount := sw.source.LineCount()
	wrote := false

	for i := sw.position; i < lineCount; i++ {
		line, err := sw.source.GetLine(i)
		if err != nil || line == nil {
			continue
		}

		// Write prefix if enabled
		if w.prefix {
			fmt.Fprintf(w.output, "[%s:%d] ", sw.name, i+1) // 1-based line numbers
		}

		// Write line content
		w.output.Write(line.Content)
		w.output.WriteString("\n")
		wrote = true
	}

	// Sync to disk so mmap reader can see the changes
	if wrote {
		w.output.Sync()
	}

	sw.position = lineCount
}

// OutputPath returns the path to the consolidated output file
func (w *Writer) OutputPath() string {
	return w.outputPath
}

// SourceCount returns the number of source files
func (w *Writer) SourceCount() int {
	return len(w.sources)
}

// SetEnabled enables or disables a source by name
func (w *Writer) SetEnabled(name string, enabled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, sw := range w.sources {
		if sw.name == name {
			sw.enabled = enabled
			return
		}
	}
}

// SetPollInterval sets the poll interval in milliseconds
func (w *Writer) SetPollInterval(ms int) {
	w.pollMs = ms
}

// Close stops the writer and cleans up resources
func (w *Writer) Close() error {
	// Signal stop
	w.cancel()

	// Wait for goroutine to finish
	w.wg.Wait()

	// Close sources
	for _, sw := range w.sources {
		sw.source.Close()
	}

	// Close output file
	w.output.Close()

	// Remove temp file
	os.Remove(w.outputPath)

	return nil
}

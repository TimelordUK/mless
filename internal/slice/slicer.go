package slice

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/user/mless/internal/source"
)

// Info contains metadata about a slice
type Info struct {
	SourcePath string     // Original file path
	CachePath  string     // Local temp file path
	StartLine  int        // Start line (0-based, inclusive)
	EndLine    int        // End line (0-based, exclusive)
	StartTime  *time.Time // If time-based slice
	EndTime    *time.Time
	Parent     *Info      // For nested slices
}

// Slicer handles extracting portions of files to cache
type Slicer struct {
	cacheDir string
}

// NewSlicer creates a new slicer
func NewSlicer() *Slicer {
	return &Slicer{
		cacheDir: os.TempDir(),
	}
}

// SliceToEnd extracts from startLine to end of file
func (s *Slicer) SliceToEnd(src *source.FileSource, startLine int) (*Info, string, error) {
	return s.SliceRange(src, startLine, src.LineCount())
}

// SliceRange extracts lines from startLine to endLine (exclusive)
func (s *Slicer) SliceRange(src *source.FileSource, startLine, endLine int) (*Info, string, error) {
	if startLine < 0 {
		startLine = 0
	}
	if endLine > src.LineCount() {
		endLine = src.LineCount()
	}
	if startLine >= endLine {
		return nil, "", fmt.Errorf("invalid range: %d-%d", startLine, endLine)
	}

	// Generate cache filename
	baseName := filepath.Base(src.Path())
	cachePath := filepath.Join(s.cacheDir, fmt.Sprintf("mless-slice-%d-%d-%s", startLine, endLine, baseName))

	// Create output file
	outFile, err := os.Create(cachePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create slice file: %w", err)
	}
	defer outFile.Close()

	// Write lines to slice file
	for i := startLine; i < endLine; i++ {
		line, err := src.GetLine(i)
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to read line %d: %w", i, err)
		}
		if line == nil {
			continue
		}

		_, err = outFile.Write(line.Content)
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to write line %d: %w", i, err)
		}
		_, err = outFile.WriteString("\n")
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to write newline: %w", err)
		}
	}

	info := &Info{
		SourcePath: src.Path(),
		CachePath:  cachePath,
		StartLine:  startLine,
		EndLine:    endLine,
	}

	return info, cachePath, nil
}

// SliceFiltered extracts only lines that pass the current filter
func (s *Slicer) SliceFiltered(src *source.FileSource, filtered *source.FilteredProvider) (*Info, string, error) {
	if !filtered.IsFiltered() {
		// No filter active, slice entire file
		return s.SliceRange(src, 0, src.LineCount())
	}

	// Generate cache filename
	baseName := filepath.Base(src.Path())
	cachePath := filepath.Join(s.cacheDir, fmt.Sprintf("mless-slice-filtered-%s", baseName))

	// Create output file
	outFile, err := os.Create(cachePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create slice file: %w", err)
	}
	defer outFile.Close()

	// Write filtered lines
	filteredCount := filtered.LineCount()
	for i := 0; i < filteredCount; i++ {
		line, err := filtered.GetLine(i)
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to read filtered line %d: %w", i, err)
		}
		if line == nil {
			continue
		}

		_, err = outFile.Write(line.Content)
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to write line: %w", err)
		}
		_, err = outFile.WriteString("\n")
		if err != nil {
			os.Remove(cachePath)
			return nil, "", fmt.Errorf("failed to write newline: %w", err)
		}
	}

	info := &Info{
		SourcePath: src.Path(),
		CachePath:  cachePath,
		StartLine:  0,
		EndLine:    filteredCount,
	}

	return info, cachePath, nil
}

// Cleanup removes a slice's cache file
func (s *Slicer) Cleanup(info *Info) error {
	if info == nil || info.CachePath == "" {
		return nil
	}
	return os.Remove(info.CachePath)
}

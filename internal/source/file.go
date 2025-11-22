package source

import (
	"github.com/user/mless/internal/index"
	mlessio "github.com/user/mless/internal/io"
)

// FileSource provides lines from a single file
type FileSource struct {
	file      *mlessio.MappedFile
	lineIndex *index.LineIndex
	path      string
}

// NewFileSource creates a new file source
func NewFileSource(path string) (*FileSource, error) {
	file, err := mlessio.OpenMapped(path)
	if err != nil {
		return nil, err
	}

	lineIndex, err := index.BuildLineIndex(file)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &FileSource{
		file:      file,
		lineIndex: lineIndex,
		path:      path,
	}, nil
}

// LineCount returns total number of lines
func (s *FileSource) LineCount() int {
	return s.lineIndex.LineCount()
}

// GetLine returns line at index
func (s *FileSource) GetLine(idx int) (*Line, error) {
	content, err := s.lineIndex.GetLine(idx)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, nil
	}

	return &Line{
		Content:       content,
		Level:         LevelUnknown,
		OriginalIndex: idx,
	}, nil
}

// GetLines returns a range of lines
func (s *FileSource) GetLines(start, count int) ([]*Line, error) {
	rawLines, err := s.lineIndex.GetLines(start, count)
	if err != nil {
		return nil, err
	}

	lines := make([]*Line, len(rawLines))
	for i, content := range rawLines {
		lines[i] = &Line{
			Content:       content,
			Level:         LevelUnknown,
			OriginalIndex: start + i,
		}
	}
	return lines, nil
}

// Close closes the file source
func (s *FileSource) Close() error {
	return s.file.Close()
}

// Path returns the file path
func (s *FileSource) Path() string {
	return s.path
}

// Refresh checks if file has grown and indexes new lines
func (s *FileSource) Refresh() (int, error) {
	oldSize := s.file.Size()
	oldLineCount := s.lineIndex.LineCount()

	changed, err := s.file.Refresh()
	if err != nil {
		return 0, err
	}

	if !changed {
		return 0, nil
	}

	// Index new lines
	if err := s.lineIndex.AppendNewLines(oldSize); err != nil {
		return 0, err
	}

	newLines := s.lineIndex.LineCount() - oldLineCount
	return newLines, nil
}

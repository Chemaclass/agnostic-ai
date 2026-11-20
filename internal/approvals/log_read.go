package approvals

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// LogPath is the decision log in the agnostic-ai home folder.
func LogPath(agnosticHome string) string {
	return filepath.Join(agnosticHome, "local", "approvals", "decisions.jsonl")
}

// maxLogLine bounds one line of the log; a longer one is no line the
// hook writes, and is skipped.
const maxLogLine = 64 << 10

// ReadLog returns the decisions in the log at path, the older file
// first, each in the order it was written. A missing file reads as
// empty, and a line that does not parse is skipped.
func ReadLog(path string) ([]Entry, error) {
	var out []Entry
	for _, file := range []string{path + ".1", path} {
		entries, err := readLogFile(file)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	return out, nil
}

func readLogFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open approvals log: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []Entry
	reader := bufio.NewReaderSize(f, maxLogLine)
	for {
		line, err := reader.ReadSlice('\n')
		for errors.Is(err, bufio.ErrBufferFull) {
			line = nil
			_, err = reader.ReadSlice('\n')
		}
		var e Entry
		if len(line) > 0 && json.Unmarshal(line, &e) == nil && e.Decision != "" {
			out = append(out, e)
		}
		switch {
		case errors.Is(err, io.EOF):
			return out, nil
		case err != nil:
			return nil, fmt.Errorf("read approvals log: %w", err)
		}
	}
}

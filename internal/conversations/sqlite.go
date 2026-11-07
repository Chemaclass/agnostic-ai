package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// errNoSQLite means the sqlite3 command is not installed, so Codex and
// Cursor stores cannot be read. Reading them through the command keeps
// a SQLite library out of the binary.
var errNoSQLite = errors.New("the sqlite3 command is not installed")

const sqliteTimeout = 10 * time.Second

// SQLiteQuery runs one read-only query and returns its rows as JSON
// objects, one per row.
type SQLiteQuery func(ctx context.Context, path, query string) ([]json.RawMessage, error)

// sqliteURI opens a database without ever writing to it. A database
// with a -wal file may have a writer: mode=ro reads its newest pages.
// Without one there is nothing in flight, and immutable=1 opens it even
// when a read-only connection could not create its shared-memory file.
func sqliteURI(path string) string {
	query := "immutable=1"
	if _, err := os.Stat(path + "-wal"); err == nil {
		query = "mode=ro"
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query}).String()
}

// RunSQLite is the SQLiteQuery that runs the system sqlite3 command.
func RunSQLite(ctx context.Context, path, query string) ([]json.RawMessage, error) {
	binary, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil, errNoSQLite
	}
	ctx, cancel := context.WithTimeout(ctx, sqliteTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "-readonly", "-json", "-cmd", ".timeout 2000", sqliteURI(path), query)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3 %s: %w: %s", path, err, bytes.TrimSpace(stderr.Bytes()))
	}
	if len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		return nil, fmt.Errorf("sqlite3 %s: read rows: %w", path, err)
	}
	return rows, nil
}

// queryRows runs query and decodes each row into T.
func queryRows[T any](ctx context.Context, run SQLiteQuery, path, query string) ([]T, error) {
	raw, err := run(ctx, path, query)
	if err != nil {
		return nil, err
	}
	rows := make([]T, 0, len(raw))
	for _, item := range raw {
		var row T
		if err := json.Unmarshal(item, &row); err != nil {
			return nil, fmt.Errorf("sqlite3 %s: decode row: %w", path, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

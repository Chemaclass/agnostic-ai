package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ClaudeJob is one Claude Code background session, started with
// claude --bg, as claude agents --json lists it.
type ClaudeJob struct {
	// ShortID is the id claude attach takes, such as "e3469759". It does
	// not take the full session id: it matches the start of a short id.
	ShortID string
	// SessionID is the session's transcript id, a conversation's ID.
	SessionID string
	// Name is the session's name, which claude attach also takes.
	Name string
}

// claudeJobsReason says why claude agents --json gave no list.
type claudeJobsReason string

const (
	claudeNotInstalled   claudeJobsReason = "no claude command on PATH or in its usual install folders"
	claudeListFailed     claudeJobsReason = "claude agents --json failed"
	claudeListUnreadable claudeJobsReason = "claude agents --json printed what this version cannot read"
)

// claudeJobsError is the one error the adapter returns. The command's
// own error stays in cause, for logs.
type claudeJobsError struct {
	reason claudeJobsReason
	cause  error
}

func (e *claudeJobsError) Error() string { return string(e.reason) }

func (e *claudeJobsError) Unwrap() error { return e.cause }

// commandRunner runs a command and returns what it printed on stdout.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// claudeAgentsRow is the part of a claude agents --json row the server reads.
type claudeAgentsRow struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// claudeBackgroundKind marks a row started with claude --bg; other
// rows, such as interactive sessions, carry no id claude attach takes.
const claudeBackgroundKind = "background"

// claudeHomeFolders and claudeSystemFolders are where claude is looked
// for after PATH, in this order: an app started from the desktop gets a
// short PATH that leaves them out. ~/.local/bin holds the claude that
// Claude Code's native installer puts there, ~/.claude/local the one its
// older local install put there, and the system folders Homebrew's and
// npm's.
var claudeHomeFolders = []string{".local/bin", ".claude/local"}
var claudeSystemFolders = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// claudeFolders lists the folders findClaude looks in after PATH.
func claudeFolders() []string {
	var folders []string
	if home, err := os.UserHomeDir(); err == nil {
		for _, folder := range claudeHomeFolders {
			folders = append(folders, filepath.Join(home, folder))
		}
	}
	return append(folders, claudeSystemFolders...)
}

// findClaude locates the claude command: on PATH, then in folders.
func findClaude(folders []string) (string, error) {
	if found, err := exec.LookPath("claude"); err == nil {
		return found, nil
	}
	for _, folder := range folders {
		if found, err := exec.LookPath(filepath.Join(folder, "claude")); err == nil {
			return found, nil
		}
	}
	return "", exec.ErrNotFound
}

// listClaudeJobs is the only code that runs claude agents --json. locate
// finds the claude command to run.
func listClaudeJobs(ctx context.Context, locate func() (string, error), runCommand commandRunner) ([]ClaudeJob, error) {
	claude, err := locate()
	if err != nil {
		return nil, &claudeJobsError{reason: claudeNotInstalled, cause: err}
	}
	out, err := runCommand(ctx, claude, "agents", "--json")
	if errors.Is(err, exec.ErrNotFound) {
		return nil, &claudeJobsError{reason: claudeNotInstalled, cause: err}
	}
	if err != nil {
		return nil, &claudeJobsError{reason: claudeListFailed, cause: err}
	}
	var rows []claudeAgentsRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, &claudeJobsError{reason: claudeListUnreadable, cause: err}
	}
	var jobs []ClaudeJob
	for _, row := range rows {
		if row.Kind != claudeBackgroundKind || row.ID == "" {
			continue
		}
		jobs = append(jobs, ClaudeJob{ShortID: row.ID, SessionID: row.SessionID, Name: row.Name})
	}
	return jobs, nil
}

// runClaudeCommand runs a command with the launch timeout and no stdin;
// claude agents --json answers without a terminal.
func runClaudeCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

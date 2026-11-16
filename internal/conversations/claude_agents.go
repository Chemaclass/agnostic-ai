package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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
	claudeNotInstalled   claudeJobsReason = "no claude command on PATH"
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

// listClaudeJobs is the only code that runs claude agents --json.
func listClaudeJobs(ctx context.Context, runCommand commandRunner) ([]ClaudeJob, error) {
	out, err := runCommand(ctx, "claude", "agents", "--json")
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

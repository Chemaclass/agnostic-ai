package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func runApprovalsLog(t *testing.T, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"approvals", "log"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("approvals log: %v", err)
	}
	return stdout.String()
}

func TestApprovalsLog_PrintsDecisions(t *testing.T) {
	source := approvalsHome(t, "")
	if out := runApprovalsLog(t); !strings.Contains(out, "No approvals decisions") {
		t.Fatalf("empty log printed %q", out)
	}
	log := approvals.Log{Path: approvals.LogPath(source)}
	now := time.Now()
	for _, e := range []approvals.Entry{
		{Time: now.Add(-3 * time.Hour), Tool: "codex", Call: "Bash", Detail: "go vet ./...", Mode: "yolo", Decision: approvals.Approved},
		{Time: now.Add(-time.Minute), Tool: "claude", Session: "s1", Call: "Bash", Detail: "go test ./...", Mode: "yolo", Decision: approvals.Approved},
		{Time: now, Tool: "claude", Session: "s1", Call: "Bash", Detail: "git push", Mode: "yolo", Decision: approvals.Asked, Rule: approvals.RulePublish},
	} {
		if err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	out := runApprovalsLog(t)
	for _, want := range []string{"go vet ./...", "approved", "go test ./...", "asked (publish)", "git push"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output misses %q:\n%s", want, out)
		}
	}

	out = runApprovalsLog(t, "--since", "1h", "--json")
	var entries []approvals.Entry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("--json output: %v\n%s", err, out)
	}
	if len(entries) != 2 || entries[0].Detail != "go test ./..." || entries[1].Rule != approvals.RulePublish {
		t.Errorf("--since 1h entries = %+v", entries)
	}
}

func TestApprovalsLog_RejectsABadSince(t *testing.T) {
	approvalsHome(t, "")
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"approvals", "log", "--since", "yesterday"})
	if err := root.Execute(); err == nil {
		t.Fatal("approvals log --since yesterday succeeded, want an error")
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/overview"
)

func runViewOverview(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newViewOverviewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("view overview %v: %v", args, err)
	}
	return out.String()
}

func TestNewViewOverviewCmd_PrintsJSONThatParsesAsAnOverviewView(t *testing.T) {
	dir := t.TempDir()
	writeBareReadProject(t, dir)
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	out := runViewOverview(t, "--dir", dir, "--json")

	var view overview.View
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not an overview view: %v\n%s", err, out)
	}
	if view.SchemaVersion != overview.SchemaVersion || len(view.Sections) != 5 || view.Snapshot.Project.Name != filepath.Base(dir) {
		t.Errorf("view = %+v", view)
	}
	if got := itemWithFinding(view, "LINT038"); got != "permissions" {
		t.Errorf("LINT038 sits on %q, want the permissions settings item", got)
	}
	if after, _ := os.Getwd(); after != before {
		t.Errorf("working directory moved from %s to %s", before, after)
	}
}

func TestNewViewOverviewCmd_PrintsSectionCountsFindingsAndSyncState(t *testing.T) {
	dir := t.TempDir()
	writeBareReadProject(t, dir)

	out := runViewOverview(t, "--dir", dir)

	for _, want := range []string{
		"Harness overview of " + filepath.Base(dir),
		"Claude Code: out of sync",
		"Context — What is the agent told?",
		"Instructions 0 · Rules 0 · Agents 0 · Skills 0",
		"Authority — What may the agent do?",
		"Permissions 1",
		"! permissions: LINT038",
		"Lifecycle — What runs around the agent, and when?",
		"Settings — How is the harness configured?",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

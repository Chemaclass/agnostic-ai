package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// globalOverviewHome prepares a user home whose global source holds a
// skill, an agent, and an MCP server for Claude Code and Codex, and
// returns the home. Nothing is synced yet.
func globalOverviewHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	source := filepath.Join(home, "source")
	t.Setenv("AGNOSTIC_AI_HOME", source)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets:\n  - claude\n  - codex\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared instructions\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code\n---\nReview it.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "helper.md"), "---\nname: helper\ndescription: Helps with chores\n---\nHelp with chores.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "db.yaml"), "name: db\ncommand: db-mcp\n")
	return home
}

func driftFindings(findings []overview.Finding) []overview.Finding {
	var out []overview.Finding
	for _, f := range findings {
		if f.Source == overview.SourceDrift {
			out = append(out, f)
		}
	}
	return out
}

func TestGlobalOverview_ListsSpecsAndReportsDriftUntilSynced(t *testing.T) {
	home := globalOverviewHome(t)

	in, err := globalOverviewInputs()
	if err != nil {
		t.Fatal(err)
	}
	view := overview.Build(in)
	if !strings.HasPrefix(view.Snapshot.Project.Name, "global (") {
		t.Errorf("project name = %q, want the global source", view.Snapshot.Project.Name)
	}
	items := map[string][]string{}
	for _, s := range view.Sections {
		for _, it := range s.Items {
			items[s.ID] = append(items[s.ID], it.Kind+":"+it.Name)
		}
	}
	for _, want := range []struct{ section, item string }{
		{overview.SectionContext, "instructions:AGNOSTIC_AI.md"},
		{overview.SectionContext, "skill:review"},
		{overview.SectionContext, "agent:helper"},
		{overview.SectionTools, "mcp:db"},
		{overview.SectionSettings, "config:agnostic-ai.yaml"},
	} {
		if !slices.Contains(items[want.section], want.item) {
			t.Errorf("%s lists %v, want %s", want.section, items[want.section], want.item)
		}
	}

	claudeMD := filepath.ToSlash(filepath.Join(home, ".claude", "CLAUDE.md"))
	if !slices.ContainsFunc(in.Findings, func(f overview.Finding) bool {
		return f.Source == overview.SourceDrift && f.Target == "claude" && f.Path == claudeMD && f.Message == driftNotGenerated
	}) {
		t.Errorf("drift = %+v, want claude %s %s", driftFindings(in.Findings), claudeMD, driftNotGenerated)
	}
	if slices.ContainsFunc(in.Findings, func(f overview.Finding) bool { return f.Source == overview.SourceDoctor }) {
		t.Error("doctor findings ran for the global configuration")
	}
	if in.Snapshot.Evidence.Installed != "checked" || !strings.Contains(in.Snapshot.Evidence.Note, "doctor checks read a project") {
		t.Errorf("evidence = %+v", in.Snapshot.Evidence)
	}
	for _, target := range view.Targets {
		if target.Sync.State != overview.SyncDrifted {
			t.Errorf("%s sync = %+v before the first sync, want drifted", target.Target, target.Sync)
		}
	}

	root := NewRootCmd("test")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"sync", "--global"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --global: %v", err)
	}
	in, err = globalOverviewInputs()
	if err != nil {
		t.Fatal(err)
	}
	if drift := driftFindings(in.Findings); len(drift) > 0 {
		t.Errorf("drift after sync --global: %+v", drift)
	}
	for _, target := range overview.Build(in).Targets {
		if target.Sync.State != overview.SyncInSync {
			t.Errorf("%s sync = %+v after sync --global, want in-sync", target.Target, target.Sync)
		}
	}

	src, err := openAuthoritySource("", true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, b, err := src.load()
	if err != nil {
		t.Fatal(err)
	}
	v, err := authority.Build(cfg, b, authority.Options{Root: src.root, ConfigFiles: src.configFiles(), Global: true, Target: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Snapshot.Revision != in.Snapshot.Revision {
		t.Errorf("overview revision %s differs from the Authority view's %s for the same sources", in.Snapshot.Revision, v.Snapshot.Revision)
	}
}

func TestServeMux_GlobalServesTheOverview(t *testing.T) {
	globalOverviewHome(t)
	src, err := openAuthoritySource("", true)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServeMux(serveScope{ID: scopeGlobal, Root: src.root, load: src.load, opts: authority.HandlerOptions{Root: src.root, ConfigFiles: src.configFiles, Global: true}}))
	defer srv.Close()

	for _, path := range []string{overview.OverviewPath} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			SchemaVersion string `json:"schemaVersion"`
			Error         string `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || body.SchemaVersion != overview.SchemaVersion {
			t.Errorf("%s = %d %+v (%v)", path, resp.StatusCode, body, err)
		}
	}
}

func TestView_GlobalPrintsTheUserLevelConfiguration(t *testing.T) {
	globalOverviewHome(t)
	out, err := runCLI(t, "view", "--global", "--json")
	if err != nil {
		t.Fatalf("view --global: %v\n%s", err, out)
	}
	var view overview.View
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if !strings.HasPrefix(view.Snapshot.Project.Name, "global (") || !slices.Equal(view.Snapshot.Targets, []string{"claude", "codex"}) {
		t.Errorf("snapshot = %+v", view.Snapshot)
	}

	if _, err := runCLI(t, "view", "--global", "--dir", "."); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("--global --dir error = %v, want a flag conflict", err)
	}
}

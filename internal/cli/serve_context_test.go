package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestServeContext_MatchesTheCommandForTheChosenTarget(t *testing.T) {
	dir := budgetProject(t, "targets: [codex, claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), duplicatedPassage+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\n"+duplicatedPassage+"\n")
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: dir}))
	defer srv.Close()

	view := getContextView(t, srv.URL+contextViewPath+"?target=claude", http.StatusOK)
	want := contextJSON(t, "claude")
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if served, _ := filepath.EvalSymlinks(view.Root); !view.Supported || view.Target != "claude" || served != root || len(view.Targets) != 2 || view.Report == nil || view.StartupWordBudget != 2000 {
		t.Fatalf("view: %+v", view)
	}
	got, _ := json.Marshal(view.Report)
	expected, _ := json.Marshal(want)
	if string(got) != string(expected) {
		t.Errorf("served report differs from explain --context:\n%s\n%s", got, expected)
	}
	if len(view.Report.Opportunities) != 1 {
		t.Errorf("opportunities: %+v", view.Report.Opportunities)
	}

	if first := getContextView(t, srv.URL+contextViewPath, http.StatusOK); first.Target != "codex" {
		t.Errorf("default target: %q", first.Target)
	}
	getContextView(t, srv.URL+contextViewPath+"?target=cursor", http.StatusBadRequest)

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\n---\nKeep functions small.\n")
	if refreshed := getContextView(t, srv.URL+contextViewPath+"?target=claude", http.StatusOK); len(refreshed.Report.Opportunities) != 0 || refreshed.Report.Startup.Words >= view.Report.Startup.Words {
		t.Errorf("refresh did not read the changed source: %+v", refreshed.Report)
	}
}

func TestServeContext_ReportsTheConfiguredWordBudget(t *testing.T) {
	dir := budgetProject(t, "targets: [claude]\nlint:\n  instructions-words: 300\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Keep functions small.\n")
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: dir}))
	defer srv.Close()
	if view := getContextView(t, srv.URL+contextViewPath+"?target=claude", http.StatusOK); view.StartupWordBudget != 300 {
		t.Errorf("budget: %d", view.StartupWordBudget)
	}
}

func TestServeContext_ExplainsThatGlobalScopeHasNoProjectContext(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Global: true}))
	defer srv.Close()
	if view := getContextView(t, srv.URL+contextViewPath, http.StatusOK); view.Supported || view.Reason == "" || view.Report != nil || view.Targets == nil {
		t.Errorf("global view: %+v", view)
	}
}

func getContextView(t *testing.T, url string, status int) contextView {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != status {
		t.Fatalf("%s: status %d, want %d", url, resp.StatusCode, status)
	}
	var view contextView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestServeContext_CountsScopedRulesForTheGivenFile(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, copilot]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nname: go\nglobs: '**/*.go'\nalwaysApply: false\n---\n"+words(40)+"\n")
	srv := httptest.NewServer(newServeMux(nil, authority.HandlerOptions{Root: dir}))
	defer srv.Close()
	if view := getContextView(t, srv.URL+contextViewPath+"?target=claude&file=src/main.go", http.StatusOK); view.Report.File != "src/main.go" || view.Report.FileScope.Words != 40 || strings.Join(view.FileTargets, ",") != "claude" {
		t.Errorf("file scope: %+v", view.Report)
	}
	if view := getContextView(t, srv.URL+contextViewPath+"?target=claude", http.StatusOK); view.Report.FileScope.Words != 0 || view.Report.OnDemand.Words < 40 {
		t.Errorf("without a file the scoped rule is on demand: %+v", view.Report)
	}
	getContextView(t, srv.URL+contextViewPath+"?target=copilot&file=src/main.go", http.StatusBadRequest)
}

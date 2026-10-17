package ellipsis

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// kitSinkBundle exercises every kind the adapter declares: a root and a
// scoped review, an environment spec, an HTTP and a stdio MCP server, and
// the settings spec that names the reviewer and selects the environment.
// Rules, agents, and skills are left out on purpose; Ellipsis reads none
// of them from these files.
func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		review("root", "", "Check the changed code for correctness."),
		review("backend", "backend", "Check that handlers use a repository."),
		settings(map[string]any{
			"pull_requests": map[string]any{"draft": false},
			"reviewer":      reviewer(nil),
			"output":        map[string]any{"on_fail": "request_changes"},
			"budget":        map[string]any{"session": 3},
			"environment": selected(map[string]any{
				"compute": map[string]any{"cpu": 2, "memory": "8GB", "timeout": "30m"},
				"hooks": map[string]any{
					"build_base":   map[string]any{"inputs": []any{"api-repo/package.json", "api-repo/package-lock.json"}},
					"before_start": "cd /sandbox/api-repo\ntest -d node_modules\n",
				},
				"variables":   []any{map[string]any{"name": "NPM_TOKEN"}, map[string]any{"name": "NODE_ENV", "value": "test"}},
				"mcp_servers": []any{"slack"},
			}),
		}),
		environmentSpec(map[string]any{"install": "npm ci", "setup": "npm run build --if-present"}),
		httpServer("internal-tools"),
		mcpSpec("repo-tools", map[string]any{
			"command": "node",
			"args":    []any{"/sandbox/api-repo/scripts/mcp-server.js"},
			"env":     map[string]any{"API_TOKEN": "${REPO_TOOLS_TOKEN}"},
		}),
	})
}

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	paths := testutil.WalkRel(t, dir)
	for _, p := range []string{reviewFile, environmentFile} {
		if !slices.Contains(paths, filepath.FromSlash(p)) {
			t.Errorf("kit-sink bundle did not write %s (paths: %v)", p, paths)
		}
	}
	observed := map[spec.Kind]string{
		spec.KindReview:      "Check the changed code for correctness.",
		spec.KindSettings:    "model: claude-opus-5-5",
		spec.KindEnvironment: "cd /sandbox/api-repo\n      npm ci",
		spec.KindMCP:         "Bearer ${TOOLS_TOKEN}",
	}
	files := readReview(t, reviewFile) + readReview(t, environmentFile)
	for _, k := range caps.Supports {
		want, ok := observed[k]
		if !ok {
			t.Errorf("declared kind %q has no observable output check", k)
			continue
		}
		if !strings.Contains(files, want) {
			t.Errorf("declared kind %q in caps.Supports has no observable output: %q is absent", k, want)
		}
	}
	if len(caps.Supports) != len(observed) {
		t.Errorf("claim only the kinds this adapter writes, got %v", caps.Supports)
	}
}

func TestEmit_NoCapabilityWarningsForKitSinkBundle(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("expected no capability warnings, got %d", got)
	}
}

func TestEmit_UnsupportedKindsWarn(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt"}},
		{Kind: spec.KindSkill, Name: "uno", Path: "skills/uno/SKILL.md", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 2 {
		t.Errorf("expected 2 capability warnings (hook, skill), got %d", got)
	}
}

func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	checked := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !header.Has(string(data)) || !strings.HasPrefix(string(data), "# ") {
			t.Errorf("missing provenance header in %s", path)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("kit-sink bundle emitted nothing")
	}
}

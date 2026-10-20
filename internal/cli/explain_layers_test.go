package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestExplainLayers_ResolvesReplacedAndExtendedSources(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	pack := ".agnostic-ai/packs/team/rules/no-console-log.md"
	personal := ".agnostic-ai/local/rules/no-console-log.md"
	mustWrite(t, pack, "---\nname: no-console-log\n---\nPack rule.\n")
	mustWrite(t, personal, "---\nname: no-console-log\n---\n::parent\nPersonal rule.\n")
	var first string
	for _, selector := range []string{pack, "rules/no-console-log.md", personal, filepath.Join(dir, pack)} {
		var out bytes.Buffer
		root := NewRootCmd("test")
		root.SetOut(&out)
		root.SetArgs([]string{"explain", selector, "--layers", "--json"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Spec struct {
				Path string `json:"path"`
			} `json:"spec"`
			Sources []struct {
				Source struct {
					Path  string `json:"path"`
					Layer string `json:"layer"`
				} `json:"source"`
				Operation string `json:"operation"`
				Role      string `json:"role"`
			} `json:"sources"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Spec.Path != personal {
			t.Errorf("effective path = %q", got.Spec.Path)
		}
		var paths, operations, roles []string
		for _, source := range got.Sources {
			paths = append(paths, source.Source.Path)
			operations = append(operations, source.Operation)
			roles = append(roles, source.Role)
		}
		if !reflect.DeepEqual(paths, []string{pack, "rules/no-console-log.md", personal}) {
			t.Errorf("paths = %v", paths)
		}
		if !reflect.DeepEqual(operations, []string{"base", "replace", "extend"}) {
			t.Errorf("operations = %v", operations)
		}
		if !reflect.DeepEqual(roles, []string{"replaced", "contributor", "contributor"}) {
			t.Errorf("roles = %v", roles)
		}
		if first == "" {
			first = out.String()
		} else if first != out.String() {
			t.Error("selector changed report")
		}
	}
}

func TestExplainLayers_RejectsOtherViews(t *testing.T) {
	for _, args := range [][]string{
		{"--layers"}, {"--layers", "rules/a.md", "--global"},
		{"--layers", "--file", "a.go", "--target", "codex"},
		{"--layers", "--context", "--target", "codex"}, {"--layers", "--inputs"},
		{"--layers", "AAI-001"}, {"--layers", "LINT011"}, {"--layers", "rules/a.md", "--target", "codex"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := NewRootCmd("test")
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(append([]string{"explain"}, args...))
			if err := root.Execute(); err == nil {
				t.Fatal("expected flag conflict")
			}
		})
	}
}

func TestExplainLayers_BuiltinRemainsDiscoverableAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [handoff]\n")
	mustWrite(t, ".agnostic-ai/skills/handoff.md", "---\nname: handoff\n---\nProject handoff.\n")
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "builtin:handoff", "--layers", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var got explainLayersOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("sources=%+v", got.Sources)
	}
	source := got.Sources[0]
	if source.Source.Builtin == nil || source.Source.Builtin.Name != "handoff" || source.Source.Path != "" || source.Role != "replaced" {
		t.Errorf("builtin source=%+v", source)
	}
	if got.Sources[1].Role != "contributor" {
		t.Errorf("project source=%+v", got.Sources[1])
	}
}

func TestExplainLayers_UnknownSpecGivesSourceHint(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	root := NewRootCmd("test")
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"explain", "no-console-log.md", "--layers"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "Did you mean: rules/no-console-log.md") {
		t.Fatalf("error=%v", err)
	}
}

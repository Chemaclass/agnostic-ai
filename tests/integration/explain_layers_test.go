package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExplainLayers_BuiltBinaryShowsResolutionWithoutChangingFiles(t *testing.T) {
	binary := buildEllipsisBinary(t)
	dir := t.TempDir()
	home := t.TempDir()
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: []\ngitignore:\n  enabled: false\n")
	writeProjectFile(t, dir, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n    source: ./offline-team\n")
	paths := []string{
		".agnostic-ai/packs/team/rules/conventional-commits.md",
		".agnostic-ai/rules/conventional-commits.md",
		".agnostic-ai/local/rules/conventional-commits.md",
	}
	writeProjectFile(t, dir, paths[0], "---\nname: conventional-commits\n---\nPack convention.\n")
	writeProjectFile(t, dir, paths[1], "---\nname: conventional-commits\n---\nProject convention.\n")
	writeProjectFile(t, dir, paths[2], "---\nname: conventional-commits\n---\n::parent\nPersonal addition.\n")
	writeProjectFile(t, dir, ".agnostic-ai/rules/untouched.md", "---\nname: untouched\n---\nUnchanged rule.\n")
	command := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		var env []string
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "AGNOSTIC_AI_TARGET=") {
				env = append(env, value)
			}
		}
		cmd.Env = append(env, "HOME="+home, "AGNOSTIC_AI_HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	command("sync")
	emitted := readProjectFile(t, dir, ".claude/rules/conventional-commits.md")
	if !strings.Contains(emitted, "Project convention.") || !strings.Contains(emitted, "Personal addition.") || strings.Contains(emitted, "Pack convention.") {
		t.Fatalf("sync did not apply replacement then extension:\n%s", emitted)
	}
	before := explainLayersSnapshot(t, dir)
	wantSources := []explainLayersSource{
		{Source: explainLayersRef{Kind: "rule", Name: "conventional-commits", Path: paths[0], Layer: "pack:team"}, Operation: "base", Role: "replaced"},
		{Source: explainLayersRef{Kind: "rule", Name: "conventional-commits", Path: paths[1], Layer: "project"}, Operation: "replace", Role: "contributor"},
		{Source: explainLayersRef{Kind: "rule", Name: "conventional-commits", Path: paths[2], Layer: "project-user"}, Operation: "extend", Role: "contributor"},
	}
	var firstText, firstJSON string
	for _, path := range paths {
		text := command("explain", path, "--layers")
		wantText := "rule conventional-commits\n"
		for _, source := range wantSources {
			wantText += "  " + source.Source.Path + " [" + source.Source.Layer + "] " + source.Operation + " (" + source.Role + ")\n"
		}
		wantText += "effective source: " + paths[2] + "\n"
		if text != wantText {
			t.Errorf("layers text = %q, want %q", text, wantText)
		}
		jsonText := command("explain", path, "--layers", "--json")
		var got explainLayersResult
		decoder := json.NewDecoder(strings.NewReader(jsonText))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&got); err != nil {
			t.Fatalf("decode layers output: %v\n%s", err, jsonText)
		}
		want := explainLayersResult{Version: "1", Command: "explain", Spec: wantSources[2].Source, Sources: wantSources}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("layers JSON = %#v, want %#v", got, want)
		}
		if firstText == "" {
			firstText, firstJSON = text, jsonText
		} else if text != firstText || jsonText != firstJSON {
			t.Error("selecting another source changed the resolution output")
		}
	}
	if got := command("explain", paths[1], "--layers"); got != firstText {
		t.Error("repeated text output changed")
	}
	if got := command("explain", paths[1], "--layers", "--json"); got != firstJSON {
		t.Error("repeated JSON output changed")
	}
	var untouched explainLayersResult
	if err := json.Unmarshal([]byte(command("explain", ".agnostic-ai/rules/untouched.md", "--layers", "--json")), &untouched); err != nil {
		t.Fatalf("decode untouched layers: %v", err)
	}
	if len(untouched.Sources) != 1 || untouched.Sources[0].Operation != "base" || untouched.Sources[0].Role != "contributor" {
		t.Errorf("unoverridden sources = %#v", untouched.Sources)
	}
	if after := explainLayersSnapshot(t, dir); !reflect.DeepEqual(after, before) {
		t.Error("explain --layers changed project files")
	}
	command("sync")
	afterSync := explainLayersSnapshot(t, dir)
	for _, name := range []string{".sync-state", ".command-lock"} {
		path := filepath.Join(dir, ".agnostic-ai", name)
		delete(before, path)
		delete(afterSync, path)
	}
	if !reflect.DeepEqual(afterSync, before) {
		t.Error("sync changed source or emitted bytes after layer diagnostics")
	}
	command("sync", "--check")
}

type explainLayersRef struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Layer string `json:"layer"`
}

type explainLayersSource struct {
	Source    explainLayersRef `json:"source"`
	Operation string           `json:"operation"`
	Role      string           `json:"role"`
}

type explainLayersResult struct {
	Version string                `json:"version"`
	Command string                `json:"command"`
	Spec    explainLayersRef      `json:"spec"`
	Sources []explainLayersSource `json:"sources"`
}

func explainLayersSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

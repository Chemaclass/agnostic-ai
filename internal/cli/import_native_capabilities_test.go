package cli

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportNativeAgentCapabilities_ExactMappingsRoundTrip(t *testing.T) {
	cases := []struct {
		target, dir, field string
		native, can        []string
	}{
		{"gemini", ".gemini/agents", "tools", []string{"read_file", "replace", "run_shell_command", "web_fetch", "google_web_search"}, []string{"read", "edit", "shell", "web"}},
		{"kiro", ".kiro/agents", "tools", []string{"read", "shell", "web", "@github", "@github/get_issue"}, []string{"read", "shell", "web", "mcp:github", "mcp:github/get_issue"}},
		{"factory", ".factory/droids", "tools", []string{"Read", "Edit", "Create", "Execute", "FetchUrl", "WebSearch"}, []string{"read", "edit", "write", "shell", "web"}},
		{"windsurf", ".devin/agents", "allowed-tools", []string{"read", "edit", "write", "exec"}, []string{"read", "edit", "write", "shell"}},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			root := t.TempDir()
			testutil.Chdir(t, root)
			silence(t)
			native := make([]any, len(tc.native))
			for i, name := range tc.native {
				native[i] = name
			}
			body, err := yaml.Marshal(map[string]any{"name": "reader", "description": "Read source.", tc.field: native})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, tc.dir, "reader.md"), "---\n"+string(body)+"---\n\nRead source.\n")
			if err := importNativeAgentFixture(root, tc.target); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "agents", "reader.md"))
			if err != nil {
				t.Fatal(err)
			}
			entry, err := spec.ParseMarkdownBytes(spec.KindAgent, data)
			if err != nil {
				t.Fatal(err)
			}
			entry.Name = "reader"
			entry.Path = "agents/reader.md"
			if got := toStringSlice(entry.Meta["can"]); !reflect.DeepEqual(got, tc.can) {
				t.Errorf("can=%v,want%v; imported:\n%s", got, tc.can, data)
			}
			adapter, err := adapters.Resolve(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			files, err := captureEmit(adapter, spec.Bundle{Agents: []spec.Entry{entry}}, &config.Config{Targets: []string{tc.target}, OnUnsupported: "silent"})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range files {
				if filepath.Base(file.Path) != "reader.md" {
					continue
				}
				meta, _ := splitMdcFrontmatter([]byte(file.Content))
				if got := toStringSlice(meta[tc.field]); !reflect.DeepEqual(got, tc.native) {
					t.Errorf("emitted %s=%v,want%v", tc.field, got, tc.native)
				}
				found = true
			}
			if !found {
				t.Error("no native agent emitted")
			}
		})
	}
}

func TestImportNativeAgentCapabilities_AmbiguousAndUnknownNamesStayNative(t *testing.T) {
	cases := []struct {
		target, dir, field string
		names              []string
	}{
		{"kiro", ".kiro/agents", "tools", []string{"write"}},
		{"kiro", ".kiro/agents", "tools", []string{"read", "read"}},
		{"factory", ".factory/droids", "tools", []string{}},
		{"gemini", ".gemini/agents", "tools", []string{"web_fetch"}},
		{"factory", ".factory/droids", "tools", []string{"read-only"}},
		{"windsurf", ".devin/agents", "allowed-tools", []string{"read", "unlisted"}},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			root := t.TempDir()
			testutil.Chdir(t, root)
			silence(t)
			body, err := yaml.Marshal(map[string]any{"description": "Read source.", tc.field: tc.names})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, tc.dir, "reader.md"), "---\n"+string(body)+"---\n\nRead source.\n")
			if err := importNativeAgentFixture(root, tc.target); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "agents", "reader.md"))
			if err != nil {
				t.Fatal(err)
			}
			entry, err := spec.ParseMarkdownBytes(spec.KindAgent, data)
			if err != nil {
				t.Fatal(err)
			}
			if _, set := entry.Meta["can"]; set {
				t.Errorf("ambiguous names became can: %v", entry.Meta["can"])
			}
			custom, _ := entry.Meta["x-"+tc.target].(map[string]any)
			if got := toStringSlice(custom[tc.field]); !reflect.DeepEqual(got, tc.names) {
				t.Errorf("native override=%v,want%v; imported:\n%s", got, tc.names, data)
			}
		})
	}
}

func importNativeAgentFixture(root, target string) error {
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		return err
	}
	switch target {
	case "gemini":
		_, err := importGeminiAgents(root, filepath.Join(root, "agents"))
		return err
	case "kiro":
		_, err := importKiroAgents(root, filepath.Join(root, "agents"))
		return err
	case "factory":
		_, err := importFactoryDroids(filepath.Join(root, ".factory/droids"), filepath.Join(root, "agents"))
		return err
	default:
		return importFromWindsurf(root, rootSources(), &config.Config{})
	}
}

func TestImportNeutralSkillTools_ScalarKeepsUnknownAliases(t *testing.T) {
	input := "---\nname: reader\nallowed-tools: 'Read, Grep, Bash(git diff *)'\n---\n\nRead source.\n"
	got := importNeutralToolNames(input, "allowed-tools", false)
	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(got))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Meta["allowed-tools"] != "read, Grep, shell(git diff *)" {
		t.Errorf("allowed-tools=%v,want exact neutral names", entry.Meta["allowed-tools"])
	}
	if entry.Body != "Read source.\n" {
		t.Errorf("skill body=%q", entry.Body)
	}
}

package kiro

import (
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

func TestEmit_CommandWritesNativePrompt(t *testing.T) {
	cases := []struct {
		name        string
		targetDir   string
		commandsDir string
		wantDir     string
		dryRun      bool
		existing    string
	}{
		{name: "default", wantDir: ".kiro/prompts"},
		{name: "override", commandsDir: "vendor/kiro prompts", wantDir: "vendor/kiro prompts"},
		{name: "target-dir", targetDir: "vendor/kiro", wantDir: "vendor/kiro/prompts"},
		{name: "per-kind-precedence", targetDir: "vendor/kiro", commandsDir: "custom/prompts", wantDir: "custom/prompts"},
		{name: "dry-run-new", commandsDir: "vendor/kiro prompts", wantDir: "vendor/kiro prompts", dryRun: true},
		{name: "dry-run-existing", wantDir: ".kiro/prompts", dryRun: true, existing: "User prompt.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			path := filepath.Join(tc.wantDir, "review.md")
			if tc.existing != "" {
				if err := os.MkdirAll(filepath.Join(dir, tc.wantDir), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, path), []byte(tc.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			body := "Review the change.\n\nKeep café examples intact.\n"
			entry := spec.Entry{
				Kind: spec.KindCommand, Name: "review", Path: "commands/review.md", Body: body,
				Meta: map[string]any{"description": "Review changes", "argument-hint": "<change>"},
			}
			cfg := &config.Config{
				OnUnsupported: emit.OnUnsupportedSilent,
				Outputs:       map[string]config.Output{target: {Dir: tc.targetDir, CommandsDir: tc.commandsDir}},
			}
			artifacts := New().NativeArtifacts(cfg)
			if len(artifacts) != 1 || artifacts[0].Label != "Commands" || artifacts[0].Location != filepath.ToSlash(tc.wantDir)+"/" {
				t.Errorf("native artifacts = %#v, want Commands at %s/", artifacts, tc.wantDir)
			}
			sess := emit.NewSession()
			sess.StartRecording()
			if err := New().Emit(sess, spec.NewBundle([]spec.Entry{entry}), cfg, tc.dryRun); err != nil {
				t.Fatalf("emit: %v", err)
			}
			if paths := sess.StopRecording(); !slices.Contains(paths, path) {
				t.Errorf("recorded paths = %v, want prompt %s", paths, path)
			}
			data, err := os.ReadFile(filepath.Join(dir, path))
			if tc.dryRun && tc.existing == "" {
				if !os.IsNotExist(err) {
					t.Errorf("dry-run prompt read error = %v, want not exist", err)
				}
			} else {
				if err != nil {
					t.Fatalf("read prompt: %v", err)
				}
				if tc.dryRun {
					if string(data) != tc.existing {
						t.Errorf("dry-run changed existing prompt: %q", data)
					}
				} else {
					if !header.Leads(path, string(data)) {
						t.Errorf("prompt lacks leading provenance: %s", data)
					}
					if got := header.Strip(string(data)); got != body {
						t.Errorf("prompt body = %q, want %q", got, body)
					}
				}
			}
			if tc.wantDir != ".kiro/prompts" {
				if _, err := os.Stat(filepath.Join(dir, ".kiro/prompts/review.md")); !os.IsNotExist(err) {
					t.Errorf("override also wrote default prompt: %v", err)
				}
			}
			if tc.commandsDir != "" && tc.targetDir != "" {
				if _, err := os.Stat(filepath.Join(dir, tc.targetDir, "prompts/review.md")); !os.IsNotExist(err) {
					t.Errorf("per-kind override also wrote target-dir prompt: %v", err)
				}
			}
		})
	}
}

func TestEmit_CommandPreservesNativeArgumentsWithoutWarnings(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCapabilityWarnings)
	body := "Review ${1} ${2} ${3} ${4} ${5} ${6} ${7} ${8} ${9} ${10}.\nAll: ${@}\nLegacy: $ARGUMENTS\n"
	entry := spec.Entry{Kind: spec.KindCommand, Name: "review", Path: "commands/review.md", Body: body}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{OnUnsupported: emit.OnUnsupportedError}, false); err != nil {
		t.Fatalf("emit native prompt arguments: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".kiro/prompts/review.md"))
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	if got := header.Strip(string(data)); got != body {
		t.Errorf("prompt arguments changed: got %q, want %q", got, body)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("native prompt generated %d capability warnings", got)
	}
}

func TestEmit_CommandPreservesLiteralLeadingYAML(t *testing.T) {
	cases := []struct {
		name       string
		blankLines string
	}{
		{name: "no-blank-line"},
		{name: "one-blank-line", blankLines: "\n"},
		{name: "several-blank-lines", blankLines: "\n\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			body := "---\nname: literal-name\ndescription: Literal YAML example\ncustom:\n  enabled: true\n---\n" + tc.blankLines + "Explain this document.\n"
			entry := spec.Entry{
				Kind: spec.KindCommand, Name: "explain", Path: "commands/explain.md", Body: body,
				Meta: map[string]any{"description": "Portable metadata outside the prompt"},
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{OnUnsupported: emit.OnUnsupportedError}, false); err != nil {
				t.Fatalf("emit literal YAML prompt: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".kiro/prompts/explain.md"))
			if err != nil {
				t.Fatalf("read prompt: %v", err)
			}
			content := string(data)
			if got := header.Strip(content); got != body {
				t.Errorf("literal YAML prompt changed: got %q, want %q", got, body)
			}
			if !strings.HasPrefix(content, header.Line(header.FormatMarkdown)) {
				t.Errorf("literal YAML became native frontmatter before provenance: %q", content)
			}
			if strings.Contains(content, "Portable metadata outside the prompt") {
				t.Errorf("portable metadata became a native prompt envelope: %q", content)
			}
		})
	}
}

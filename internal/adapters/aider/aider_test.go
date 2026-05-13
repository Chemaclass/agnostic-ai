package aider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesConventions(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "aider" {
		t.Errorf("expected aider, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1.md", Body: "skill body"},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "CONVENTIONS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"rule body",
		"## Skills",
		"### sk1",
		"skills/sk1.md",
		"<!-- source: rules/r1.md -->",
		"<!-- source: skills/sk1.md -->",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".aider.conf.yml")); !os.IsNotExist(err) {
		t.Fatalf("conf-file should not exist by default, got err=%v", err)
	}
}

func TestEmit_WritesConfFileWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {
				ConfFile:  ".aider.conf.yml",
				Model:     "gpt-4o",
				WeakModel: "gpt-4o-mini",
			},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	doc := readConf(t, filepath.Join(dir, ".aider.conf.yml"))
	if got := doc["model"]; got != "gpt-4o" {
		t.Errorf("model: want gpt-4o, got %v", got)
	}
	if got := doc["weak-model"]; got != "gpt-4o-mini" {
		t.Errorf("weak-model: want gpt-4o-mini, got %v", got)
	}
	want := []any{"CONVENTIONS.md"}
	if got, _ := doc["read"].([]any); !equalAny(got, want) {
		t.Errorf("read: want %v, got %v", want, got)
	}
}

func TestEmit_ConfFileMergesPreservingUserKeys(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	existing := "auto-commits: false\nread:\n  - notes.md\nmodel: legacy-model\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: ".aider.conf.yml"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	doc := readConf(t, filepath.Join(dir, ".aider.conf.yml"))
	if got := doc["auto-commits"]; got != false {
		t.Errorf("auto-commits preserved: want false, got %v", got)
	}
	if got := doc["model"]; got != "legacy-model" {
		t.Errorf("user model preserved: want legacy-model, got %v", got)
	}
	got, _ := doc["read"].([]any)
	want := []any{"notes.md", "CONVENTIONS.md"}
	if !equalAny(got, want) {
		t.Errorf("read merged: want %v, got %v", want, got)
	}
}

func TestEmit_ConfFileDoesNotDuplicateReadEntry(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	existing := "read:\n  - CONVENTIONS.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: ".aider.conf.yml"},
		},
	}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	doc := readConf(t, filepath.Join(dir, ".aider.conf.yml"))
	got, _ := doc["read"].([]any)
	want := []any{"CONVENTIONS.md"}
	if !equalAny(got, want) {
		t.Errorf("read: want %v, got %v", want, got)
	}
}

func TestEmit_ConfFilePromotesScalarRead(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	existing := "read: notes.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: ".aider.conf.yml"},
		},
	}
	if err := New().Emit(spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	doc := readConf(t, filepath.Join(dir, ".aider.conf.yml"))
	got, _ := doc["read"].([]any)
	want := []any{"notes.md", "CONVENTIONS.md"}
	if !equalAny(got, want) {
		t.Errorf("read promoted: want %v, got %v", want, got)
	}
}

func readConf(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func equalAny(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

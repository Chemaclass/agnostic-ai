package aider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// TestEmit_ConfFileCaptureReadsExistingUserKeys regresses #465. Under
// capture mode (sync --check / doctor), the conf reader must read the
// existing .aider.conf.yml so the captured bytes carry the user's keys.
// Otherwise doctor reports false drift and --fix writes the managed keys
// only, deleting the user's config.
func TestEmit_ConfFileCaptureReadsExistingUserKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	const existing = "auto-commits: false\nmodel: legacy-model\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: filepath.Join(dir, ".aider.conf.yml")},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}

	sess := emit.NewSession()
	sess.StartCapture()
	err := New().Emit(sess, spec.NewBundle(entries), cfg, false)
	files := sess.StopCapture()
	if err != nil {
		t.Fatal(err)
	}

	var conf string
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".aider.conf.yml") {
			conf = f.Content
		}
	}
	if conf == "" {
		t.Fatalf("no .aider.conf.yml captured: %v", files)
	}
	for _, want := range []string{"auto-commits", "legacy-model", "CONVENTIONS.md"} {
		if !strings.Contains(conf, want) {
			t.Errorf("captured conf missing %q:\n%s", want, conf)
		}
	}
}

func TestEmit_NoConventionsByDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	a := New()
	if a.Name() != "aider" {
		t.Errorf("expected aider, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1.md", Body: "skill body"},
	}
	if err := a.Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CONVENTIONS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write CONVENTIONS.md by default; sync owns the entry-point, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".aider.conf.yml")); !os.IsNotExist(err) {
		t.Fatalf("conf-file should not exist by default, got err=%v", err)
	}
}

func TestEmit_LegacyRulesFile_WritesConcatenated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cfg := &config.Config{
		Outputs: map[string]config.Output{"aider": {RulesFile: filepath.Join(dir, "CONVENTIONS.md")}},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
		{Kind: spec.KindSkill, Name: "sk1", Path: "skills/sk1.md", Body: "skill body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
		"<!-- source: rules/r1.md -->",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_WritesConfFileWhenConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {
				ConfFile:  filepath.Join(dir, ".aider.conf.yml"),
				Model:     "gpt-4o",
				WeakModel: "gpt-4o-mini",
			},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	t.Parallel()
	dir := t.TempDir()

	existing := "auto-commits: false\nread:\n  - notes.md\nmodel: legacy-model\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: filepath.Join(dir, ".aider.conf.yml")},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
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
	t.Parallel()
	dir := t.TempDir()

	existing := "read:\n  - CONVENTIONS.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: filepath.Join(dir, ".aider.conf.yml")},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
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
	t.Parallel()
	dir := t.TempDir()

	existing := "read: notes.md\n"
	if err := os.WriteFile(filepath.Join(dir, ".aider.conf.yml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"aider": {ConfFile: filepath.Join(dir, ".aider.conf.yml")},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
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

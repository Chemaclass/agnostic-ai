package adapters

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestProtectedPathsEnforcement_NamesTheNativeMechanism(t *testing.T) {
	want := map[string]string{"claude": "permission", "codex": "hook", "cursor": "permission", "gemini": "hook"}
	for _, name := range Names() {
		if got := ProtectedPathsEnforcement(name); got != want[name] {
			t.Errorf("%s enforcement = %q, want %q", name, got, want[name])
		}
	}
}

func TestEmitWithProvenance_NotesAdvisoryProtectedPaths(t *testing.T) {
	testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "p", Path: "settings/p.yaml",
		Meta: map[string]any{"protected": map[string]any{"paths": []any{".github/**"}}}}
	b := spec.NewBundle([]spec.Entry{settings})
	var notes bytes.Buffer
	old := emit.Warner
	emit.Warner = &notes
	ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
	for _, target := range []string{"claude", "codex", "gemini", "kilo"} {
		adapter, err := Resolve(target)
		if err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{Targets: []string{target}}
		if err := EmitWithProvenance(NewSession(), adapter, b, cfg, false); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
	}
	FlushCoverageNotes()
	out := notes.String()
	for _, target := range []string{"kilo"} {
		if !strings.Contains(out, target) || !strings.Contains(out, "protected") {
			t.Errorf("no protected note for %s:\n%s", target, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "protected") && slices.ContainsFunc([]string{"claude", "codex", "gemini"}, func(t string) bool { return strings.Contains(line, t+":") }) {
			t.Errorf("enforcing target noted as advisory: %s", line)
		}
	}
}

func TestEmitWithProvenance_RejectsAnInvalidProtectedBlockOnEveryTarget(t *testing.T) {
	testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "p", Path: "settings/p.yaml",
		Meta: map[string]any{"protected": map[string]any{"paths": []any{"a"}, "decision": "maybe"}}}
	adapter, err := Resolve("kilo")
	if err != nil {
		t.Fatal(err)
	}
	err = EmitWithProvenance(NewSession(), adapter, spec.NewBundle([]spec.Entry{settings}), &config.Config{Targets: []string{"kilo"}}, false)
	if err == nil || !strings.Contains(err.Error(), "settings/p.yaml") {
		t.Fatalf("err = %v, want one naming the spec", err)
	}
}

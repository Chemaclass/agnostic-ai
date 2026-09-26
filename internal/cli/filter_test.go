package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestFilterTargets_NoFilter(t *testing.T) {
	configured := []string{"claude", "cursor", "codex"}
	got, err := filterTargets(configured, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 targets, got %d", len(got))
	}
}

func TestFilterTargets_Only(t *testing.T) {
	configured := []string{"claude", "cursor", "codex"}
	got, err := filterTargets(configured, []string{"claude", "cursor"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "claude" || got[1] != "cursor" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestFilterTargets_OnlyUnknown(t *testing.T) {
	configured := []string{"claude", "cursor"}
	_, err := filterTargets(configured, []string{"gemini"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown target in --only")
	}
}

func TestFilterTargets_Except(t *testing.T) {
	configured := []string{"claude", "cursor", "codex"}
	got, err := filterTargets(configured, nil, []string{"codex"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d: %v", len(got), got)
	}
	for _, t2 := range got {
		if t2 == "codex" {
			t.Error("codex should have been excluded")
		}
	}
}

func TestFilterTargets_ExceptUnknown(t *testing.T) {
	configured := []string{"claude", "cursor"}
	_, err := filterTargets(configured, nil, []string{"gemini"})
	if err == nil {
		t.Fatal("expected error for unknown target in --except")
	}
}

func TestFilterTargets_ExceptAll(t *testing.T) {
	configured := []string{"claude"}
	got, err := filterTargets(configured, nil, []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestFilterTargets_TypoSuggestsConfiguredTarget(t *testing.T) {
	_, err := filterTargets([]string{"claude", "cursor"}, []string{"cursr"}, nil)

	if err == nil || !strings.Contains(err.Error(), "did you mean cursor?") {
		t.Errorf("want a suggestion, got %v", err)
	}
}

func TestFilterTargets_KnownAdapterOutsideRunNamesTheRunTargets(t *testing.T) {
	_, err := filterTargets([]string{"claude"}, []string{"codex"}, nil)

	if err == nil || !strings.HasSuffix(err.Error(), "codex is not in this run's targets (claude)") {
		t.Errorf("got %v", err)
	}
}

func TestFilterTargets_EmptyRunOmitsTheList(t *testing.T) {
	_, err := filterTargets(nil, []string{"codex"}, nil)

	if err == nil || !strings.HasSuffix(err.Error(), "codex is not in this run's targets") {
		t.Errorf("got %v", err)
	}
}

func TestSyncGlobal_MistypedTargetSuggestsClosestName(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	silence(t)

	err := runSync(t, "--global", "-t", "claud")

	if err == nil || !strings.Contains(err.Error(), "did you mean claude?") {
		t.Errorf("got %v", err)
	}
}

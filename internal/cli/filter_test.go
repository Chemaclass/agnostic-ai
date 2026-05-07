package cli

import (
	"testing"
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

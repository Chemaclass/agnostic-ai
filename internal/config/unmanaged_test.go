package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestMatchUnmanaged(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{"exact path", []string{".cursor/rules/legacy.mdc"}, ".cursor/rules/legacy.mdc", true},
		{"glob matches within a segment", []string{".claude/agents/hand-*.md"}, ".claude/agents/hand-x.md", true},
		{"glob does not cross a slash", []string{".claude/*.md"}, ".claude/agents/x.md", false},
		{"dir prefix matches nested file", []string{".claude/skills/legacy/"}, ".claude/skills/legacy/assets/a.txt", true},
		{"dir prefix matches the dir itself", []string{".claude/skills/legacy/"}, ".claude/skills/legacy", true},
		{"dir prefix does not match a sibling name", []string{".claude/skills/legacy/"}, ".claude/skills/legacy-two/SKILL.md", false},
		{"leading dot slash on pattern", []string{"./AGENTS.md"}, "AGENTS.md", true},
		{"leading slash on path", []string{"AGENTS.md"}, "/AGENTS.md", true},
		{"backslash path input", []string{".claude/agents/x.md"}, `.claude\agents\x.md`, true},
		{"no match", []string{"AGENTS.md"}, "CLAUDE.md", false},
		{"empty pattern ignored", []string{""}, "", false},
		{"no patterns", nil, "AGENTS.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchUnmanaged(tc.patterns, tc.path); got != tc.want {
				t.Errorf("MatchUnmanaged(%q, %q) = %v, want %v", tc.patterns, tc.path, got, tc.want)
			}
		})
	}
}

func TestMatchUnmanagedDir(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		dir      string
		want     bool
	}{
		{"exact file inside", []string{".claude/agents/hand.md"}, ".claude/agents", true},
		{"glob file inside", []string{".claude/agents/hand-*.md"}, ".claude/agents", true},
		{"glob in a directory segment", []string{".claude/*/hand.md"}, ".claude/agents", true},
		{"deeper directory pattern", []string{".claude/skills/legacy/"}, ".claude/skills", true},
		{"directory pattern equal to dir", []string{".claude/skills/"}, ".claude/skills", true},
		{"directory pattern above dir", []string{".claude/"}, ".claude/skills", true},
		{"file in a sibling dir", []string{".claude/rules/hand.md"}, ".claude/agents", false},
		{"file pattern naming the dir itself", []string{".claude/agents"}, ".claude/agents", false},
		{"root file", []string{"AGENTS.md"}, ".claude/agents", false},
		{"leading slash and dot", []string{"./.claude/agents/x.md"}, "/.claude/agents/", true},
		{"no patterns", nil, ".claude/agents", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchUnmanagedDir(tc.patterns, tc.dir); got != tc.want {
				t.Errorf("MatchUnmanagedDir(%q, %q) = %v, want %v", tc.patterns, tc.dir, got, tc.want)
			}
		})
	}
}

func TestLoad_RejectsBadUnmanagedPattern(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nsync:\n  unmanaged:\n    - \"[bad\"\n"
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a malformed sync.unmanaged glob")
	}
	if got := errs.CodeOf(err); got != errs.CodeConfigDecode {
		t.Errorf("code = %q, want %q (err: %v)", got, errs.CodeConfigDecode, err)
	}
}

func TestLoad_RejectsUnmanagedEntryThatNamesNothing(t *testing.T) {
	for _, entry := range []string{"", ".", "./", "/"} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			body := "version: 1\nsync:\n  unmanaged:\n    - \"" + entry + "\"\n"
			if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(dir)
			if got := errs.CodeOf(err); got != errs.CodeConfigDecode {
				t.Errorf("entry %q: code = %q, want %q (err: %v)", entry, got, errs.CodeConfigDecode, err)
			}
		})
	}
}

func TestLoad_ReadsUnmanagedList(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nsync:\n  unmanaged:\n    - AGENTS.md\n    - .claude/skills/legacy/\n"
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsUnmanaged("AGENTS.md") || !cfg.IsUnmanaged(".claude/skills/legacy/SKILL.md") {
		t.Errorf("loaded list not honored: %q", cfg.Sync.Unmanaged)
	}
}

func TestConfig_IsUnmanaged_NilSafe(t *testing.T) {
	var cfg *Config
	if cfg.IsUnmanaged("AGENTS.md") {
		t.Error("nil config must own nothing")
	}
}

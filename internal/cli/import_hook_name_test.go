package cli

import (
	"regexp"
	"strings"
	"testing"
)

var hookNamePattern = regexp.MustCompile(`^[a-z]+(-[a-z0-9-]+)?-[0-9a-f]{8}$`)

func TestHookSpecName_MatchesExpectedShape(t *testing.T) {
	got := hookSpecName("PostToolUse", "Edit", []string{"fmt"})
	if !hookNamePattern.MatchString(got) {
		t.Errorf("name %q does not match expected pattern", got)
	}
	if !strings.HasPrefix(got, "posttooluse-edit-") {
		t.Errorf("expected lowercase event and slug prefix, got %q", got)
	}
}

func TestHookSpecName_Stable(t *testing.T) {
	a := hookSpecName("PostToolUse", "Edit", []string{"fmt", "lint"})
	b := hookSpecName("PostToolUse", "Edit", []string{"fmt", "lint"})
	if a != b {
		t.Errorf("same inputs produced different names: %q vs %q", a, b)
	}
}

func TestHookSpecName_StableAcrossCommandOrder(t *testing.T) {
	a := hookSpecName("PostToolUse", "Edit", []string{"fmt", "lint"})
	b := hookSpecName("PostToolUse", "Edit", []string{"lint", "fmt"})
	if a != b {
		t.Errorf("command order should not affect hash: %q vs %q", a, b)
	}
}

func TestHookSpecName_DiffersOnCommandChange(t *testing.T) {
	a := hookSpecName("PostToolUse", "Edit", []string{"fmt"})
	b := hookSpecName("PostToolUse", "Edit", []string{"fmt-different"})
	if a == b {
		t.Errorf("changing command should change hash, both got %q", a)
	}
}

func TestHookSpecName_DiffersOnMatcherChange(t *testing.T) {
	a := hookSpecName("PostToolUse", "Edit", []string{"fmt"})
	b := hookSpecName("PostToolUse", "Write", []string{"fmt"})
	if a == b {
		t.Errorf("changing matcher should change name, both got %q", a)
	}
}

func TestHookSpecName_OmitsEmptyMatcher(t *testing.T) {
	got := hookSpecName("PostToolUse", "", []string{"fmt"})
	parts := strings.Split(got, "-")
	if len(parts) != 2 {
		t.Fatalf("expected 2 segments without matcher, got %q (%d parts)", got, len(parts))
	}
	if parts[0] != "posttooluse" {
		t.Errorf("expected event prefix, got %q", got)
	}
}

func TestHookSpecName_OmitsMatcherWhenSlugEmpty(t *testing.T) {
	got := hookSpecName("PostToolUse", "!!!", []string{"fmt"})
	if strings.Count(got, "-") != 1 {
		t.Errorf("punctuation-only matcher should produce no slug segment, got %q", got)
	}
}

func TestHookSpecName_SlugifiesMatcher(t *testing.T) {
	got := hookSpecName("PostToolUse", "Edit|Write", []string{"fmt"})
	if !strings.HasPrefix(got, "posttooluse-edit-write-") {
		t.Errorf("expected punctuation collapsed to single dash, got %q", got)
	}
}

func TestHookSpecName_CapsLongMatcher(t *testing.T) {
	long := strings.Repeat("abcde", 20)
	got := hookSpecName("PostToolUse", long, []string{"fmt"})
	// stem is event ("posttooluse") + "-" + slug + "-" + 8 hex chars.
	// slug length is capped at hookSlugMaxLen (24).
	parts := strings.Split(got, "-")
	slug := strings.Join(parts[1:len(parts)-1], "-")
	if len(slug) > hookSlugMaxLen {
		t.Errorf("slug exceeded cap of %d: %q (len %d)", hookSlugMaxLen, slug, len(slug))
	}
}

func TestHookSpecName_EmptyEventFallsBack(t *testing.T) {
	got := hookSpecName("", "Edit", []string{"fmt"})
	if !strings.HasPrefix(got, "hook-") {
		t.Errorf("expected fallback event prefix, got %q", got)
	}
}

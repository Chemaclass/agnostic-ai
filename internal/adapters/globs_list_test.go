package adapters

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globRule is a rule that loads only on Go files, with its globs in the
// given form.
func globRule(globs any, meta map[string]any) spec.Entry {
	m := map[string]any{"name": "go", "alwaysApply": false}
	if globs != nil {
		m["globs"] = globs
	}
	for k, v := range meta {
		m[k] = v
	}
	return spec.Entry{Kind: spec.KindRule, Name: "go", Path: "rules/go.md", Meta: m, Body: "Go body."}
}

func captureRule(t *testing.T, target string, r spec.Entry) map[string]string {
	t.Helper()
	a, err := Resolve(target)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartCapture()
	if err := EmitWithProvenance(sess, a, spec.NewBundle([]spec.Entry{r}), &config.Config{}, false); err != nil {
		sess.StopCapture()
		t.Fatalf("%s: %v", target, err)
	}
	out := map[string]string{}
	for _, f := range sess.StopCapture() {
		out[f.Path] = f.Content
	}
	return out
}

// A YAML-list `globs` names the same patterns as the comma-joined string
// (#1234). It used to read as empty on these targets, so the rule loaded
// in every session.
func TestGlobsList_EmitsLikeTheCommaString(t *testing.T) {
	list := []any{"*.go", "*.mod"}
	for _, target := range []string{"cline", "copilot", "cursor", "trae", "kiro", "windsurf", "openhands"} {
		t.Run(target, func(t *testing.T) {
			for name, pair := range map[string][2]spec.Entry{
				"globs":                  {globRule(list, nil), globRule("*.go,*.mod", nil)},
				"x-" + target + ".globs": {globRule(nil, map[string]any{"x-" + target: map[string]any{"globs": list}}), globRule(nil, map[string]any{"x-" + target: map[string]any{"globs": "*.go,*.mod"}})},
			} {
				fromList, fromString := captureRule(t, target, pair[0]), captureRule(t, target, pair[1])
				if len(fromString) == 0 {
					t.Fatalf("%s: the string form wrote nothing", name)
				}
				for path, want := range fromString {
					if got := fromList[path]; got != want {
						t.Errorf("%s: %s differs\n--- list ---\n%s\n--- string ---\n%s", name, path, got, want)
					}
				}
				if len(fromList) != len(fromString) {
					t.Errorf("%s: list wrote %v, string wrote %v", name, keys(fromList), keys(fromString))
				}
				if AlwaysOnRule(target, pair[0]) != AlwaysOnRule(target, pair[1]) {
					t.Errorf("%s: AlwaysOnRule differs between the list and the string", name)
				}
			}
		})
	}
}

// Claude and Continue write a list natively and Antigravity joins it, so
// their bytes differ from the string form, but the rule must still load
// only on a match.
func TestGlobsList_StaysScopedWhereWrittenNatively(t *testing.T) {
	for _, target := range []string{"claude", "continue", "antigravity"} {
		if AlwaysOnRule(target, globRule([]any{"*.go", "*.mod"}, nil)) {
			t.Errorf("%s: a list globs rule must not load in every session", target)
		}
	}
}

func TestEntryGlobs_JoinsAList(t *testing.T) {
	if got := globRule([]any{"*.go", "*.mod"}, nil).Globs(); got != "*.go,*.mod" {
		t.Errorf("Globs() = %q, want *.go,*.mod", got)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// A brace set holds commas that do not separate patterns, in the string
// form and in a list item alike.
func TestGlobs_BraceSetStaysOnePattern(t *testing.T) {
	for name, globs := range map[string]any{
		"string": "src/**/*.{ts,tsx},lib/*.js",
		"list":   []any{"src/**/*.{ts,tsx}", "lib/*.js"},
	} {
		got := captureRule(t, "cline", globRule(globs, nil))[filepath.FromSlash(".clinerules/go.md")]
		if !strings.Contains(got, "  - src/**/*.{ts,tsx}\n") || !strings.Contains(got, "  - lib/*.js\n") || strings.Contains(got, "{ts\n") {
			t.Errorf("%s: cline paths must keep the brace set whole:\n%s", name, got)
		}
		got = captureRule(t, "openhands", globRule(globs, nil))[filepath.FromSlash(".agents/skills/go/SKILL.md")]
		if !strings.Contains(got, "src/**/*.{ts,tsx}") || strings.Contains(got, "  - tsx}") {
			t.Errorf("%s: openhands triggers must keep the brace set whole:\n%s", name, got)
		}
	}
	got := captureRule(t, "cline", globRule("src/**/*.{ts,tsx}", nil))[filepath.FromSlash(".clinerules/go.md")]
	if !strings.Contains(got, "paths:\n  - src/**/*.{ts,tsx}\n---") {
		t.Errorf("a lone brace-set string must stay one cline path:\n%s", got)
	}
}

// Kiro takes several patterns as a YAML array; one pattern stays a string.
func TestGlobs_KiroWritesSeveralPatternsAsAList(t *testing.T) {
	for name, globs := range map[string]any{
		"string": "*.go,*.mod",
		"list":   []any{"*.go", "*.mod"},
	} {
		got := captureRule(t, "kiro", globRule(globs, nil))[filepath.FromSlash(".kiro/steering/go.md")]
		if !strings.Contains(got, "fileMatchPattern:\n  - \"*.go\"\n  - \"*.mod\"\n") && !strings.Contains(got, "fileMatchPattern:\n    - '*.go'\n    - '*.mod'\n") {
			t.Errorf("%s: kiro fileMatchPattern must list both patterns:\n%s", name, got)
		}
	}
	got := captureRule(t, "kiro", globRule([]any{"*.go"}, nil))[filepath.FromSlash(".kiro/steering/go.md")]
	if !strings.Contains(got, "fileMatchPattern: ") {
		t.Errorf("one pattern must stay a string:\n%s", got)
	}
}

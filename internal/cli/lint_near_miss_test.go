package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A near-miss of a key the tool owns is not an extension, it is a typo
// that silently disarms a setting. Nine phel-lang agents ran with full
// tool access because `allowed_tools:` parsed, emitted, and did
// nothing (#617).
func TestLintNearMissKeys_FlagsAllowedTools(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"allowed_tools": []any{"Read"}}, Body: "b",
	}}

	findings := lintNearMissKeys(entries, nil)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(findings), findings)
	}
	if !strings.Contains(findings[0].Message, "tools") {
		t.Errorf("message must name the key it meant: %q", findings[0].Message)
	}
	if findings[0].Severity != lintWarn {
		t.Errorf("expected warn so --strict gates it, got %v", findings[0].Severity)
	}
}

func TestLintNearMissKeys_FlagsEveryDocumentedNearMiss(t *testing.T) {
	for _, key := range []string{"allowed_tools", "allowedTools", "disallowed_tools", "max_turns", "model_name"} {
		entries := []spec.Entry{{
			Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
			Meta: map[string]any{key: "x"}, Body: "b",
		}}
		if got := lintNearMissKeys(entries, nil); len(got) != 1 {
			t.Errorf("%s: expected a finding, got %v", key, got)
		}
	}
}

// The real key must stay silent, or the rule punishes correct specs.
func TestLintNearMissKeys_IgnoresCorrectKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"tools": []any{"Read"}, "model": "opus", "description": "d"}, Body: "b",
	}}

	if got := lintNearMissKeys(entries, nil); len(got) != 0 {
		t.Errorf("correct keys must not be flagged, got %v", got)
	}
}

// Passthrough is the right default for genuine extensions. A key under
// x-<target> is deliberate, including target-native spellings like
// Junie's real disallowedTools.
func TestLintNearMissKeys_IgnoresTargetNamespacedKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"x-junie": map[string]any{"disallowedTools": []any{"Bash"}}}, Body: "b",
	}}

	if got := lintNearMissKeys(entries, nil); len(got) != 0 {
		t.Errorf("x-<target> keys are deliberate, got %v", got)
	}
}

// An unrelated custom key is a genuine extension, not a near miss.
func TestLintNearMissKeys_IgnoresUnrelatedKeys(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"owner": "platform-team"}, Body: "b",
	}}

	if got := lintNearMissKeys(entries, nil); len(got) != 0 {
		t.Errorf("unknown keys pass through by design, got %v", got)
	}
}

// The rule has to be wired in, not merely defined.
func TestCollectLintFindings_IncludesNearMissKeys(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{
		Kind: spec.KindAgent, Name: "a", Path: "agents/a.md",
		Meta: map[string]any{"allowed_tools": []any{"Read"}}, Body: "b",
	}})

	var found bool
	for _, f := range collectLintFindings([]string{"claude"}, b) {
		if strings.Contains(f.Message, "allowed_tools") {
			found = true
		}
	}
	if !found {
		t.Error("lint must report the near-miss key; a rule nobody calls fixes nothing")
	}
}

// A key one edit from a documented key is a typo, not an extension:
// `glob:` leaves a rule meant for Go files applying everywhere.
func TestLintNearMissKeys_FlagsTyposOfDocumentedKeys(t *testing.T) {
	cases := map[string]string{"glob": "globs", "descriptin": "description", "alwaysapply": "alwaysApply", "matchr": "matcher", "mode": "model"}
	for key, want := range cases {
		entries := []spec.Entry{{
			Kind: spec.KindRule, Name: "r", Path: "rules/r.md",
			Meta: map[string]any{key: "x"}, Body: "b",
		}}

		got := lintNearMissKeys(entries, nil)

		if len(got) != 1 || !strings.Contains(got[0].Message, "`"+want+":`") {
			t.Errorf("%s: want a finding naming %s, got %v", key, want, got)
		}
	}
}

// Environments and settings pass their keys through to native files, so a
// key close to a spec field there is the tool's own, not a typo.
func TestLintNearMissKeys_SkipsPassthroughKinds(t *testing.T) {
	for _, kind := range []spec.Kind{spec.KindEnvironment, spec.KindSettings} {
		entries := []spec.Entry{{
			Kind: kind, Name: "e", Path: "e.yaml",
			Meta: map[string]any{"ports": "x", "globs": "x", "glob": "x"},
		}}
		if got := lintNearMissKeys(entries, nil); len(got) != 0 {
			t.Errorf("%s keys pass through, got %v", kind, got)
		}
	}
}

// Every field the spec-format page documents must count as known, or a
// correct spec would be flagged as a typo of its neighbor.
func TestSpecKeys_CoverEveryDocumentedField(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "content", "docs", "spec-format.md"))
	if err != nil {
		t.Fatal(err)
	}
	field := regexp.MustCompile("`([A-Za-z][A-Za-z0-9_-]*)`")
	skip := map[string]bool{"## Settings": true, "## Target-specific extensions: `x-<target>` namespace": true}
	section, header := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			section = line
		}
		if !strings.HasPrefix(line, "|") {
			header = ""
			continue
		}
		if header == "" {
			header = line
		}
		if skip[section] {
			continue
		}
		cols := strings.Split(line, "|")
		// A field row names keys in its first column; an "Extra fields"
		// row ("| [Kiro](...) | `autoApprove`, ... |") in its second.
		keys := cols[1]
		switch {
		case strings.HasPrefix(line, "| `"):
		case strings.HasPrefix(line, "| [") && len(cols) > 2 && strings.Contains(header, "Extra fields"):
			keys = cols[2]
		default:
			continue
		}
		for _, m := range field.FindAllStringSubmatch(keys, -1) {
			if _, targetOnly := targetKeys[m[1]]; !targetOnly && !slices.Contains(specKeys, m[1]) {
				t.Errorf("%s documents `%s`, missing from specKeys", section, m[1])
			}
		}
	}
}

func TestRunSyncOnce_WarnsAboutAMistypedKey(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nglob: \"*.go\"\n---\n\nUse gofmt.\n")
	silence(t)
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "! .agnostic-ai/rules/go.md: `glob:` is read only by qoder, not a target here") {
		t.Errorf("sync should name the mistyped key:\n%s", buf.String())
	}
}

// Some top-level keys belong to one target: Qoder reads `glob:`, OpenCode
// and Kilo read `mode:`. A project that targets one of them means it.
func TestLintNearMissKeys_TargetKeyIsFineWhenItsTargetIsConfigured(t *testing.T) {
	for key, target := range map[string]string{"glob": "qoder", "mode": "opencode"} {
		entries := []spec.Entry{{
			Kind: spec.KindRule, Name: "r", Path: "rules/r.md",
			Meta: map[string]any{key: "x"}, Body: "b",
		}}
		if got := lintNearMissKeys(entries, []string{"claude", target}); len(got) != 0 {
			t.Errorf("%s with %s configured: got %v", key, target, got)
		}
	}
}

func TestLintNearMissKeys_TargetKeyNamesItsReaders(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindRule, Name: "r", Path: "rules/r.md",
		Meta: map[string]any{"glob": "*.go"}, Body: "b",
	}}

	got := lintNearMissKeys(entries, []string{"claude", "cursor"})

	if len(got) != 1 || !strings.Contains(got[0].Message, "`glob:` is read only by qoder, not a target here") ||
		!strings.Contains(got[0].Message, "Did you mean `globs:`?") {
		t.Errorf("got %v", got)
	}
}

// Keys only some targets read, documented in the MCP extra-fields table.
func TestLintNearMissKeys_IgnoresTargetMCPFields(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindMCP, Name: "m", Path: "mcps/m.yaml",
		Meta: map[string]any{"disabledTools": []any{"x"}, "autoApprove": []any{"y"}, "envFile": ".env"},
	}}

	if got := lintNearMissKeys(entries, nil); len(got) != 0 {
		t.Errorf("documented MCP fields must pass, got %v", got)
	}
}

package opencode

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// projectConfig reads the emitted opencode.json back from disk.
func projectConfig(t *testing.T, dir string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "opencode.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func settingsEntry(name string, lists map[string]any) spec.Entry {
	return spec.Entry{Kind: spec.KindSettings, Name: name, Meta: map[string]any{"permissions": lists}}
}

func TestBuildPermissions(t *testing.T) {
	cases := []struct {
		name    string
		entries []spec.Entry
		want    map[string]any
		dropped int
	}{
		{
			// A bare tool name covers the whole tool, which is
			// OpenCode's own simplest form.
			name:    "bare rules take the string form",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"allow": []any{"Read"}, "deny": []any{"Bash"}})},
			want:    map[string]any{"read": "allow", "bash": "deny"},
		},
		{
			// The colon convention is this project's prefix spelling.
			// OpenCode matches a bare glob against the command, and
			// its own examples write that as `git *`.
			name:    "a prefix rule becomes a glob pattern",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"allow": []any{"Bash(go test:*)"}})},
			want:    map[string]any{"bash": map[string]any{"go test *": "allow"}},
		},
		{
			// No colon means the author wrote an exact command, and
			// every character outside * and ? is literal to OpenCode.
			name:    "an exact command stays exact",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"deny": []any{"Bash(rm -rf /)"}})},
			want:    map[string]any{"bash": map[string]any{"rm -rf /": "deny"}},
		},
		{
			// A bare rule alongside scoped ones becomes the catch-all.
			// OpenCode is last-match-wins and asks for the catch-all
			// first, which is where "*" sorts.
			name: "a bare rule joins its scoped siblings as the catch-all",
			entries: []spec.Entry{settingsEntry("a", map[string]any{
				"ask":   []any{"Bash"},
				"allow": []any{"Bash(git:*)"},
				"deny":  []any{"Bash(rm:*)"},
			})},
			want: map[string]any{"bash": map[string]any{
				"*": "ask", "git *": "allow", "rm *": "deny",
			}},
		},
		{
			// OpenCode's `edit` covers edit, write and patch, so both
			// portable spellings land on it.
			name: "Write and Edit both land on edit",
			entries: []spec.Entry{settingsEntry("a", map[string]any{
				"deny": []any{"Write(.env*)", "Edit(secrets/**)"},
			})},
			want: map[string]any{"edit": map[string]any{
				".env*": "deny", "secrets/**": "deny",
			}},
		},
		{
			// Two lists claiming the same tool and pattern is a
			// conflict the author did not resolve. Take the most
			// restrictive, which is the only safe direction.
			name: "the most restrictive action wins a collision",
			entries: []spec.Entry{settingsEntry("a", map[string]any{
				"allow": []any{"Bash(rm:*)"},
				"deny":  []any{"Bash(rm:*)"},
			})},
			want: map[string]any{"bash": map[string]any{"rm *": "deny"}},
		},
		{
			// webfetch is typed as a bare action in OpenCode's schema,
			// with no pattern object, so a scoped rule has no form.
			name:    "a scoped rule on an action-only tool drops",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"allow": []any{"WebFetch(https://example.test)"}})},
			want:    nil,
			dropped: 1,
		},
		{
			name:    "a bare rule on an action-only tool maps",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"allow": []any{"WebFetch"}})},
			want:    map[string]any{"webfetch": "allow"},
		},
		{
			// OpenCode's vocabulary has no MCP-scoped key at all.
			name:    "an MCP rule has no key",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"deny": []any{"mcp__github__list_issues"}})},
			want:    nil,
			dropped: 1,
		},
		{
			name:    "an unknown tool drops",
			entries: []spec.Entry{settingsEntry("a", map[string]any{"allow": []any{"Telepathy"}})},
			want:    nil,
			dropped: 1,
		},
		{
			// One spec counts once however many of its rules drop, so
			// the coverage note reads per file rather than per rule.
			name: "a spec with several unmappable rules counts once",
			entries: []spec.Entry{settingsEntry("a", map[string]any{
				"allow": []any{"Telepathy", "mcp__x__y"},
				"deny":  []any{"Read(src/**)"},
			})},
			want:    map[string]any{"read": map[string]any{"src/**": "deny"}},
			dropped: 1,
		},
		{
			name: "specs merge across files",
			entries: []spec.Entry{
				settingsEntry("a", map[string]any{"allow": []any{"Read"}}),
				settingsEntry("b", map[string]any{"deny": []any{"Bash(rm:*)"}}),
			},
			want: map[string]any{"read": "allow", "bash": map[string]any{"rm *": "deny"}},
		},
		{
			name:    "no permissions at all",
			entries: []spec.Entry{{Kind: spec.KindSettings, Name: "a", Meta: map[string]any{"model": "x"}}},
			want:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, dropped := buildPermissions(tc.entries)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildPermissions() = %#v, want %#v", got, tc.want)
			}
			if dropped != tc.dropped {
				t.Errorf("dropped = %d, want %d", dropped, tc.dropped)
			}
		})
	}
}

// TestBuildPermissions_NativeOverrideWinsPerTool mirrors the escape
// hatch kilo and augment already give: an author who writes OpenCode's
// own spelling is presumed to mean it, and it replaces the translated
// rules for that tool without wiping the others.
func TestBuildPermissions_NativeOverrideWinsPerTool(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindSettings,
		Name: "a",
		Meta: map[string]any{
			"permissions": map[string]any{
				"allow": []any{"Bash(git:*)", "Read"},
			},
			"x-opencode": map[string]any{
				"permission": map[string]any{
					"bash": map[string]any{"*": "deny"},
				},
			},
		},
	}}
	got, dropped := buildPermissions(entries)
	want := map[string]any{
		"bash": map[string]any{"*": "deny"},
		"read": "allow",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildPermissions() = %#v, want %#v", got, want)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
}

// TestEmit_WritesPermissionIntoProjectConfig is the end-to-end check
// that the map reaches opencode.json rather than only the builder.
func TestEmit_WritesPermissionIntoProjectConfig(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{settingsEntry("base", map[string]any{
		"allow": []any{"Read"},
		"deny":  []any{"Bash(rm:*)"},
	})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	doc := projectConfig(t, dir)
	perms, ok := doc["permission"].(map[string]any)
	if !ok {
		t.Fatalf("no permission key in opencode.json: %#v", doc)
	}
	if perms["read"] != "allow" {
		t.Errorf("read = %#v, want allow", perms["read"])
	}
	bash, _ := perms["bash"].(map[string]any)
	if bash["rm *"] != "deny" {
		t.Errorf("bash = %#v, want rm * deny", perms["bash"])
	}
}

// TestEmit_NotesRulesWithNoOpenCodeKey keeps the unmappable rules
// visible instead of dropping them in silence, the failure #917 was
// filed for.
func TestEmit_NotesRulesWithNoOpenCodeKey(t *testing.T) {
	testutil.TempCwd(t)
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{settingsEntry("base", map[string]any{
		"deny": []any{"mcp__github__list_issues"},
	})}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"`permissions`", "opencode", "x-opencode"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
}

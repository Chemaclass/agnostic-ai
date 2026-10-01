package cursor

import (
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCLIPermissionRule_TranslatesOnlyFaithfulSpellings(t *testing.T) {
	cases := []struct {
		rule, list, want string
	}{
		{"Bash(git:*)", "allow", "Shell(git)"},
		{"Bash(git *)", "allow", "Shell(git)"},
		{"Bash(rm:*)", "deny", "Shell(rm)"},
		{"Bash(rm)", "deny", ""},
		{"Bash(ls)", "allow", ""},
		{"Bash(go test:*)", "allow", ""},
		{"Bash(git push:*)", "deny", ""},
		{"Bash(rm -rf:*)", "deny", ""},
		{"Bash(rm -rf /)", "deny", ""},
		{"Bash(ls*)", "allow", ""},
		{"Bash", "deny", ""},
		{"Read(src/**/*.ts)", "allow", "Read(src/**/*.ts)"},
		{"Read(src/**/*.ts)", "deny", "Read(src/**/*.ts)"},
		{"Read(./.env*)", "deny", "Read(.env*)"},
		{"Read(/docs/**)", "allow", "Read(docs/**)"},
		{"Read(secrets)", "allow", "Read(secrets)"},
		{"Read(secrets)", "deny", "Read(secrets), Read(secrets/**)"},
		{"Read(//etc/passwd)", "deny", "Read(/etc/passwd), Read(/etc/passwd/**)"},
		{"Read(~/.ssh/**)", "deny", ""},
		{"Read(src/{a,b}.ts)", "allow", ""},
		{"Read(!src/**)", "deny", ""},
		{"Edit(src/**)", "allow", "Write(src/**)"},
		{"Edit(/vendor)", "deny", "Write(vendor), Write(vendor/**)"},
		{"Write(**/*.key)", "deny", "Write(**/*.key)"},
		{"WebFetch(domain:docs.github.com)", "allow", "WebFetch(docs.github.com)"},
		{"WebFetch(domain:*.github.com)", "allow", "WebFetch(*.github.com)"},
		{"WebFetch(domain:*)", "deny", "WebFetch(*)"},
		{"WebFetch(domain:*.)", "allow", ""},
		{"WebFetch(domain:example.com/docs)", "allow", ""},
		{"WebFetch(domain:example.com:8080)", "deny", ""},
		{"WebFetch(domain:example.*)", "allow", ""},
		{"WebFetch(https://example.com)", "allow", ""},
		{"mcp__datadog__query", "allow", "Mcp(datadog:query)"},
		{"mcp__datadog__*", "allow", "Mcp(datadog:*)"},
		{"mcp__datadog", "deny", "Mcp(datadog:*)"},
		{"mcp__github__get_*", "allow", "Mcp(github:get_*)"},
		{"mcp__*", "deny", "Mcp(*:*)"},
		{"mcp__*", "allow", ""},
		{"mcp__*__search", "deny", "Mcp(*:search)"},
		{"mcp__*__search", "allow", ""},
		{"mcp__gh*__x", "deny", ""},
		{"WebSearch", "deny", ""},
		{"Grep(src/**)", "allow", ""},
	}
	for _, c := range cases {
		got := strings.Join(cliPermissionRules(c.rule, c.list), ", ")
		if got != c.want {
			t.Errorf("cliPermissionRules(%q, %s) = %q; want %q", c.rule, c.list, got, c.want)
		}
	}
}

// Dropping a deny rule loosens the policy, so each one is named.
func TestEmit_UntranslatedDenyRulesAreNamedInTheirOwnNote(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	emitSettings(t, settingsSpec(map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(go test:*)"},
			"deny":  []any{"Bash(git push:*)", "Bash(rm:*)", "Read(~/.ssh/**)"},
		},
	}))
	emit.FlushCoverageNotes()

	out := buf.String()
	if !strings.Contains(out, "deny rule not enforced on cursor: Bash(git push:*), Read(~/.ssh/**)") {
		t.Errorf("want a deny note naming each dropped rule, got:\n%s", out)
	}
	if strings.Contains(out, "Bash(go test:*)") {
		t.Errorf("an allow rule leaked into the deny note:\n%s", out)
	}
	if !strings.Contains(out, "`permissions`") {
		t.Errorf("want the allow note too, got:\n%s", out)
	}
}

func TestEmit_PortablePermissionsReachCLIConfig(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	emitSettings(t, settingsSpec(map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git:*)", "Read(src/**)"},
			"deny":  []any{"Bash(rm:*)", "Read(.env*)"},
		},
		"protected": map[string]any{"paths": []any{"composer.lock"}, "decision": "deny"},
	}))
	emit.FlushCoverageNotes()

	if got, want := cliRules(t, dir, "allow"), []string{"Shell(git)", "Read(src/**)"}; !slices.Equal(got, want) {
		t.Errorf("allow = %v, want %v", got, want)
	}
	wantDeny := []string{"Shell(rm)", "Read(.env*)", "Write(composer.lock)", "Write(composer.lock/**)"}
	if got := cliRules(t, dir, "deny"); !slices.Equal(got, wantDeny) {
		t.Errorf("deny = %v, want %v", got, wantDeny)
	}
	if strings.Contains(buf.String(), "permissions") {
		t.Errorf("want no permissions note when every rule translates, got:\n%s", buf.String())
	}
}

func TestEmit_UntranslatedAndAskPermissionsRaiseNotes(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	emitSettings(t, settingsSpec(map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(go test:*)", "Bash(git:*)"},
			"ask":   []any{"Bash(git push:*)"},
		},
	}))
	emit.FlushCoverageNotes()

	if got, want := cliRules(t, dir, "allow"), []string{"Shell(git)"}; !slices.Equal(got, want) {
		t.Errorf("allow = %v, want %v", got, want)
	}
	out := buf.String()
	if !strings.Contains(out, "`permissions`") || !strings.Contains(out, "no faithful Cursor CLI spelling") {
		t.Errorf("want an untranslated-rule note, got:\n%s", out)
	}
	if !strings.Contains(out, "`permissions.ask`") {
		t.Errorf("want an ask note, got:\n%s", out)
	}
}

func TestEmit_RemovingAPortableRuleRemovesItFromCLIConfigAndKeepsHandWrittenOnes(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeCLIConfig(t, dir, `{"permissions": {"allow": ["Shell(ls)"], "deny": ["Shell(curl)"]}}`)
	emitSettings(t, settingsSpec(map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(git:*)", "Bash(ls:*)"}, "deny": []any{"Bash(rm:*)"}},
	}))

	emitSettings(t, settingsSpec(map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(git:*)"}},
	}))
	if got, want := cliRules(t, dir, "allow"), []string{"Shell(ls)", "Shell(git)"}; !slices.Equal(got, want) {
		t.Errorf("after narrowing, allow = %v, want %v", got, want)
	}
	if got, want := cliRules(t, dir, "deny"), []string{"Shell(curl)"}; !slices.Equal(got, want) {
		t.Errorf("after narrowing, deny = %v, want %v", got, want)
	}

	emitSettings(t)
	if got, want := cliRules(t, dir, "allow"), []string{"Shell(ls)"}; !slices.Equal(got, want) {
		t.Errorf("after removal, allow = %v, want %v", got, want)
	}
}

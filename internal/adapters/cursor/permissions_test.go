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
		{"Bash(rm)", "deny", "Shell(rm)"},
		{"Bash(ls)", "allow", ""},
		{"Bash(go test:*)", "allow", ""},
		{"Bash(go test:*)", "deny", ""},
		{"Bash(rm -rf /)", "deny", ""},
		{"Bash(ls*)", "allow", ""},
		{"Bash", "deny", ""},
		{"Read(src/**/*.ts)", "allow", "Read(src/**/*.ts)"},
		{"Read(./.env*)", "deny", "Read(.env*)"},
		{"Read(/docs/**)", "allow", "Read(docs/**)"},
		{"Read(//etc/passwd)", "deny", "Read(/etc/passwd)"},
		{"Read(~/.ssh/**)", "deny", ""},
		{"Read(src/{a,b}.ts)", "allow", ""},
		{"Read(!src/**)", "deny", ""},
		{"Edit(src/**)", "allow", "Write(src/**)"},
		{"Write(**/*.key)", "deny", "Write(**/*.key)"},
		{"WebFetch(domain:docs.github.com)", "allow", "WebFetch(docs.github.com)"},
		{"WebFetch(domain:*.github.com)", "allow", "WebFetch(*.github.com)"},
		{"WebFetch(domain:*)", "deny", "WebFetch(*)"},
		{"WebFetch(domain:example.*)", "allow", ""},
		{"WebFetch(https://example.com)", "allow", ""},
		{"mcp__datadog__query", "allow", "Mcp(datadog:query)"},
		{"mcp__datadog__*", "allow", "Mcp(datadog:*)"},
		{"mcp__datadog", "deny", "Mcp(datadog:*)"},
		{"mcp__github__get_*", "allow", "Mcp(github:get_*)"},
		{"mcp__*", "deny", "Mcp(*:*)"},
		{"mcp__*", "allow", ""},
		{"mcp__gh*__x", "deny", ""},
		{"WebSearch", "deny", ""},
		{"Grep(src/**)", "allow", ""},
	}
	for _, c := range cases {
		got, ok := cliPermissionRule(c.rule, c.list)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("cliPermissionRule(%q, %s) = %q, %v; want %q", c.rule, c.list, got, ok, c.want)
		}
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

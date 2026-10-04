package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// syncedSettingsTree syncs one settings spec to every target and returns
// the files sync wrote, without the sources.
func syncedSettingsTree(t *testing.T, permissions string) map[string]string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(adapters.Names(), ", ")+"]\noutputs:\n  codex:\n    exec-policies-from-permissions: true\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n"+permissions)
	mustSync(t)
	tree := snapshotTree(t, dir)
	for rel := range tree {
		if strings.HasPrefix(rel, ".agnostic-ai"+string(filepath.Separator)) {
			delete(tree, rel)
		}
	}
	return tree
}

// Permission capabilities sync to every target exactly as the Claude
// Code rules they stand for.
func TestSync_PermissionCapabilitiesWriteTheSameFilesAsTheClaudeRules(t *testing.T) {
	neutral := syncedSettingsTree(t, "  allow: [read, shell(go test *), edit(src/**), web, mcp:github]\n  ask: [shell(git push:*)]\n  deny: [shell(git push --force*), edit(.env), write]\n")
	claude := syncedSettingsTree(t, "  allow: [Read, Bash(go test *), Edit(src/**), WebFetch, WebSearch, mcp__github]\n  ask: [Bash(git push:*)]\n  deny: [Bash(git push --force*), Edit(.env), Write]\n")
	if len(neutral) == 0 {
		t.Fatal("sync wrote nothing")
	}
	for rel, want := range claude {
		if got, ok := neutral[rel]; !ok || got != want {
			t.Errorf("%s differs:\n%s\nwant:\n%s", rel, got, want)
		}
	}
	for rel := range neutral {
		if _, ok := claude[rel]; !ok {
			t.Errorf("capabilities wrote %s, which the Claude rules do not", rel)
		}
	}
}

func TestSync_StopsOnAPermissionCapabilityItCannotRead(t *testing.T) {
	cases := []struct{ rule, want string }{
		{"raed", `unknown capability "raed" for permissions.deny: (did you mean read?)`},
		{"write(.env)", `permissions.deny: "write(.env)": Claude Code checks file writes against Edit rules only`},
		{"read()", `permissions.deny: "read()" needs a path pattern`},
		{"web(go.dev)", `permissions.deny: "web(go.dev)": only shell, read, and edit take a pattern`},
		{"shell(curl -H Authorization: x)", "permissions.deny: entry map[shell(curl -H Authorization:x)] is not a rule"},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
			rule := "'" + tc.rule + "'"
			if strings.Contains(tc.rule, ": ") {
				rule = tc.rule
			}
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "p.yaml"), "permissions:\n  deny:\n    - "+rule+"\n")
			if out, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("sync: %v\n%s", err, out)
			}
			if exists(filepath.Join(dir, ".claude", "settings.json")) {
				t.Error("sync must not write settings it cannot read")
			}
			if out, err := runCLI(t, "lint"); err == nil || !strings.Contains(out, "LINT036 [error] .agnostic-ai/settings/p.yaml: ") {
				t.Errorf("lint: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, tc.want) {
				t.Errorf("validate: %v\n%s", err, out)
			}
		})
	}
}

func TestLint_MidWildcardReadsAShellCapability(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "p.yaml"), "permissions:\n  allow: ['shell(git * main)']\n")
	if out, _ := runCLI(t, "lint"); !strings.Contains(out, "LINT009") {
		t.Errorf("lint misses LINT009:\n%s", out)
	}
}

func TestMigrate_CapabilitiesRewritesPermissionRulesInPlace(t *testing.T) {
	dir := migrationFixture(t, "capabilities-settings-permissions")
	silence(t)

	out, err := runCLI(t, "migrate", "--only", "capabilities")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for _, want := range []string{
		"rewrote .agnostic-ai/settings/permissions.yaml",
		"skipped .agnostic-ai/settings/permissions.yaml: keeps WebFetch(domain:go.dev) as an alias: no capability stands for it alone",
		"skipped .agnostic-ai/settings/permissions.yaml: keeps Write(.env) as an alias: Claude Code never consults a Write(path) rule",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	got, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Keep this comment: migrations must not reformat settings.\n" +
		"permissions:\n" +
		"  allow:\n" +
		"    - read                 # look anywhere\n" +
		"    - 'shell(go test:*)'\n" +
		"    - edit(src/**)\n" +
		"    - WebFetch(domain:go.dev)\n" +
		"  ask: [shell(git push:*), mcp:github/create_issue]\n" +
		"  deny:\n" +
		"    - \"shell(rm:*)\"\n" +
		"    - Write(.env)\n" +
		"model: sonnet\n"
	if string(got) != want {
		t.Errorf("permissions.yaml =\n%s\nwant:\n%s", got, want)
	}
}

// A pack's permission rule a target would drop stops sync too, since the
// lost rule may be a deny.
func TestSync_StopsOnAPackPermissionItCannotRead(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "packs", "p", "settings", "p.yaml"), "permissions:\n  allow: [edit]\n  deny: ['edit(.env']\n")
	mustWrite(t, filepath.Join(dir, "agnostic.packs.lock"), "version: 1\npacks:\n  - name: p\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [opencode]\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "is missing its closing parenthesis") {
		t.Errorf("sync: %v\n%s", err, out)
	}
}

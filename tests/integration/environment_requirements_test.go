package integration

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEnvironmentRequirements_SyncsDomainsToCursorOnly(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, claude]\n")
	const env = ".agnostic-ai/environments/dev.yaml"
	const setup = "setup: make deps\n"
	run := func(args ...string) string {
		t.Helper()
		out, err := ellipsisCommand(t, binary, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	writeProjectFile(t, dir, env, setup)
	run("sync")
	nativeClaude := readProjectFile(t, dir, ".claude/settings.json")
	writeProjectFile(t, dir, env, setup+`variables:
  - name: LOGS_API_TOKEN
    description: Read-only token for the log search API
    secret: true
  - name: APP_ENV
allowed-domains:
  - logs.example.com
  - '*.packages.example.com'
`)
	run("validate")
	run("lint")
	if out := run("sync"); strings.Contains(out, "has no effect") {
		t.Errorf("sync reported a no-effect note for checklist fields:\n%s", out)
	}
	raw := readProjectFile(t, dir, ".cursor/environment.json")
	var got struct {
		Domains []string `json:"egressAllowlist"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode Cursor environment: %v", err)
	}
	if !slices.Equal(got.Domains, []string{"logs.example.com", "*.packages.example.com"}) {
		t.Errorf("Cursor domains = %q", got.Domains)
	}
	for _, name := range []string{"variables", "allowed-domains", "LOGS_API_TOKEN", "APP_ENV"} {
		if strings.Contains(raw, name) {
			t.Errorf("Cursor environment retains %s", name)
		}
	}
	if after := readProjectFile(t, dir, ".claude/settings.json"); after != nativeClaude {
		t.Error("checklist fields changed Claude settings")
	}
	run("sync", "--check")
}

func TestEnvironmentRequirements_LintRejectsValuesWithoutPrintingThem(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", `variables:
  - name: WITH_VALUE
    value: fixture-value-1
  - TOKEN=fixture-value-2
  - name: REPEAT
  - name: REPEAT
  - name: 1INVALID
x-cursor:
  allowed-domains:
    - https://user:fixture-value-3@logs.example.com
`)
	out, err := ellipsisCommand(t, binary, dir, "lint")
	if err == nil {
		t.Errorf("lint accepted malformed requirements:\n%s", out)
	}
	if count := strings.Count(out, "LINT066 [error]"); count != 5 {
		t.Errorf("got %d LINT066 errors, want 5:\n%s", count, out)
	}
	if strings.Contains(out, "fixture-value-") {
		t.Error("lint disclosed credential text")
	}
	out, err = ellipsisCommand(t, binary, dir, "sync", "--dry-run")
	if err == nil {
		t.Error("sync preview accepted a credential URL as a domain")
	}
	if strings.Contains(out, "fixture-value-") {
		t.Error("sync preview disclosed credential text")
	}
}

func TestEnvironmentRequirements_ImportsNativeAllowlist(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeProjectFile(t, dir, ".cursor/environment.json", `{"egressAllowlist":["native.example.com"]}`)
	run := func(args ...string) {
		t.Helper()
		if out, err := ellipsisCommand(t, binary, dir, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run("import", "cursor")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "allowed-domains: [portable.example.com]\n")
	run("lint")
	run("sync")
	raw := readProjectFile(t, dir, ".cursor/environment.json")
	if !strings.Contains(raw, "native.example.com") || strings.Contains(raw, "portable.example.com") {
		t.Errorf("Cursor environment lost native allowlist precedence:\n%s", raw)
	}
	run("sync", "--check")
}

func TestEnvironmentRequirements_LintChecksSharedInputUnderALocalOverride(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	const shared = ".agnostic-ai/environments/dev.yaml"
	writeProjectFile(t, dir, shared, "variables: [TOKEN=fixture-value]\n")
	writeProjectFile(t, dir, ".agnostic-ai/local/environments/dev.yaml", "variables: [{name: TOKEN}]\n")
	out, err := ellipsisCommand(t, binary, dir, "lint", "--files", shared)
	if err == nil {
		t.Errorf("lint ignored the invalid source under a local override:\n%s", out)
	}
	if !strings.Contains(out, "LINT066") || !strings.Contains(filepath.ToSlash(out), shared) || strings.Contains(out, "fixture-value") {
		t.Errorf("unexpected lint output:\n%s", out)
	}
}

func TestEnvironmentRequirements_ImportKeepsPortableDomainsInChargeOnAFreshClone(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	const environment = ".agnostic-ai/environments/dev.yaml"
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeProjectFile(t, dir, environment, "allowed-domains: [old.example.com]\n")
	writeProjectFile(t, dir, ".cursor/environment.json", `{"egressAllowlist":["old.example.com"],"start":"npm start"}`)
	if out, err := ellipsisCommand(t, binary, dir, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	writeProjectFile(t, dir, environment, "allowed-domains: [new.example.com]\n")
	if out, err := ellipsisCommand(t, binary, dir, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := readProjectFile(t, dir, ".cursor/environment.json")
	if !strings.Contains(native, "new.example.com") || strings.Contains(native, "old.example.com") || !strings.Contains(native, "npm start") {
		t.Errorf("the imported file lost native keys or hid the portable update:\n%s", native)
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupComparePermissionFixture(t *testing.T) string {
	t.Helper()
	dir := setupCompareFixture(t)
	files := map[string]string{
		"agnostic-ai.yaml": `targets: [claude, codex]
outputs:
  codex:
    exec-policies-from-permissions: true
`,
		".agnostic-ai/settings/policy.yaml": `name: policy
permissions:
  allow:
    - shell(go test:*)
    - read(src/**)
  ask:
    - shell(git push:*)
  deny:
    - shell(rm:*)
    - delete
  default-mode: plan
`,
	}
	for relative, body := range files {
		path := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCompare_PortablePermissionEntriesAppear(t *testing.T) {
	dir := setupComparePermissionFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	out := compareJSON(t, "claude", "codex")
	var fields []string
	for _, entry := range out.Specs {
		if entry.Kind == "settings" && entry.Path == ".agnostic-ai/settings/policy.yaml" {
			for _, field := range entry.Fields {
				fields = append(fields, field.Field)
			}
		}
	}
	want := []string{"permissions.allow[0]", "permissions.allow[1]", "permissions.ask[0]", "permissions.deny[0]", "permissions.deny[1]", "permissions.default-mode"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("permission source rows = %v, want %v", fields, want)
	}
	path := ".agnostic-ai/settings/policy.yaml"
	read := findCompareResult(t, out, path, "permissions.allow[1]", "claude")
	if read.Status != statusTranslated || len(read.Paths) == 0 || !strings.Contains(read.Reason, "read(src/**)") {
		t.Errorf("Claude scoped read = %+v, want translated native output naming the source rule", read)
	}
	unsupported := findCompareResult(t, out, path, "permissions.allow[1]", "codex")
	if unsupported.Status != statusUnsupported || !strings.Contains(unsupported.Reason, "read(src/**)") {
		t.Errorf("Codex scoped read = %+v, want an independently unsupported source rule", unsupported)
	}
	shell := findCompareResult(t, out, path, "permissions.allow[0]", "codex")
	if shell.Status != statusTranslated || len(shell.Paths) == 0 || !strings.Contains(shell.Reason, "shell(go test:*)") {
		t.Errorf("Codex wildcard shell = %+v, want translated native command policy", shell)
	}
}

func TestCompare_PermissionPolicyUsesOtherSettingsSources(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	allow := spec.Entry{Kind: spec.KindSettings, Name: "allow", Path: "settings/allow.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell(git push)"}}}}
	deny := spec.Entry{Kind: spec.KindSettings, Name: "deny", Path: "settings/deny.yaml", Meta: map[string]any{"permissions": map[string]any{"deny": []any{"shell(git *)"}}}}
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Outputs: map[string]config.Output{"codex": {ExecPoliciesFromPermissions: true}}}
	settings := []spec.Entry{allow, deny}
	before, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	out, err := compareTargets(cfg, spec.Bundle{Settings: settings}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	got := findCompareResult(t, out, allow.Path, "permissions.allow[0]", "codex")
	if got.Status != statusTranslated || len(got.Paths) == 0 {
		t.Errorf("contextual exact allow = %+v, want evidence from the complete policy", got)
	}
	after, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("comparison mutated settings: before %s, after %s", before, after)
	}
}

func TestCompare_PermissionNativeOverrideShowsDifferentDecisions(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{
		"permissions": map[string]any{"allow": []any{"read"}},
		"x-opencode":  map[string]any{"permission": map[string]any{"read": "deny"}},
	}}
	cfg := &config.Config{Targets: []string{"opencode", "kilo"}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	deny := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "opencode")
	allow := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "kilo")
	if deny.Status != statusTranslated || allow.Status != statusTranslated || !strings.Contains(deny.Reason, "deny") || !strings.Contains(deny.Reason, "x-opencode.permission") {
		t.Errorf("effective decisions must remain visible: opencode=%+v, kilo=%+v", deny, allow)
	}
	if out.Differences != 1 || len(out.Specs) != 1 || !out.Specs[0].Fields[0].Differs {
		t.Errorf("opposite native decisions must differ even when both are translated: %+v", out)
	}
}

func TestCompare_PermissionsKeepDuplicateSourceRowsAndPartialLists(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"read(src/**)", "read(src/**)", "WebFetch(domain:go.dev)", "web"}}}}
	cfg := &config.Config{Targets: []string{"claude", "opencode"}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"permissions.allow[0]", "permissions.allow[1]", "permissions.allow[3]"} {
		got := findCompareResult(t, out, entry.Path, field, "opencode")
		if got.Status != statusTranslated || len(got.Paths) == 0 {
			t.Errorf("supported duplicate or expanded permission %s = %+v", field, got)
		}
	}
	got := findCompareResult(t, out, entry.Path, "permissions.allow[2]", "opencode")
	if got.Status != statusUnsupported || !strings.Contains(got.Reason, "WebFetch(domain:go.dev)") {
		t.Errorf("unsupported sibling hidden by supported read = %+v", got)
	}
	reversed, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, []string{"opencode", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Specs) != 1 || len(reversed.Specs) != 1 || len(out.Specs[0].Fields) != 4 || len(reversed.Specs[0].Fields) != 4 {
		t.Fatalf("source rows missing: forward=%+v reverse=%+v", out, reversed)
	}
	for i, field := range out.Specs[0].Fields {
		if field.Field != reversed.Specs[0].Fields[i].Field {
			t.Errorf("selected target order changed source identity: %s versus %s", field.Field, reversed.Specs[0].Fields[i].Field)
		}
	}
}

func TestCompare_PermissionsRespectCodexOptInAndAuthoritativeEmptyPolicy(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell(go test:*)"}}}}
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Outputs: map[string]config.Output{"codex": {}}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	got := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "codex")
	if got.Status != statusUnsupported || !strings.Contains(got.Next, "exec-policies-from-permissions") {
		t.Errorf("disabled output prerequisite = %+v", got)
	}
	cfg.Outputs["codex"] = config.Output{ExecPoliciesFromPermissions: true, ExecPolicies: []config.CodexExecPolicy{}}
	out, err = compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	got = findCompareResult(t, out, entry.Path, "permissions.allow[0]", "codex")
	if got.Status != statusUnknown || len(got.Paths) != 0 || !strings.Contains(got.Reason, "outputs.codex.exec-policies") {
		t.Errorf("authoritative empty policy must not claim portable output = %+v", got)
	}
}

func TestCompare_PermissionDefaultModeRemainsGlobalOnly(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "mode", Path: "settings/mode.yaml", Meta: map[string]any{"permissions": map[string]any{"default-mode": "plan"}, "x-claude": map[string]any{"permissions": map[string]any{"defaultMode": "plan"}}}}
	cfg := &config.Config{Targets: []string{"claude", "codex"}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range cfg.Targets {
		got := findCompareResult(t, out, entry.Path, "permissions.default-mode", target)
		if got.Status != statusUnsupported || len(got.Paths) != 0 || !strings.Contains(got.Reason, "global sync") {
			t.Errorf("%s raw or native mode must not prove portable project mapping: %+v", target, got)
		}
	}
}

func TestCompare_PermissionFiltersAndOutputPathsKeepUnrelatedSettingsOut(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Read"}}, "model": "sonnet"}}
	excluded := spec.Entry{Kind: spec.KindSettings, Name: "excluded", Path: "settings/excluded.yaml", Meta: map[string]any{"targets-exclude": []any{"codex"}, "permissions": map[string]any{"deny": []any{"shell(git *)"}}}}
	cfg := &config.Config{Targets: []string{"claude", "codex"}, Outputs: map[string]config.Output{"claude": {Dir: "custom instructions"}, "codex": {ExecPoliciesFromPermissions: true}}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry, excluded}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	alias := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "claude")
	if alias.Status != statusPreserved || !reflect.DeepEqual(alias.Paths, []string{"custom instructions/settings.json"}) {
		t.Errorf("native alias must retain its configured settings path: %+v", alias)
	}
	unsupported := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "codex")
	if unsupported.Status != statusUnsupported || len(unsupported.Paths) != 0 {
		t.Errorf("unrelated model output must not prove read support: %+v", unsupported)
	}
	filtered := findCompareResult(t, out, excluded.Path, "permissions.deny[0]", "codex")
	if filtered.Status != statusExcluded || len(filtered.Paths) != 0 {
		t.Errorf("target-filtered permission must stay excluded: %+v", filtered)
	}
	for _, entry := range out.Specs {
		for _, field := range entry.Fields {
			if field.Field == "model" {
				t.Error("portable permission comparison expanded into model settings")
			}
		}
	}
}

func TestCompare_PermissionPartialLossDiffersFromCompleteTranslation(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{
		"permissions": map[string]any{"allow": []any{"web"}},
	}}
	cfg := &config.Config{Targets: []string{"claude", "windsurf"}}
	out, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	complete := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "claude")
	partial := findCompareResult(t, out, entry.Path, "permissions.allow[0]", "windsurf")
	if complete.Status != statusTranslated || partial.Status != statusTranslated || !strings.Contains(partial.Reason, "part of this permission is unsupported") {
		t.Errorf("complete and partial mappings must retain their independent translated results: Claude=%+v Windsurf=%+v", complete, partial)
	}
	if len(out.Specs) != 1 || len(out.Specs[0].Fields) != 1 {
		t.Fatalf("expected one source permission: %+v", out)
	}
	if !out.Specs[0].Fields[0].Differs || out.Differences != 1 {
		t.Errorf("partial permission loss must differ from a complete mapping: %+v", out)
	}
	var text bytes.Buffer
	writeCompareReport(&text, out)
	if !strings.Contains(text.String(), "permissions.allow[0] (differs)") || !strings.Contains(text.String(), "1 of 1 fields differ") {
		t.Errorf("text must identify and count partial permission loss:\n%s", text.String())
	}
	completePair, err := compareTargets(cfg, spec.Bundle{Settings: []spec.Entry{entry}}, []string{"claude", "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if completePair.Differences != 0 || completePair.Specs[0].Fields[0].Differs {
		t.Errorf("complete translations with different native names must not differ: %+v", completePair)
	}
}

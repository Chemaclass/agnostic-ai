package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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

func TestCompare_PermissionWebhookCredentialsHiddenInTextAndJSON(t *testing.T) {
	for _, test := range []struct {
		name, policy, secret string
	}{
		{"URL token", "webhookUrl: https://example.invalid/check?token=COMPARE_WEBHOOK_SECRET", "COMPARE_WEBHOOK_SECRET"},
		{"authorization header", "webhookUrl: https://example.invalid/check\n        headers:\n          Authorization: Bearer COMPARE_HEADER_SECRET", "COMPARE_HEADER_SECRET"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "targets: [augment, claude]\n")
			mustWriteFile(t, ".agnostic-ai/settings/policy.yaml", "name: policy\npermissions:\n  allow: [shell]\nx-augment:\n  toolPermissions:\n    - toolName: terminal\n      permission:\n        type: webhook-policy\n        "+test.policy+"\n")
			mustWriteFile(t, ".augment/settings.json", "{\"manual\":\"Keep existing output.\"}\n")
			before := snapshotTree(t, dir)
			for _, format := range [][]string{nil, {"--json"}} {
				args := append([]string{"augment", "claude"}, format...)
				out, err := runCompare(t, args...)
				if err != nil {
					t.Fatalf("compare native webhook policy: %v", err)
				}
				if strings.Contains(out, test.secret) {
					t.Error("comparison exposed the synthetic native policy credential")
				}
				for _, visible := range []string{"permissions.allow[0]", "terminal", "webhook-policy", "x-augment.toolPermissions"} {
					if !strings.Contains(out, visible) {
						t.Errorf("comparison lost useful policy detail %q", visible)
					}
				}
			}
			out := compareJSON(t, "augment", "claude")
			result := findCompareResult(t, out, ".agnostic-ai/settings/policy.yaml", "permissions.allow[0]", "augment")
			if result.Status != statusTranslated || len(result.Paths) == 0 {
				t.Errorf("raw webhook override no longer matches native output: status %s, paths %v", result.Status, result.Paths)
			}
			if out.Differences == 0 {
				t.Error("webhook policy and ordinary allow became indistinguishable")
			}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			captured, err := capturePermissions(cfg, bundle.Settings, "augment")
			if err != nil {
				t.Fatal(err)
			}
			translated := adapters.TranslatePermissionCapabilityIn("augment", "allow", "shell", bundle.Settings[0], bundle.Settings, cfg)
			if len(translated.Native) != 1 || !strings.Contains(translated.Native[0], test.secret) {
				t.Fatal("native translation lost its original credential before display")
			}
			paths, _, meaning := permissionNativePaths(captured.files, "allow", "shell", translated)
			if len(paths) == 0 || meaning != "webhook-policy" {
				t.Error("raw native webhook evidence or existing decision meaning changed")
			}
			mismatched := translated
			mismatched.Native = []string{strings.ReplaceAll(translated.Native[0], test.secret, test.secret+"-different")}
			paths, _, _ = permissionNativePaths(captured.files, "allow", "shell", mismatched)
			if len(paths) != 0 {
				t.Error("different raw native credentials incorrectly matched the emitted policy")
			}
			if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
				t.Error("permission comparison changed source or emitted configuration")
			}
		})
	}
}

func TestCompare_PermissionOrdinaryWebhookDetailsRemainVisible(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "targets: [augment, claude]\n")
	mustWriteFile(t, ".agnostic-ai/settings/policy.yaml", "name: policy\npermissions:\n  allow: [shell]\nx-augment:\n  toolPermissions:\n    - toolName: terminal\n      permission:\n        type: webhook-policy\n        webhookUrl: https://example.invalid/check?mode=review\n")
	for _, format := range [][]string{nil, {"--json"}} {
		out, err := runCompare(t, append([]string{"augment", "claude"}, format...)...)
		if err != nil {
			t.Fatal(err)
		}
		for _, visible := range []string{"https://example.invalid/check?mode=review", "Bash", "permissions.allow[0]", "webhook-policy"} {
			if !strings.Contains(out, visible) {
				t.Errorf("ordinary permission detail %q was hidden", visible)
			}
		}
	}
}

func TestCompare_PermissionFilenamePatternsKeepTheirOrdinaryActions(t *testing.T) {
	for _, target := range []string{"opencode", "kilo"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "targets: ["+target+", claude]\n")
			mustWriteFile(t, ".agnostic-ai/settings/policy.yaml", "permissions:\n  allow: [read]\nx-"+target+":\n  permission:\n    read:\n      TOKEN: deny\n")
			before := snapshotTree(t, dir)
			for _, format := range [][]string{nil, {"--json"}} {
				out, err := runCompare(t, append([]string{target, "claude"}, format...)...)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out, "TOKEN") || !strings.Contains(out, "deny") || strings.Contains(out, "<redacted>") || strings.Contains(out, `\u003credacted\u003e`) {
					t.Error("ordinary filename-pattern permission action was hidden")
				}
			}
			out := compareJSON(t, target, "claude")
			result := findCompareResult(t, out, ".agnostic-ai/settings/policy.yaml", "permissions.allow[0]", target)
			if !strings.Contains(result.Reason, `"TOKEN":"deny"`) {
				t.Error("native filename-pattern decision is absent from the permission summary")
			}
			if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
				t.Error("comparison changed filename-pattern policy sources")
			}
		})
	}
}

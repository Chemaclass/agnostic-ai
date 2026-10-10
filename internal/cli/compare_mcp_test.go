package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func setupCompareMCPFixture(t *testing.T) string {
	t.Helper()
	dir := setupCompareFixture(t)
	files := map[string]string{
		"local": `name: local
command: mcp-test-command
args: ["--root", "${TEST_MCP_ROOT}"]
env:
  ROOT: !literal /tmp
  TOKEN: ${TEST_MCP_TOKEN}
`,
		"remote": `name: remote
type: http
url: https://mcp.example.com/mcp
headers:
  Authorization: Bearer ${TEST_MCP_TOKEN}
`,
	}
	for name, body := range files {
		path := filepath.Join(dir, ".agnostic-ai", "mcps", name+".yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCompare_MCPServersAppear(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")
	findCompareResult(t, out, ".agnostic-ai/mcps/local.yaml", "command", "claude")
	findCompareResult(t, out, ".agnostic-ai/mcps/remote.yaml", "url", "cursor")
}

func TestCompare_MCPConnectionFieldsUseNativeOutput(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "cursor")
	checks := []struct {
		name, field, target string
		status              compareStatus
	}{
		{"local", "command", "claude", statusPreserved},
		{"local", "args", "claude", statusPreserved},
		{"local", "args", "cursor", statusTranslated},
		{"local", "env.ROOT", "cursor", statusPreserved},
		{"local", "env.TOKEN", "claude", statusPreserved},
		{"local", "env.TOKEN", "cursor", statusTranslated},
		{"remote", "type", "claude", statusPreserved},
		{"remote", "url", "cursor", statusPreserved},
		{"remote", "headers.Authorization", "claude", statusPreserved},
		{"remote", "headers.Authorization", "cursor", statusTranslated},
	}
	for _, check := range checks {
		r := findCompareResult(t, out, ".agnostic-ai/mcps/"+check.name+".yaml", check.field, check.target)
		if r.Status != check.status || len(r.Paths) == 0 {
			t.Errorf("%s/%s/%s = %+v, want %s with output paths", check.name, check.field, check.target, r, check.status)
		}
	}
}

func writeCompareMCP(t *testing.T, name, body string) {
	t.Helper()
	path := filepath.Join(".agnostic-ai", "mcps", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompare_MCPPartialReferencesAndWholeServerOmission(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "partial", `name: partial
type: http
url: https://mcp.example.com/mcp
headers:
  Authorization: Bearer ${TEST_MCP_TOKEN:-DO_NOT_REPORT_FALLBACK}
  X-Team: !literal DO_NOT_REPORT_LITERAL
`)
	out := compareJSON(t, "claude", "cursor")
	missing := findCompareResult(t, out, ".agnostic-ai/mcps/partial.yaml", "headers.Authorization", "cursor")
	kept := findCompareResult(t, out, ".agnostic-ai/mcps/partial.yaml", "headers.X-Team", "cursor")
	if missing.Status != statusUnsupported || missing.Reason == "" || len(missing.Paths) != 0 {
		t.Errorf("omitted header = %+v", missing)
	}
	if kept.Status != statusPreserved || len(kept.Paths) == 0 {
		t.Errorf("retained literal header = %+v", kept)
	}
	out = compareJSON(t, "claude", "codex")
	omitted := findCompareResult(t, out, ".agnostic-ai/mcps/local.yaml", "args", "codex")
	if omitted.Status != statusUnsupported || !strings.Contains(omitted.Reason, "server") || len(omitted.Paths) != 0 {
		t.Errorf("unwritable args must explain whole server omission: %+v", omitted)
	}
}

func TestCompare_MCPFiltersUnsupportedAndOutputOptions(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "excluded", "name: excluded\ncommand: mcp-test-command\ntarget-exclude: cursor\n")
	out := compareJSON(t, "claude", "cursor")
	if r := findCompareResult(t, out, ".agnostic-ai/mcps/excluded.yaml", "command", "cursor"); r.Status != statusExcluded {
		t.Errorf("target filter = %+v", r)
	}
	out = compareJSON(t, "claude", "aider")
	if r := findCompareResult(t, out, ".agnostic-ai/mcps/remote.yaml", "url", "aider"); r.Status != statusUnsupported {
		t.Errorf("unsupported kind = %+v", r)
	}
	writeCompareMCP(t, "plain", "name: plain\ncommand: mcp-test-command\nargs: [--plain]\n")
	for _, optIn := range []bool{false, true} {
		config := "targets: [claude, copilot]\noutputs:\n  claude:\n    mcp-file: custom/mcp.json\n"
		if optIn {
			config += "  copilot:\n    root-mcp-file: extra/root.json\n"
		}
		if err := os.WriteFile("agnostic-ai.yaml", []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		out = compareJSON(t, "claude", "copilot")
		if r := findCompareResult(t, out, ".agnostic-ai/mcps/plain.yaml", "command", "claude"); !slices.Contains(r.Paths, "custom/mcp.json") {
			t.Errorf("custom output = %+v", r)
		}
		if r := findCompareResult(t, out, ".agnostic-ai/mcps/plain.yaml", "command", "copilot"); slices.Contains(r.Paths, "extra/root.json") != optIn {
			t.Errorf("root opt-in %v = %+v", optIn, r)
		}
	}
}

func TestCompare_MCPReportsNeverIncludeValues(t *testing.T) {
	dir := setupCompareMCPFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	secrets := []string{"PRIVATE_COMMAND", "PRIVATE_ARG", "PRIVATE_ENV", "PRIVATE_HEADER", "PRIVATE_URL", "PRIVATE_DEFAULT", "PRIVATE_UNKNOWN", "PRIVATE_OVERRIDE", "PRIVATE_ESCAPED"}
	writeCompareMCP(t, "secret-local", `name: secret-local
command: PRIVATE_COMMAND
args: [PRIVATE_ARG]
env:
  LITERAL: !literal PRIVATE_ENV
  DEFAULT: ${DEFAULT:-PRIVATE_DEFAULT}
  UNKNOWN: ${broken:PRIVATE_UNKNOWN}
  ESCAPED: $${PRIVATE_ESCAPED}
x-cursor:
  env:
    LITERAL: !literal PRIVATE_OVERRIDE
`)
	writeCompareMCP(t, "secret-remote", `name: secret-remote
type: http
url: https://PRIVATE_URL:password@mcp.example.com/mcp?token=PRIVATE_URL
headers:
  Literal: !literal PRIVATE_HEADER
  Unknown: ${broken:PRIVATE_UNKNOWN}
  Default: Bearer ${TOKEN:-PRIVATE_DEFAULT}
`)
	before := snapshotTree(t, dir)
	for _, targets := range [][]string{{"claude", "cursor"}, {"claude", "codex"}} {
		var previous string
		for _, format := range []string{"text", "json", "json"} {
			args := append([]string(nil), targets...)
			if format == "json" {
				args = append(args, "--json")
			}
			out, err := runCompare(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range secrets {
				if strings.Contains(out, secret) {
					t.Errorf("%s/%s report exposed %s", targets, format, secret)
				}
			}
			if format == "json" {
				if previous != "" && out != previous {
					t.Error("repeated JSON is unstable")
				}
				previous = out
			}
		}
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("comparison wrote project files")
	}
}

func TestCompare_MCPRequiredProjectionsRemainValid(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	for i := range 3 {
		name := fmt.Sprintf("probe-%d", i)
		writeCompareMCP(t, name, fmt.Sprintf("name: %s\ncommand: agnostic_ai_compare_mcp_command%s\n", name, strings.Repeat("_", i)))
	}
	out := compareJSON(t, "claude", "cursor")
	for i := range 3 {
		path := fmt.Sprintf(".agnostic-ai/mcps/probe-%d.yaml", i)
		if r := findCompareResult(t, out, path, "command", "claude"); r.Status != statusPreserved || r.Reason != "" {
			t.Errorf("valid required command = %+v", r)
		}
	}
}

func TestCompare_MCPEmissionErrorsDoNotExposeValues(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	entry := spec.Entry{Kind: spec.KindMCP, Name: "bad", Path: "mcps/bad.yaml", Meta: map[string]any{
		"command": "mcp-test-command",
		"x-warp":  map[string]any{"extra": privateMCPValue{}},
	}}
	_, _, err := classifyEntry(&config.Config{Targets: []string{"warp"}}, entry, []string{"command"}, "warp")
	if err == nil {
		t.Fatal("invalid MCP JSON value must fail analysis")
	}
	if strings.Contains(err.Error(), "PRIVATE_ERROR_VALUE") || !strings.Contains(err.Error(), entry.Path) {
		t.Errorf("MCP emission error must retain source without values: %v", err)
	}
}

func TestCompare_MCPNativeOverridesDoNotMutateSources(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "override", `name: override
command: portable-command
args: [--portable]
env:
  TOKEN: ${TOKEN}
x-continue:
  command: native-command
  args: ["${NATIVE_ARG}"]
  env:
    TOKEN: ${NATIVE_TOKEN}
`)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(bundle.MCPs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := compareTargets(cfg, bundle, []string{"claude", "continue"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"command", "args", "env.TOKEN"} {
		r := findCompareResult(t, out, ".agnostic-ai/mcps/override.yaml", field, "continue")
		if len(r.Paths) == 0 || r.Status == statusUnsupported || r.Status == statusUnknown {
			t.Errorf("native override %s = %+v", field, r)
		}
	}
	after, err := json.Marshal(bundle.MCPs)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("comparison changed source metadata")
	}
}

func TestCompare_MCPForwardingAndInactiveFields(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "forward", `name: forward
command: mcp-test-command
env:
  TOKEN: ${TOKEN}
  RENAMED: ${OTHER}
  PLAIN: !literal plain-setting
url: https://${INACTIVE_URL}/mcp
`)
	out := compareJSON(t, "claude", "codex")
	for _, check := range []struct {
		field  string
		status compareStatus
	}{
		{"env.TOKEN", statusTranslated},
		{"env.RENAMED", statusUnsupported},
		{"env.PLAIN", statusPreserved},
		{"url", statusUnsupported},
	} {
		r := findCompareResult(t, out, ".agnostic-ai/mcps/forward.yaml", check.field, "codex")
		if r.Status != check.status {
			t.Errorf("%s = %+v, want %s", check.field, r, check.status)
		}
	}
	writeCompareMCP(t, "url-probe", `name: url-probe
type: http
url: https://agnostic-ai-compare.invalid/mcp
`)
	out = compareJSON(t, "claude", "cursor")
	if r := findCompareResult(t, out, ".agnostic-ai/mcps/url-probe.yaml", "url", "claude"); r.Status != statusPreserved {
		t.Errorf("required URL probe collision = %+v", r)
	}
}

type privateMCPValue struct{}

func (privateMCPValue) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("PRIVATE_ERROR_VALUE")
}

func TestCompare_MCPEffectiveNativeOverrideOutlivesPortableOmissionNote(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "effective-override", `name: effective-override
command: mcp-test-command
env:
  TOKEN: ${TOKEN:-PRIVATE_PORTABLE_FALLBACK}
x-continue:
  env:
    TOKEN: !literal PRIVATE_EFFECTIVE_NATIVE
`)
	out := compareJSON(t, "claude", "continue")
	r := findCompareResult(t, out, ".agnostic-ai/mcps/effective-override.yaml", "env.TOKEN", "continue")
	if r.Status != statusPreserved || len(r.Paths) == 0 || r.Reason != "" {
		t.Errorf("usable effective native override must take priority over the omitted portable reference: %+v", r)
	}
	raw, err := runCompare(t, "claude", "continue", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"PRIVATE_PORTABLE_FALLBACK", "PRIVATE_EFFECTIVE_NATIVE"} {
		if strings.Contains(raw, value) {
			t.Errorf("override result exposed %s", value)
		}
	}
}

func TestCompare_MCPSelectedOverrideOnlyConnectionFieldsAppear(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "override-only", `name: override-only
command: mcp-test-command
x-continue:
  args: [--native]
  env:
    EXTRA: !literal PRIVATE_NATIVE_ONLY
x-cursor:
  headers:
    Unselected: !literal PRIVATE_UNSELECTED
x-claude:
  arbitrary-native-key: PRIVATE_UNSUPPORTED_KEY
`)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(bundle.MCPs)
	if err != nil {
		t.Fatal(err)
	}
	out := compareJSON(t, "claude", "continue")
	for _, field := range []string{"args", "env.EXTRA"} {
		r := findCompareResult(t, out, ".agnostic-ai/mcps/override-only.yaml", field, "continue")
		if r.Status != statusPreserved || len(r.Paths) == 0 {
			t.Errorf("override-only covered field %s must have native output evidence: %+v", field, r)
		}
	}
	for _, entry := range out.Specs {
		if entry.Name != "override-only" {
			continue
		}
		for _, field := range entry.Fields {
			if field.Field == "headers.Unselected" || field.Field == "arbitrary-native-key" {
				t.Errorf("unselected or uncovered field was reported: %s", field.Field)
			}
		}
	}
	first, err := runCompare(t, "claude", "continue", "--json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runCompare(t, "claude", "continue", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("override-only field order is unstable")
	}
	for _, secret := range []string{"PRIVATE_NATIVE_ONLY", "PRIVATE_UNSELECTED", "PRIVATE_UNSUPPORTED_KEY"} {
		if strings.Contains(first, secret) {
			t.Errorf("override-only result exposed %s", secret)
		}
	}
	if _, err := compareTargets(cfg, bundle, []string{"claude", "continue"}); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(bundle.MCPs)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("override-only projections changed the source")
	}
}

func TestCompare_MCPTransportDoesNotUseUnrelatedNativeKeys(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "no-explicit-type", `name: no-explicit-type
command: mcp-test-command
env:
  type: !literal stdio
`)
	if err := os.MkdirAll(".agnostic-ai/overlays", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"stdio", "unrelated-transport"} {
		body := fmt.Sprintf("[extra]\ntype = %q\n", value)
		if err := os.WriteFile(".agnostic-ai/overlays/codex.config.toml", []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		out := compareJSON(t, "claude", "codex")
		for _, target := range []string{"claude", "codex"} {
			r := findCompareResult(t, out, ".agnostic-ai/mcps/no-explicit-type.yaml", "type", target)
			if r.Status != statusUnknown || len(r.Paths) != 0 {
				t.Errorf("unrelated overlay or nested env type cannot establish %s transport: %+v", target, r)
			}
		}
	}
}

func TestCompare_MCPTransportIgnoresNestedObjectsNamedAfterServer(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	writeCompareMCP(t, "nested-name", `name: nested-name
command: mcp-test-command
x-warp:
  metadata:
    nested-name:
      type: stdio
`)
	out := compareJSON(t, "claude", "warp")
	r := findCompareResult(t, out, ".agnostic-ai/mcps/nested-name.yaml", "type", "warp")
	if r.Status != statusUnknown || len(r.Paths) != 0 {
		t.Errorf("nested metadata named after the server cannot establish its transport: %+v", r)
	}
}

func TestCompare_MCPContinueWrapperKeepsNativeConnectionEvidence(t *testing.T) {
	testutil.Chdir(t, setupCompareMCPFixture(t))
	silence(t)
	out := compareJSON(t, "claude", "continue")
	for _, check := range []struct {
		name, field string
		status      compareStatus
	}{
		{"local", "command", statusPreserved},
		{"local", "env.ROOT", statusPreserved},
		{"remote", "url", statusPreserved},
		{"remote", "type", statusTranslated},
	} {
		r := findCompareResult(t, out, ".agnostic-ai/mcps/"+check.name+".yaml", check.field, "continue")
		if r.Status != check.status || len(r.Paths) == 0 {
			t.Errorf("Continue wrapper must preserve connection evidence for %s/%s: %+v, want %s with paths", check.name, check.field, r, check.status)
		}
	}
}

package emit

import (
	"strings"
	"testing"
)

var testPluginHost = PluginHookHost{
	Target: "opencode", Tool: "OpenCode", ImportModule: "@opencode-ai/plugin", BusEvents: PluginBusEvents(),
}

// The export has to be a legal JavaScript identifier whatever the spec
// name looks like on disk.
func TestPluginIdentifier_AlwaysLegalJavaScript(t *testing.T) {
	cases := map[string]string{
		"sample-hook":  "SampleHookPlugin",
		"fmt_go":       "FmtGoPlugin",
		"2fa":          "Hook2faPlugin",
		"no.rm.rf":     "NoRmRfPlugin",
		"alreadyCamel": "AlreadyCamelPlugin",
	}
	for name, want := range cases {
		if got := pluginIdentifier(name); got != want {
			t.Errorf("pluginIdentifier(%q) = %q, want %q", name, got, want)
		}
	}
}

// Bun's `$` reads a template's raw strings, so escaping a backslash
// doubles it on the way to the shell. A `{ raw }` value reaches the
// shell exactly as authored.
func TestPluginModule_CommandReachesShellVerbatim(t *testing.T) {
	got := pluginModule(testPluginHost, "g", "tool.execute.after", "PostToolUse", "", []string{"grep -qE '\\.go$' f && echo `x` ${HOME}"})
	want := "await $`${{ raw: \"grep -qE '\\\\.go$' f && echo `x` ${HOME}\" }}`.nothrow()"
	if !strings.Contains(got, want) {
		t.Errorf("want %s in:\n%s", want, got)
	}
}

// A PreToolUse command that exits 2 blocks the tool, as in Claude; any
// other exit status is a warning, and later commands still run.
func TestPluginModule_BeforeHookBlocksOnlyOnExitTwo(t *testing.T) {
	got := pluginModule(testPluginHost, "guard", "tool.execute.before", "PreToolUse", "", []string{"a", "b"})
	for _, want := range []string{
		"const r1 = await $`${{ raw: \"a\" }}`.nothrow()",
		"if (r1.exitCode === 2) throw new Error(r1.stderr.toString() || \"blocked by hook guard\")",
		"const r2 = await $`${{ raw: \"b\" }}`.nothrow()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

// Claude matches a plain tool name exactly, so the guard is anchored,
// and it compiles once at module scope instead of on every tool call.
func TestPluginModule_MatcherIsAnchoredAtModuleScope(t *testing.T) {
	got := pluginModule(testPluginHost, "fmt", "tool.execute.after", "PostToolUse", "write|edit", []string{"x"})
	if !strings.Contains(got, "const MATCHER = new RegExp(\"^(?:write|edit)$\")\n") {
		t.Errorf("want an anchored module-scope matcher:\n%s", got)
	}
	if !strings.Contains(got, "if (!MATCHER.test(input.tool)) return") {
		t.Errorf("want the handler to reuse MATCHER:\n%s", got)
	}
}

func TestPluginModule_ExportShapePerHost(t *testing.T) {
	op := pluginModule(testPluginHost, "n", "", "session.idle", "", []string{"x"})
	if !strings.Contains(op, "export const NPlugin: Plugin") || strings.Contains(op, "export default") {
		t.Errorf("OpenCode exports the const:\n%s", op)
	}
	kilo := testPluginHost
	kilo.DefaultExport = true
	kilo.ImportModule = "@kilocode/plugin"
	k := pluginModule(kilo, "n", "", "session.idle", "", []string{"x"})
	if !strings.Contains(k, "\nconst NPlugin: Plugin") || !strings.Contains(k, `export default { id: "n", server: NPlugin }`) {
		t.Errorf("Kilo default-exports a descriptor:\n%s", k)
	}
}

// RE2 accepts syntax JavaScript's RegExp throws on or reads as literal
// letters. Those matchers are dropped instead of shipped.
func TestJSCompatibleMatcher(t *testing.T) {
	cases := map[string]bool{
		"bash":          true,
		"edit|write":    true,
		"mcp__.*":       true,
		`read\.x`:       true,
		"(?:a|b)":       true,
		"(?<tool>bash)": true,
		"(?i)bash":      false,
		"(?P<n>bash)":   false,
		`\Abash\z`:      false,
		`\pL+`:          false,
		"[[:alpha:]]":   false,
		"(":             false,
		`\\A`:           true,
	}
	for m, want := range cases {
		if got := jsCompatibleMatcher(m); got != want {
			t.Errorf("jsCompatibleMatcher(%q) = %v, want %v", m, got, want)
		}
	}
}

func TestHasClaudeToolName_CatchesAlternations(t *testing.T) {
	for m, want := range map[string]bool{"Bash": true, "Edit|Write": true, "(Read|grep)": true, "edit|write": false} {
		if got := hasClaudeToolName(m); got != want {
			t.Errorf("hasClaudeToolName(%q) = %v, want %v", m, got, want)
		}
	}
}

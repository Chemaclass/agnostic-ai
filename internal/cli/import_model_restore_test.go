package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestImportTrae_ReimportKeepsTheModelEntryWhenTheNativeLineIsGone(t *testing.T) {
	effortMapProject(t, "trae", "model:\n  trae: m-one\n  zed: low\n")
	mustWriteFile(t, ".trae/agents/rev.md", strings.Replace(readFile(t, ".trae/agents/rev.md"), "model: m-one\n", "", 1))

	got := reimportModelMap(t, "trae")
	if !hasBlock(got, "model:\n  trae: m-one\n  zed: low\n") {
		t.Errorf("Trae cannot show a shared model, so its entry stays:\n%s", got)
	}
	assertSyncCheckClean(t)
}

func TestImportClaude_ReimportReadsAClaudeAliasEntry(t *testing.T) {
	effortMapProject(t, "claude", "model:\n  claude: opus\n  zed: low\n  default: m-two\n")
	before := readFile(t, ".agnostic-ai/agents/rev.md")
	if got := reimportModelMap(t, "claude"); got != before {
		t.Errorf("an unchanged alias must not rewrite the map:\n%s", got)
	}

	editNativeModel(t, ".claude/agents/rev.md", "model", "sonnet")
	got := reimportModelMap(t, "claude")
	if !strings.Contains(got, "model:\n  claude: sonnet\n  zed: low\n  default: m-two\n") {
		t.Errorf("only claude's entry should change:\n%s", got)
	}
	assertSyncCheckClean(t)
}

func addNativeLine(t *testing.T, file, key, value string) {
	t.Helper()
	content := readFile(t, file)
	if strings.HasSuffix(file, ".toml") {
		mustWriteFile(t, file, fmt.Sprintf("%s = %q\n%s", key, value, content))
		return
	}
	lines := strings.Split(content, "\n")
	lines = append(lines[:1], append([]string{key + ": " + value}, lines[1:]...)...)
	mustWriteFile(t, file, strings.Join(lines, "\n"))
}

func TestImportAgents_ReimportOfARestoredModelDropsTheDeleteMarker(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  zed: low\n  default: m-two\nx-%s:\n  model: null\n", tc.target))
			addNativeLine(t, tc.file, tc.key, "m-new")

			got := reimportModelMap(t, tc.target)
			want := fmt.Sprintf("model:\n  zed: low\n  default: m-two\n  %s: m-new\n", tc.target)
			// Trae drops an unscoped model anyway, so the settle step that
			// keeps keys no tool shows puts its marker back; it does nothing.
			markerGone := tc.target == "trae" || !strings.Contains(got, "model: null")
			if !hasBlock(got, want) || !markerGone {
				t.Errorf("the native model joins the map and the marker goes:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportOfARestoredEffortDropsTheDeleteMarker(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("effort:\n  zed: low\n  default: high\nx-%s:\n  effort: null\n", tc.target))
			addNativeLine(t, tc.file, tc.key, "medium")

			got := reimportEffortMap(t, tc.target)
			want := fmt.Sprintf("effort:\n  zed: low\n  default: high\n  %s: medium\n", tc.target)
			if !strings.Contains(got, want) || strings.Contains(got, "effort: null") {
				t.Errorf("the native effort joins the map and the marker goes:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportCodex_ReimportKeepsAVendorAliasEntryAcrossAnUnrelatedEdit(t *testing.T) {
	effortMapProject(t, "codex", "model:\n  claude: opus\n  codex: sol\n")
	native := readFile(t, ".codex/agents/rev.toml")
	if !strings.Contains(native, "gpt-6.1-sol") {
		t.Fatalf("codex should write the alias's id:\n%s", native)
	}
	mustWriteFile(t, ".codex/agents/rev.toml", strings.Replace(native, `description = "r"`, `description = "r2"`, 1))

	got := reimportModelMap(t, "codex")
	if !hasBlock(got, "model:\n  claude: opus\n  codex: sol\n") {
		t.Errorf("the alias must stay so later upgrades apply:\n%s", got)
	}
	assertSyncCheckClean(t)
}

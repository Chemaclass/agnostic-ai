package cli

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// modelMapTargets are the importers that read a native key back into an
// agent's `model`, with the native file and key each one writes.
var modelMapTargets = []struct{ target, file, key string }{
	{"claude", ".claude/agents/rev.md", "model"},
	{"cursor", ".cursor/agents/rev.md", "model"},
	{"copilot", ".github/agents/rev.agent.md", "model"},
	{"kiro", ".kiro/agents/rev.md", "model"},
	{"qoder", ".qoder/agents/rev.md", "model"},
	{"goose", ".agents/agents/rev.md", "model"},
	{"openhands", ".agents/agents/rev.md", "model"},
	{"augment", ".augment/agents/rev.md", "model"},
	{"windsurf", ".devin/agents/rev.md", "model"},
	{"factory", ".factory/droids/rev.md", "model"},
	{"qwen", ".qwen/agents/rev.md", "model"},
	{"grok", ".grok/agents/rev.md", "model"},
	{"junie", ".junie/agents/rev.md", "model"},
	{"opencode", ".opencode/agents/rev.md", "model"},
	{"gemini", ".gemini/agents/rev.md", "model"},
	{"trae", ".trae/agents/rev.md", "model"},
	{"codex", ".codex/agents/rev.toml", "model"},
	{"vibe", ".vibe/agents/rev.toml", "active_model"},
}

// needNativeModel skips a case whose spec leaves the native file without
// a model line, which Trae does for a shared model: its format takes only
// a scoped one, so a missing line there is no deletion either.
func needNativeModel(t *testing.T, file, key string) {
	t.Helper()
	content := readFile(t, file)
	if !strings.Contains(content, key+":") && !strings.Contains(content, key+" =") {
		t.Skip("target writes no native model for this spec")
	}
}

func editNativeModel(t *testing.T, file, key, value string) {
	t.Helper()
	needNativeModel(t, file, key)
	if value == "" && strings.HasPrefix(file, ".trae/") {
		t.Skip("Trae's field set does not list model, so a missing line is not read as a deletion")
	}
	lines := strings.Split(readFile(t, file), "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, key+":"):
			lines[i] = key + ": " + value
		case strings.HasPrefix(line, key+" ="):
			lines[i] = fmt.Sprintf("%s = %q", key, value)
		default:
			continue
		}
		if value == "" {
			lines = append(lines[:i], lines[i+1:]...)
		}
		mustWriteFile(t, file, strings.Join(lines, "\n"))
		return
	}
	t.Fatalf("%s has no %s line:\n%s", file, key, readFile(t, file))
}

// hasBlock reports whether got holds the `model:` block in want. The Codex
// importer rewrites a changed spec with sorted keys and four-space
// indents, so each block is compared by its sorted entries.
func hasBlock(got, want string) bool {
	return strings.Contains(sortBlocks(got), sortBlocks(want))
}

func sortBlocks(doc string) string {
	lines := strings.Split(doc, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		if lines[i] != "model:" {
			continue
		}
		var entries []string
		for i+1 < len(lines) && strings.HasPrefix(lines[i+1], " ") {
			i++
			entries = append(entries, "  "+strings.TrimSpace(lines[i]))
		}
		slices.Sort(entries)
		out = append(out, entries...)
	}
	return strings.Join(out, "\n")
}

func reimportModelMap(t *testing.T, target string) string {
	t.Helper()
	return reimportEffortMap(t, target)
}

func TestImportAgents_ReimportUpdatesOnlyTheTargetsModelEntry(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  %s: m-one\n  zed: low\n  default: m-two\n", tc.target))
			editNativeModel(t, tc.file, tc.key, "m-new")

			got := reimportModelMap(t, tc.target)
			want := fmt.Sprintf("model:\n  %s: m-new\n  zed: low\n  default: m-two\n", tc.target)
			if !hasBlock(got, want) {
				t.Errorf("only %s's entry should change:\n%s", tc.target, got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportKeepsAnUnchangedModelMapAsItIs(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, "model:\n  zed: low\n  default: m-one\n")
			before := readFile(t, ".agnostic-ai/agents/rev.md")

			if got := reimportModelMap(t, tc.target); got != before {
				t.Errorf("a value the map already gives %s must not rewrite it:\n%s", tc.target, got)
			}
		})
	}
}

func TestImportAgents_ReimportAddsATargetModelEntryWhenTheValueDiffersFromDefault(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, "model:\n  zed: low\n  default: m-one\n")
			editNativeModel(t, tc.file, tc.key, "m-new")

			got := reimportModelMap(t, tc.target)
			want := fmt.Sprintf("model:\n  zed: low\n  default: m-one\n  %s: m-new\n", tc.target)
			if !hasBlock(got, want) {
				t.Errorf("a new entry for %s expected:\n%s", tc.target, got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportSetsTheEntryWhenTheNativeModelBecomesTheDefault(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  %s: m-one\n  default: m-two\n", tc.target))
			editNativeModel(t, tc.file, tc.key, "m-two")

			got := reimportModelMap(t, tc.target)
			want := fmt.Sprintf("model:\n  %s: m-two\n  default: m-two\n", tc.target)
			if !hasBlock(got, want) {
				t.Errorf("the entry should follow the native file:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportRemovesOnlyTheTargetsModelEntryWhenTheNativeKeyIsGone(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  %s: m-one\n  zed: low\n", tc.target))
			editNativeModel(t, tc.file, tc.key, "")

			got := reimportModelMap(t, tc.target)
			if !hasBlock(got, "model:\n  zed: low\n") || strings.Contains(got, tc.target+": m-one") {
				t.Errorf("only %s's entry should go:\n%s", tc.target, got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportDropsTheModelMapWhenItsLastEntryGoes(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  %s: m-one\n", tc.target))
			editNativeModel(t, tc.file, tc.key, "")

			if got := reimportModelMap(t, tc.target); strings.Contains(got, "model:") {
				t.Errorf("an empty map should go:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportKeepsAScalarModelScalar(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, "model: m-one\n")
			if !strings.Contains(readFile(t, tc.file), tc.key) {
				t.Skip("target writes no native model for a scalar")
			}
			editNativeModel(t, tc.file, tc.key, "m-new")

			if got := reimportModelMap(t, tc.target); !strings.Contains(got, "model: m-new\n") {
				t.Errorf("a scalar model should stay one:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_DeletedNativeModelStaysGoneWhenTheMapHasADefault(t *testing.T) {
	shapes := map[string]string{
		"entry and default": "model:\n  %s: m-one\n  default: m-two\n",
		"default only":      "model:\n  zed: low\n  default: m-two\n",
	}
	for _, tc := range modelMapTargets {
		for name, shape := range shapes {
			t.Run(tc.target+"/"+name, func(t *testing.T) {
				model := shape
				if strings.Contains(shape, "%s") {
					model = fmt.Sprintf(shape, tc.target)
				}
				effortMapProject(t, tc.target, model)
				editNativeModel(t, tc.file, tc.key, "")

				got := reimportModelMap(t, tc.target)
				if !strings.Contains(got, "default: m-two") {
					t.Errorf("the other targets keep their values:\n%s", got)
				}
				assertSyncCheckClean(t)
				if native := readFile(t, tc.file); strings.Contains(native, tc.key+":") || strings.Contains(native, tc.key+" =") {
					t.Errorf("the deleted %s must stay gone after sync:\n%s\nspec:\n%s", tc.key, native, got)
				}
			})
		}
	}
}

func modelTierProject(t *testing.T, target, spec string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [%s]\nmodels:\n  strong: {%s: m-one}\n", target, target))
	mustWriteFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: r\n"+spec+"---\n\nbody\n")
	runSyncOK(t)
}

func TestImportAgents_ReimportKeepsATierNamedInAModelMapDefault(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			modelTierProject(t, tc.target, "model:\n  zed: low\n  default: strong\n")
			before := readFile(t, ".agnostic-ai/agents/rev.md")

			if got := reimportModelMap(t, tc.target); got != before {
				t.Errorf("the tier's model for %s is not a change:\n%s", tc.target, got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportWritesTheNativeModelNotTheTierForAChangedTarget(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			modelTierProject(t, tc.target, "model:\n  zed: low\n  default: strong\n")
			editNativeModel(t, tc.file, tc.key, "m-new")

			got := reimportModelMap(t, tc.target)
			want := fmt.Sprintf("model:\n  zed: low\n  default: strong\n  %s: m-new\n", tc.target)
			if !hasBlock(got, want) {
				t.Errorf("the entry holds the native name and the tier stays the default:\n%s", got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportAgents_ReimportLeavesTheModelMapWhenXTargetModelWins(t *testing.T) {
	for _, tc := range modelMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("model:\n  zed: low\n  default: m-two\nx-%s:\n  model: m-new\n", tc.target))
			before := readFile(t, ".agnostic-ai/agents/rev.md")

			if got := reimportModelMap(t, tc.target); got != before {
				t.Errorf("x-%s.model gives the native value, so the map stays:\n%s", tc.target, got)
			}
			assertSyncCheckClean(t)
		})
	}
}

func TestImportCodebuddy_ReimportKeepsTheModelMapAndScopesTheNativeModel(t *testing.T) {
	effortMapProject(t, "codebuddy", "model:\n  codebuddy: m-one\n  zed: low\n  default: m-two\n")
	editNativeModel(t, ".codebuddy/agents/rev.md", "model", "m-new")

	got := reimportModelMap(t, "codebuddy")
	if !hasBlock(got, "model:\n  codebuddy: m-one\n  zed: low\n  default: m-two\n") || !strings.Contains(got, "x-codebuddy:\n  model: m-new\n") {
		t.Errorf("the importer scopes the value under x-codebuddy and the map stays:\n%s", got)
	}
	assertSyncCheckClean(t)
}

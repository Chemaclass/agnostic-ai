package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	// gooseMainFile is the entry point Goose reads at the project root,
	// and the file `sync` inlines rule bodies into by default.
	gooseMainFile = "AGENTS.md"
	// gooseHintsFile is Goose's older concatenated hints document. It is
	// opt-in on the emit side (`outputs.goose.rules-file`), so it is read
	// only when AGENTS.md carried no rules.
	gooseHintsFile = ".goosehints"
	// gooseAgentsDir is Goose's project-agent path: "Project agents are
	// available when goose is working in that project: `<project>/
	// .agents/agents/`" (documentation/docs/guides/context-engineering/
	// custom-agents.md). OpenHands reads the same tree.
	gooseAgentsDir = ".agents/agents"
	// gooseSkillsDir is the cross-tool skills tree Goose calls "the
	// recommended standard", shared byte-for-byte with codex, amp, zed,
	// and crush.
	gooseSkillsDir = ".agents/skills"
	// goosePluginsDir holds Open Plugins packages: "Project plugin |
	// `<project>/.agents/plugins/<plugin-name>/`" (plugins.md). Each
	// package may carry a `skills/` directory, a `hooks/hooks.json`, or
	// both.
	goosePluginsDir = ".agents/plugins"
	// gooseReviewFile is the review-instruction file `goose review`
	// reads: "goose review can discover review checks from
	// `.agents/checks/*.md` and scoped review instructions from
	// `.agents/REVIEW.md`" (documentation/docs/guides/goose-cli-commands.md).
	gooseReviewFile = ".agents/REVIEW.md"
	// gooseReviewSpecName is the filename the root review file imports
	// to. The spec itself is target-neutral, so it is not named after
	// goose: every review-capable target emits it on the next sync.
	gooseReviewSpecName = "review"
)

// importFromGoose reads an existing Block Goose project and writes specs
// into the configured source directories, reversing the goose emit:
//
//   - `AGENTS.md` carries rule bodies inlined in a sentinel-marked
//     `## Rules` block (Goose has no per-rule directory); each
//     `### <name>` child becomes a rule. The same file mirrors to
//     `.agnostic-ai/AGNOSTIC_AI.md`. A project that opted into
//     `.goosehints` instead falls back to that document, read the same
//     way, when AGENTS.md held no rules.
//   - `.agents/agents/*.md` reconstructs project agents byte-for-byte
//     minus the provenance header, so `description` and `model`
//     round-trip untouched.
//   - `.agents/skills/<name>/SKILL.md` folders reconstruct skills, with
//     bundled sibling assets copied byte-for-byte.
//   - `.agents/plugins/<name>/` packages contribute both components a
//     plugin may carry: `skills/<name>/SKILL.md` folders and
//     `hooks/hooks.json` matcher groups. See importGoosePlugins.
//   - `.agents/REVIEW.md` reconstructs one review spec.
//
// Lossy fields (the goose emit cannot carry them, so a round-trip drops
// them without changing Goose's output): a rule's source-layout scope
// collapses, since rules reach Goose through one inlined block per
// scope and only the root block is read back, the same line
// `import claude` holds on nested CLAUDE.md files; review specs sharing
// a scope concatenate into one file on emit and re-import as a single
// spec; a scoped `<scope>/.agents/REVIEW.md` is not read back for the
// same reason.
func importFromGoose(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Hooks, src.Reviews); err != nil {
		return err
	}
	rules, err := importGooseRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importFlatMarkdownFiles(filepath.Join(root, gooseAgentsDir), filepath.Join(root, src.Agents), gooseAgentFields)
	if err != nil {
		return err
	}
	skills, err := importSkillFolders(filepath.Join(root, gooseSkillsDir), filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	pluginSkills, hooks, err := importGoosePlugins(root, src)
	if err != nil {
		return err
	}
	reviews, err := importGooseReview(root, filepath.Join(root, src.Reviews))
	if err != nil {
		return err
	}
	if _, err := mirrorMainFile(root, gooseMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d hooks, %d reviews\n",
		rules, agents, skills+pluginSkills, hooks, reviews)
	printImportNextSteps(root, "goose")
	return nil
}

// importGooseRules reads rule bodies from `AGENTS.md`, falling back to
// `.goosehints` when that file carried none. Both are read by the same
// H2 slicer: the root hints document is a merged document with the same
// `## Rules` / `### <name>` shape, and a hand-authored one with no
// headings still imports as a single rule.
//
// The fallback is one-way rather than additive on purpose. With the
// `outputs.goose.rules-file` opt-in set, sync writes the same rule
// bodies to both files, so reading both would import every rule twice
// and report a doubled count.
func importGooseRules(root, dstDir string) (int, error) {
	// With the `outputs.goose.rules-file` opt-in set, sync writes the
	// rule bodies to `.goosehints` and leaves AGENTS.md a pointer body
	// with no sentinel, which imports nothing and falls through (#894).
	rules, err := sliceEntryPointRules(root, gooseMainFile, dstDir)
	if err != nil || rules > 0 {
		return rules, err
	}
	return sliceMainFileByH2(root, gooseHintsFile, dstDir)
}

// importGoosePlugins walks `.agents/plugins/` and imports each package's
// components: skill folders under `<plugin>/skills/` and hook groups
// from `<plugin>/hooks/hooks.json`. Returns the skill and hook counts.
// A missing plugins directory imports nothing.
//
// Every plugin is read, not just the `agnostic-ai` one this tool writes.
// A hand-installed package is exactly the configuration a migrating
// project wants picked up, and `outputs.goose.hooks-file` can point the
// emit side at any plugin root already.
func importGoosePlugins(root string, src config.Sources) (skills, hooks int, err error) {
	dir := filepath.Join(root, filepath.FromSlash(goosePluginsDir))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		plugin := filepath.Join(dir, e.Name())
		n, err := importSkillFolders(filepath.Join(plugin, "skills"), filepath.Join(root, src.Skills))
		if err != nil {
			return skills, hooks, err
		}
		skills += n
		n, err = importGooseHooks(filepath.Join(plugin, "hooks", "hooks.json"), filepath.Join(root, src.Hooks))
		if err != nil {
			return skills, hooks, err
		}
		hooks += n
	}
	return skills, hooks, nil
}

// gooseHookEntry is one command inside a matcher group. Goose adds
// `on_failure` to the shared `{type, command, timeout}` object, its
// PreToolUse failure policy.
type gooseHookEntry struct {
	groupedHookEntry
	OnFailure string `json:"on_failure"`
}

// gooseHookGroup mirrors one `{matcher, hooks: [...]}` object inside
// `hooks.<EventName>`.
type gooseHookGroup struct {
	Matcher string           `json:"matcher"`
	Hooks   []gooseHookEntry `json:"hooks"`
}

// importGooseHooks reads one plugin's `hooks/hooks.json` and writes one
// yaml per matcher group into dstDir. No-op when the file or its `hooks`
// key is absent.
//
// `on_failure` is per-command in the file and per-spec here, so the
// first non-empty value in a group wins: the emit side writes the spec's
// single value onto every command in the group, which is the only shape
// that survives a second round-trip.
func importGooseHooks(src, dstDir string) (int, error) {
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc struct {
		Hooks map[string][]gooseHookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	count := 0
	for _, event := range sortedHookEvents(doc.Hooks) {
		for _, g := range doc.Hooks[event] {
			entries := make([]groupedHookEntry, 0, len(g.Hooks))
			onFailure := ""
			for _, h := range g.Hooks {
				entries = append(entries, h.groupedHookEntry)
				if onFailure == "" {
					onFailure = h.OnFailure
				}
			}
			var extra map[string]any
			if onFailure != "" {
				extra = map[string]any{"x-goose": map[string]any{"on_failure": onFailure}}
			}
			n, err := writeGroupedHookSpec(dstDir, "goose", event, g.Matcher, entries, extra)
			if err != nil {
				return count, err
			}
			count += n
		}
	}
	return count, nil
}

// importGooseReview reads the root `.agents/REVIEW.md` and writes one
// review spec into dstDir. The file is a plain body with no frontmatter
// (the loader reads it as plain text), so the spec carries only a name.
// No-op when the file is absent or holds nothing but the provenance
// header.
func importGooseReview(root, dstDir string) (int, error) {
	src := filepath.Join(root, filepath.FromSlash(gooseReviewFile))
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	body := strings.TrimSpace(header.Strip(string(data)))
	if body == "" {
		return 0, nil
	}
	out := filepath.Join(dstDir, gooseReviewSpecName+".md")
	if err := writeRule(out, gooseReviewSpecName, body); err != nil {
		return 0, err
	}
	return 1, nil
}

// Package cline emits configs for the Cline VSCode extension and CLI.
//
// Rules emit as Markdown files under `.clinerules/`. Cline reads two
// project rules layouts, not one: `resolveWorkspaceRulesConfigPaths`
// (sdk/packages/shared/src/storage/paths.ts) returns both
// `<workspace>/.clinerules` and `<workspace>/.cline/rules`, over a
// comment reading "Every Cline surface (CLI, VS Code extension,
// desktop app) must honor both". The docs agree: "Both layouts are
// supported by VS Code, Desktop, and the CLI" and "Both directories
// are searched when present"
// (docs.cline.bot/customization/cline-rules). `.clinerules/` stays the
// default here because the VS Code Rules panel still creates there.
// Set `outputs.cline.rules-dir: .cline/rules` to emit at the other
// layout, which is read just as well; a stale managed tree at the
// layout you are not using is swept on sync.
//
// An earlier version of this doc called `.cline/rules` a path no Cline
// surface reads (target-audit 2026-09-18, #853). That was wrong. It
// came from counting `GlobalFileNames` hits in the VS Code extension,
// and `GlobalFileNames.clineRules` now has zero call sites, so the
// count measured nothing (target-audit 2026-09-19). Note also that
// `.clinerules` is not deprecated: the SDK spells its constant
// `DEPRECATED_CONFIG_DIR`, but docs.cline.bot/resources/deprecations
// lists only `.clineignore`, "Explain Changes" and "Focus Chain".
//
// Agents stay at `.cline/agents/`, on positive source evidence:
// `resolveAgentConfigSearchPaths` returns `<workspace>/.cline/agents`
// (sdk/packages/shared/src/storage/paths.ts:476), and all three
// shipping readers resolve the directory through it. Skills stay at
// `.cline/skills/`, confirmed by `resolveSkillsConfigSearchPaths` in
// the same file, which also scans `.clinerules/skills` and
// `.agents/skills`.
//
// Emission stays at `.cline/skills/` even though `.agents/skills` is
// now scanned too. `.cline/skills` is the path the docs recommend, the
// one the VS Code extension lists first among its own project entries,
// and the current default that `outputs.cline.skills-dir` overrides.
// `.agents/skills` is the tree amp, codex, windsurf and zed write, so
// moving cline onto it would put two adapters' renders in one folder
// and change every existing user's layout. A project that wants one
// on-disk copy has two supported ways to get it: `sync.shared-skills:
// true`, which links byte-identical trees, or
// `outputs.cline.skills-dir: .agents/skills`. Import reads
// `.agents/skills` either way.
//
// An agent file is `<name>.yml` carrying `---`-delimited frontmatter
// over a Markdown system prompt. Every part of that is load-bearing in
// `configured-agent-config.ts`: `isYamlFile` accepts only `.yml` and
// `.yaml` (L113-115) and the directory scan skips everything else
// (L176-178); the content must match /^(---)[^\S\r\n]*(?:\r?\n|$)/ or
// the loader throws "Missing YAML frontmatter block in agent config
// file." (L49-52); and the schema requires `name` and `description`,
// both `z.string().trim().min(1)` (L8-10). A spec with no description
// falls back to its name, since an empty one fails the schema and
// drops the agent. The five optional keys (`tools`, `skills`,
// `providerId`, `modelId`, `maxIterations`) go in through `x-cline`.
//
// The provenance comment sits below the closing delimiter, inside the
// system prompt. Above it, it would break the `^---` anchor and the
// file would not load at all. Releases #534 through #886 wrote
// `<name>.md` with no frontmatter and a leading HTML comment, so every
// agent was invisible to `cline config`, to Agent Teams, and to the
// hub (target-audit 2026-09-19, #886). A stale managed `.md` there is
// swept on sync; hand-authored files survive.
//
// No heading is synthesized above the body. A "# Agent: <name>" line
// (the pre-#534 rule-form shape) would round-trip back into the body
// on the next `import cline` and double itself on the next sync, since
// nothing downstream of a native agent file expects to strip one out.
//
// A rule that declares `alwaysApply: false` narrows through Cline's one
// conditional, `paths`: "Currently, `paths` is the supported
// conditional. It takes an array of glob patterns"
// (docs.cline.bot/customization/cline-rules). Explicit globs win over
// the rule's source-layout scope, which falls back to `<scope>/**`.
// Everything else stays bare, since "Rules without frontmatter are
// always active" and writing an empty block would churn every existing
// file for no behavior change. Before this existed no file carried
// frontmatter at all, so a rule scoped to `src/components/**` loaded on
// every request (target-audit 2026-08-27, #639).
//
// A rule that asks not to be always-on but gives nothing to match on
// has no Cline representation: `paths: []` is documented, but it "means
// the rule never activates", which disables the rule instead of
// narrowing it, so that case stays always-active and surfaces as a
// coverage note.
//
// Cline also reads the cross-tool root `AGENTS.md`, which is written
// centrally by `sync` as a slim pointer to the source specs (shared
// with codex, amp, warp, and zed).
//
// Skills emit as a folder per skill under `.cline/skills/<name>/
// SKILL.md`, Cline's recommended skills path
// (docs.cline.bot/customization/skills also accepts `.clinerules/
// skills/` and `.claude/skills/`, but never a flat file directly under
// `.clinerules/`); the SKILL.md frontmatter carries `name` +
// `description` and sibling assets copy byte-for-byte.
//
// Hooks emit as one executable script per event under `.cline/hooks/`,
// named after the event. Cline discovers a hook by file name and
// nothing else: `HookConfigFileName` declares ten names
// (sdk/packages/core/src/hooks/hook-file-config.ts:17), an extension
// allowlist gates the file (L49), and `listHookConfigFiles` (L81) scans
// `resolveHooksConfigSearchPaths`, which at paths.ts:487-500 pushes
// `<workspace>/.clinerules/hooks` and `<workspace>/.cline/hooks`. The
// scripts are run, not merely listed: `hook-file-hooks.ts` builds a
// command per file with `inferHookCommand` and `parseShebangCommand`,
// then runs it through `runSubprocessEvent`.
//
// This is source-confirmed and doc-unconfirmed.
// docs.cline.bot/customization/hooks is still a one-line stub pointing
// at SDK Plugins, where hooks are a TypeScript `AgentPlugin` API rather
// than shell-command-on-event; that stub is why this adapter declined
// hooks until now. docs.cline.bot/getting-started/config corroborates
// the directory, showing `.cline/  hooks/  # Lifecycle hooks` in its
// project tree and warning "Hooks and plugins can execute code."
//
// The shape has no matcher and no JSON wrapper, so a Claude-style
// `matcher` is inert and earns a note. So is `timeout`: Cline times
// hook subprocesses out on a runtime setting, not per file. Two specs
// on one event share one script, since the file name is the event and
// there is no second slot. `PreCompact` is a name Cline lists but maps
// to no runtime event today (hook-file-config.ts:40), so its script is
// written, reported, and never run until Cline wires the event up.
//
// When `outputs.cline.workflows-dir` is set, each agent additionally
// emits as a Markdown file at `<dir>/<name>.md`, in the shape this
// adapter calls a Workflow: invokable in chat as `/<name>.md`. The
// surface is confirmed in source even though
// docs.cline.bot/features/workflows still 404s:
// `resolveWorkflowsConfigSearchPaths` (paths.ts:595) returns
// `<workspace>/.clinerules/workflows` and `<workspace>/.cline/workflows`.
// Set the key to `.clinerules/workflows`. That is the only one of the
// two the VS Code extension resolves (`GlobalFileNames.workflows` in
// disk.ts:24, read by workflows.ts), and the extension excludes it from
// the rules scan, so a workflow there never doubles as an always-on
// rule. Pointing the key at `.cline/workflows` reaches the CLI and SDK
// only, and says so in a note. The key stays opt-in either way: a
// workflow is a second copy of an agent already emitted at
// `.cline/agents/<name>.yml`.
package cline

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "cline"
	defaultRulesDir  = ".clinerules"
	defaultAgentsDir = ".cline/agents"
	defaultSkillsDir = ".cline/skills"
	// altRulesDir is the second project rules layout Cline reads (see
	// the package doc). Only one of the two layouts is written at a
	// time, so a sync that lands on the default sweeps a stale managed
	// copy here, unless the user opted into it explicitly via
	// outputs.cline.rules-dir.
	altRulesDir = ".cline/rules"
	// agentExt is the only extension the agent loader accepts besides
	// `.yaml` (isYamlFile, configured-agent-config.ts L113-115).
	agentExt = ".yml"
	// legacyAgentExt is what releases #534 through #886 wrote there.
	// The loader skips it, so a managed leftover is swept.
	legacyAgentExt = ".md"
	// recommendedWorkflowsDir is the workflows path both Cline hosts
	// read. `resolveWorkflowsConfigSearchPaths` (paths.ts:595) returns
	// it first, and it is the only one the VS Code extension resolves
	// (`GlobalFileNames.workflows` in disk.ts:24).
	recommendedWorkflowsDir = ".clinerules/workflows"
	// cliOnlyWorkflowsDir is the second path that resolver returns.
	// The CLI and the SDK read it; the VS Code extension does not.
	cliOnlyWorkflowsDir = ".cline/workflows"
)

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindHook},
}

// Adapter emits Cline configs.
type Adapter struct{}

// New returns a Cline adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one .md per rule under the rules directory (default
// `.clinerules`), one .yml per agent under the agents directory
// (default `.cline/agents`), and one folder per skill under the skills
// directory (Cline's native SKILL.md layout; a flat file there never
// loads as a skill). A stale managed tree at the rules layout not in
// use is swept unless the user explicitly opted into it via
// outputs.cline.rules-dir; Cline reads both layouts, so leaving one
// behind would load the rules twice. Hooks emit as one executable script per
// event under the hooks directory (default `.cline/hooks`), named
// after the event, which is how Cline discovers them. When
// `outputs.cline.workflows-dir` is set, each agent additionally emits
// as a Markdown file at `<dir>/<name>.md` in the shape this adapter
// calls a Workflow (see the package doc for why `.clinerules/workflows`
// is the path to set); the native agent file emission stays in place
// either way.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        rulesDir,
		SkipAgents: true,
		SkipSkills: true,
		FormatRule: rule,
	}, dryRun); err != nil {
		return err
	}
	emit.NoteFieldNoOp(target, spec.KindRule, "alwaysApply", unscopableRules(b.Rules),
		"Cline's only conditional is `paths`; a rule with no globs and no scope stays always-active")
	// Sweep managed leftovers at the other rules layout unless the user
	// explicitly opted to emit there. Both layouts are read, so leaving
	// a stale managed copy behind would load the rules twice.
	// Hand-authored files (no provenance marker) survive.
	if rulesDir != altRulesDir {
		if err := sess.RemoveGeneratedTree(altRulesDir, dryRun); err != nil {
			return err
		}
	}

	agentsDir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgentFiles(sess, b.Agents, agentsDir, dryRun); err != nil {
		return err
	}

	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}

	if err := emitHooks(sess, b.Hooks, cfg, dryRun); err != nil {
		return err
	}
	return emitWorkflows(sess, b, cfg, dryRun)
}

// emitAgentFiles writes one `<dir>/<name>.yml` per agent spec and
// sweeps the `<name>.md` files earlier releases left there, which no
// Cline surface reads (see the package doc).
func emitAgentFiles(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	if err := sess.RemoveGeneratedTreeExt(dir, legacyAgentExt, dryRun); err != nil {
		return err
	}
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+agentExt)
		if err := sess.WriteFile(path, emit.WithHeader(agentFile(a), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// agentFile renders one agent config: required `name` + `description`
// frontmatter, the five optional vendor keys through `x-cline`, and the
// spec body as the system prompt. The header format is Markdown, not
// YAML, because emit.WithHeader places a Markdown header below the
// closing delimiter; a `#` line above it would break the `^---` anchor
// the loader requires.
func agentFile(a spec.Entry) string {
	desc, _ := emit.ResolveMeta(a.Meta, target)["description"].(string)
	if desc == "" {
		// `description` is z.string().trim().min(1): an empty one
		// throws and the agent never loads. Mirror the skill emitter
		// and fall back to the name.
		desc = a.Name
	}
	meta := map[string]any{"name": a.Name, "description": desc}
	keys := []string{"name", "description"}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// emitWorkflows writes one workflow per agent under the configured
// workflows directory. No-op when the dir is unset: a workflow is a
// second copy of an agent this adapter already emits natively, so it
// stays opt-in rather than doubling every agent by default.
//
// `.clinerules/workflows` is the path to set. Both hosts read it, and
// the VS Code extension excludes it from the rules scan
// (`CLINERULES_EXCLUDED_SUBDIRECTORIES` in cline-rules.ts:11-15), so a
// workflow there never doubles as an always-on rule. `.cline/workflows`
// resolves in the CLI and SDK only, and earns a surface note.
func emitWorkflows(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputWorkflowsDir(cfg, target, "")
	if dir == "" {
		return nil
	}
	if filepath.ToSlash(filepath.Clean(dir)) == cliOnlyWorkflowsDir {
		emit.NoteSurfaceGap(target, spec.KindAgent, len(b.Agents), "the Cline VS Code extension",
			"it resolves workflows at `"+recommendedWorkflowsDir+"` only; set `outputs.cline.workflows-dir` there to reach both hosts")
	}
	for _, a := range b.Agents {
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(renderWorkflow(a), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// renderWorkflow returns the Markdown body for a Cline Workflow file.
// Cline workflows are plain Markdown with no required frontmatter; the
// description (when present) prefixes the body as an italic line so
// users can see at a glance what the workflow does when listed.
func renderWorkflow(e spec.Entry) string {
	if d := e.Description(); d != "" {
		return "_" + d + "_\n\n" + e.Body
	}
	return e.Body
}

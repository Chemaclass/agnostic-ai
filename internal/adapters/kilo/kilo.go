// Package kilo emits configs for Kilo Code.
//
// Rules emit as one Markdown file per rule spec under `.kilo/rules/`
// (override via outputs.kilo.rules-dir), each one also listed as its
// own entry in `kilo.jsonc`'s `instructions` array (raw doc
// packages/kilo-docs/pages/customize/custom-rules.md: "Each entry
// points to a file path or glob pattern"). This adapter lists explicit
// per-file paths rather than a `.kilo/rules/*.md` directory glob: a
// rule's scope nests its file under `.kilo/rules/<scope>/`, and the
// vendor doc's own glob example is a single, non-recursive `*.md`, so
// a lone glob entry would silently miss anything scoped. Kilo Code's
// own precedence order is agent prompt > project `instructions` >
// AGENTS.md > global (agents-md.md), so `instructions` outranks the
// project-root AGENTS.md that `sync` writes centrally as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). AGENTS.md "cannot be individually
// disabled: it is always loaded if present" (agents-md.md), so this
// adapter keeps inlining full rule bodies there too (see
// inlineRulesTargets in internal/adapters/internal/emit/
// rules_appendix.go): `instructions` is how a rule wins a conflict
// with a user's own entry, not a reason to drop the AGENTS.md fallback.
// `.kilocode/rules/` (the pre-rename Kilo Code branding) is a separate,
// genuinely legacy tree Kilo Code still reads automatically for
// backward compatibility; this adapter intentionally never emits it,
// and it is distinct from the current `.kilo/rules/` mechanism above
// (target-audit 2026-08-01, #535).
//
// Agents emit as one Markdown file per agent spec at
// `.kilo/agents/<name>.md` (override via outputs.kilo.agents-dir).
// Kilo Code takes the agent's name from the filename, not from
// frontmatter, so `name:` is never written. Frontmatter otherwise
// carries `description` (falls back to the spec name) plus `color`,
// `mode`, and `model` when the spec sets them. All three read from the
// plain top-level field: `color` is the same generic top-level key
// augment (augment.go:224) and qoder also promote, written through
// without per-target validation; the three targets document different
// value spaces (`color` support by target, docs/site/content/docs/spec-format.md), and
// `mode` shares OpenCode's `primary`/`subagent`/`all` vocabulary under
// the identical name. Kilo Code's full agent Configuration Options table
// also documents `disable`, `hidden`, `steps`, `temperature`, and `top_p`
// (target-audit 2026-08-08, #562); none has a confirmed counterpart on
// another registered target, and `temperature`/`top_p` are
// provider-scaled tuning knobs besides, so all five stay reachable only
// through `x-kilo` (e.g. `x-kilo: {temperature: 0.1, steps: 15}`) rather
// than a generic top-level key. Every other arbitrary `x-kilo`
// key passes through the same way. `tools` is never written under that
// spelling, or under `x-kilo`: Kilo Code's full agent option table has
// no `tools` field, so the key itself would be a silent no-op.
//
// The list behind it does reach Kilo Code now. Its real access control
// is a per-tool `permission` map (`allow` / `ask` / `deny`), and both
// halves of the translation are published: the permission table on
// kilo.ai/docs/getting-started/settings/auto-approving-actions and the
// tool-group table on kilo.ai/docs/automate/tools. A `tools` allowlist
// becomes `permission: {"*": deny, <tool>: allow, ...}`, the shape
// kilo.ai/docs/customize/agent-permissions documents outright: "Top
// level permission keys follow the same rule", with the example
// `permission: {"*": ask, bash: allow}`. The catch-all comes first
// because "the last matching rule wins". A name with no row in Kilo's
// table drops and folds into one coverage note per sync; if no name
// translates at all, no map is written, since a bare `{"*": deny}`
// would lock the agent out of everything nobody asked to restrict.
// `x-kilo: {permission: {...}}` still wins outright for an author who
// already knows Kilo's spelling (target-audit 2026-09-19, #890). The
// premise recorded here before that audit, that no vendor-confirmed
// mapping existed, had expired; see permission.go.
//
// Skills emit into the shared `.agents/skills/<name>/SKILL.md` tree
// (override via outputs.kilo.skills-dir): Kilo Code documents its own
// `.kilo/skills/` path, but also lists `.agents/skills/` as a
// compatibility directory "loaded by default" (target-audit
// 2026-08-01), and that is the same tree codex, amp, zed, crush,
// openhands, windsurf, and augment already write byte-identically, so
// pointing here dedupes instead of adding a second on-disk copy.
//
// Kilo Code scans three project skill trees without configuration:
// `.kilo/skills/`, `.agents/skills/`, and `.claude/skills/`. The third
// carries a condition on the VS Code tab, "`.claude/skills/` - Claude
// Code compatibility, loaded when Claude Code Compatibility is
// enabled", while the CLI tab still lists it unconditionally. Nothing
// here depends on it: this adapter writes `.agents/skills/`, which is
// unconditional on both tabs. An outputs.kilo.skills-dir pointing
// anywhere else is listed in `kilo.jsonc`'s `skills.paths`, which
// "accepts absolute paths, `~/` home-relative paths, or paths relative
// to the project root"
// (packages/kilo-docs/pages/customize/skills.md). Without that
// entry the folders were written where nothing reads them (target-audit
// 2026-09-18, #861). Entries a user put there themselves are carried
// over, and `skills.urls` is left alone.
//
// Commands emit as one Markdown file per command spec at
// `.kilo/commands/<name>.md` (override via outputs.kilo.commands-dir),
// the new Kilo Code extension's slash-command path: "Workflows are
// Markdown files stored as slash commands in `.kilo/commands/`"
// (packages/kilo-docs/pages/customize/workflows.md, mirrored on GitHub
// since kilo.ai's rendered docs defeat fetching). Kilo Code takes the
// command name from the filename, so `name` is never written.
// Frontmatter carries `description`, `agent`, `model`, `variant`, and
// `subtask`, near-identical to OpenCode's own command frontmatter
// (internal/adapters/opencode); `variant` (a reasoning-effort override)
// is the one extra key this vendor documents. Arbitrary `x-kilo` keys
// pass through the same way commands.go's OpenCode counterpart does
// (#630). One name is off limits: "A custom command or an MCP prompt
// named `goal` is reserved. Kilo rejects it and reports an error;
// rename it" (code-with-ai/agents/goals, shipped in v7.6.0). A command
// spec called `goal` still emits, so the spec is never lost in
// silence, and surfaces a coverage note naming the rename (#736).
//
// MCP servers merge into the project `kilo.jsonc` (override via
// outputs.kilo.mcp-file) under an `mcp` map, the key current Kilo Code
// reads (`mcpServers` is the deprecated MCP-spec 2025-03-26 form).
// Stdio entries combine `command` + `args` into one `command` array
// and set `"type": "local"`; HTTP / SSE / remote entries render as
// `{"type": "remote", "url": ..., "headers": {...}}`. `environment`
// (not `env`) carries a stdio server's environment variables. A spec's
// `disabled: true` writes `"enabled": false`, the key Kilo Code's own
// documented MCP example carries; `disabled` itself is never written,
// and an enabled server (the common case) gets no explicit key at all.
// kilo.jsonc also holds user-managed keys (models, providers, ...);
// the merge only touches `mcp` and `instructions` and skips whichever
// of the two has nothing to contribute, so those survive a sync. This
// adapter writes plain JSON: JSONC is a superset of JSON, so every
// JSONC parser accepts the output, and agnostic-ai never needs to emit
// comments of its own.
//
// It does need to read them. The vendor documents JSONC on this exact
// file ("Disable a rule temporarily: Comment out the line in kilo.jsonc
// (JSONC supports // comments)", kilo.ai/docs/customize/custom-rules),
// and the page's own worked example carries both a comment and two
// trailing commas. `encoding/json` rejects both, so emit.MergeJSONFile
// strips JSONC before parsing. Keys survive; comments do not, since the
// document is re-rendered from parsed values, and the sync that drops
// them says so (target-audit 2026-09-11, #725).
//
// Kilo's docs also read from `.kilo/kilo.jsonc` when present, a second
// project-tier config file this adapter does not write. The vendor's
// documented 8-level config precedence
// (kilo.ai/docs/getting-started/settings#config-file-precedence) places
// `.kilo/` above project-root `kilo.jsonc` and describes higher levels
// as overriding lower ones, i.e. a merge, not an exclusive first-match
// read: an untouched key on the root file still reaches Kilo Code even
// when `.kilo/kilo.jsonc` exists. A hand-authored `.kilo/kilo.jsonc`
// that redeclares `mcp` or `instructions` would still shadow this
// adapter's output for those two keys specifically (target-audit
// 2026-08-27, #644).
//
// Ignore specs emit project-root .kilocodeignore (outputs.kilo.ignore-file
// overrides the path). Kilo migrates that compatibility input into read/edit
// permission denials; this adapter does not translate patterns itself.
// Settings specs merge their last non-empty model into top-level `model` in
// `kilo.jsonc`, alongside the instructions and MCP keys.
//
// The portable `allow`, `deny`, and `ask` lists merge into the same
// file's `permission` key, the map the vendor names outright:
// "Permissions are configured under the `permission` key in
// `kilo.jsonc`". Each rule becomes one glob pattern under one tool
// key, since Kilo matches "against the tool's arguments (command
// strings, file paths, etc.)": `Bash(git:*)` becomes
// `bash: {"git *": "allow"}`, `Read(docs/*)` becomes
// `read: {"docs/*": "allow"}`, and a bare `Bash` becomes
// `bash: {"*": "allow"}`. The emitted key order is alphabetical, which
// puts `*` ahead of every exception, the order Kilo asks for: "Put
// broad fallbacks first and exceptions after them", since "the last
// matching rule wins". One rule repeated across two lists resolves to
// the stricter action. Anything with no key in Kilo's table drops with
// a coverage note, and `x-kilo.permission` on a settings spec wins
// outright for the tool keys it names (target-audit 2026-09-19, #890).
// See permission.go.
//
// MCP entries preserve timeout in milliseconds, including zero, and remote
// oauth:false, with x-kilo overrides. import kilo reads the ignore file, the
// portable default model, and the `permission` map.
package kilo

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "kilo"
	defaultRulesDir  = ".kilo/rules"
	defaultAgentsDir = ".kilo/agents"
	defaultMCPFile   = "kilo.jsonc"
	// defaultSkillsDir is the shared cross-tool skills tree Kilo Code
	// scans by default (alongside its own .kilo/skills/); codex, amp,
	// zed, crush, openhands, windsurf, and augment already write here,
	// so identical skill folders dedupe under sync.shared-skills.
	defaultSkillsDir = ".agents/skills"
	// defaultCommandsDir is the new Kilo Code extension's slash-command
	// path: "Workflows are Markdown files stored as slash commands in
	// `.kilo/commands/`" (packages/kilo-docs/pages/customize/
	// workflows.md). "Project commands" in the vendor's own wording.
	defaultCommandsDir = ".kilo/commands"
)

const defaultIgnoreFile = ".kilocodeignore"

// scannedSkillTrees are the only project directories Kilo Code loads
// skills from without configuration: its own `.kilo/skills/`, the
// shared `.agents/skills/` ("Open agent standard, loaded by default"),
// and `.claude/skills/`. A skills dir outside all three reaches Kilo
// Code only through `skills.paths` in kilo.jsonc (#861).
var scannedSkillTrees = map[string]bool{
	".kilo/skills":   true,
	".agents/skills": true,
	".claude/skills": true,
}

var caps = emit.Capabilities{
	Target:   target,
	Supports: []spec.Kind{spec.KindRule, spec.KindAgent, spec.KindMCP, spec.KindSkill, spec.KindCommand, spec.KindIgnore, spec.KindSettings},
}

// Adapter emits Kilo Code configs.
type Adapter struct{}

// New returns a Kilo adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes one Markdown file per rule under `.kilo/rules/`, one
// agent Markdown file per agent spec under `.kilo/agents/`, one shared
// `.agents/skills/<name>/SKILL.md` folder per skill, one command
// Markdown file per command spec under `.kilo/commands/`, plus a
// merged `kilo.jsonc` carrying the `instructions` array (one entry per
// rule file), the `mcp` map, a `skills.paths` entry when the skills dir
// is outside the trees Kilo Code scans by itself, and a portable
// default `model`. The project-root AGENTS.md (still read, but lower
// priority than `instructions`; see the package doc) is written by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	if err := sess.WriteIgnoreFile(b.Ignores, target, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	rulesDir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	if err := sess.RulesDirectory(b, emit.RulesDirOpts{
		Dir:        rulesDir,
		SkipAgents: true,
		SkipSkills: true,
	}, dryRun); err != nil {
		return err
	}
	dir := emit.OutputAgentsDir(cfg, target, defaultAgentsDir)
	if err := emitAgents(sess, b.Agents, dir, dryRun); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	commandsDir := emit.OutputCommandsDir(cfg, target, defaultCommandsDir)
	if err := emitCommands(sess, b.Commands, commandsDir, dryRun); err != nil {
		return err
	}
	return emitKiloJSONC(sess, b, rulesDir, skillsDir, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitAgents writes one `<dir>/<name>.md` per agent spec. A spec's
// `tools` allowlist becomes Kilo's own `permission` frontmatter (see
// agentMarkdown); only the names outside Kilo's vocabulary drop, and
// the whole batch surfaces one coverage note for those instead of a
// silent loss.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	unmapped := 0
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".md")
		md, hadUnmapped := agentMarkdown(a)
		if hadUnmapped {
			unmapped++
		}
		body := emit.WithHeader(md, emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", unmapped, agentToolsUntranslatedReason)
	return nil
}

// agentMarkdown renders a single agent definition: `description`
// (falls back to the spec name) plus `color`, `mode`, and `model` when
// set, then the `permission` map a `tools` allowlist translates into,
// followed by arbitrary x-kilo passthrough and the spec body as the
// agent's system prompt. Kilo Code takes the agent name from the
// filename, so `name` is never written; `tools` is never written under
// that spelling either, since Kilo Code's agent option table has no
// such key. Both stay excluded from the x-kilo passthrough too, so an
// escape-hatch attempt cannot reintroduce a confirmed no-op key.
// `color` and `mode` are also excluded from the x-kilo passthrough
// below: ResolveMeta already flattens any `x-kilo.color` /
// `x-kilo.mode` onto `resolved` before this function runs, so the
// top-level loop above already carries an override through, and
// re-merging the same key from raw `e.Meta` would only be redundant,
// not additive.
//
// An `x-kilo.permission` wins outright: it reaches the frontmatter
// through the passthrough and the translated map is not built at all,
// so the two never fight over one key.
//
// unmapped reports whether the spec named at least one tool Kilo Code
// has no permission key for, so the caller can fold those into one
// coverage note per sync instead of losing them silently.
func agentMarkdown(e spec.Entry) (body string, unmapped bool) {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	meta := map[string]any{
		"description": desc,
	}
	keys := []string{"description"}
	for _, k := range []string{"color", "mode", "model"} {
		if v, _ := resolved[k].(string); v != "" {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	if !hasNativePermission(e) {
		var permission map[string]any
		permission, unmapped = agentPermission(emit.StringSlice(resolved["tools"]))
		if len(permission) > 0 {
			meta[permissionKey] = permission
			keys = append(keys, permissionKey)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, e.Meta, target, "description", "color", "mode", "model", "name", "tools")
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(e.Body)
	if trimmed == "" {
		return front + "\n", unmapped
	}
	return front + "\n" + trimmed + "\n", unmapped
}

// hasNativePermission reports whether the spec already carries an
// `x-kilo.permission` object. That object wins outright over the
// translated form, so the translation is skipped entirely rather than
// written and then overwritten by the passthrough.
func hasNativePermission(e spec.Entry) bool {
	custom, _ := emit.CustomTargetMeta(e.Meta, target)
	if custom == nil {
		return false
	}
	_, ok := custom[permissionKey]
	return ok
}

// emitKiloJSONC merges the `instructions`, `mcp`, `skills`, and `model`
// keys into kilo.jsonc in a single read-modify-write. Routes through
// emit.MergeJSONFile so any pre-existing user-managed keys (providers,
// themes, ...) survive the sync, in JSONC form as well as plain JSON
// (see the package doc). `skills` merges one level deep so a user's own
// `skills.urls` survives alongside the managed `skills.paths`. Each key
// is set only when its source contributes, and no file is written when
// every source is empty.
func emitKiloJSONC(sess *emit.Session, b spec.Bundle, rulesDir, skillsDir, path string, dryRun bool) error {
	keys := map[string]any{}
	if instructions := ruleInstructions(b.Rules, rulesDir); len(instructions) > 0 {
		keys["instructions"] = instructions
	}
	if servers := buildMCPMap(b.MCPs); len(servers) > 0 {
		keys["mcp"] = servers
	}
	if paths := skillsPaths(sess, b.Skills, skillsDir, path, dryRun); len(paths) > 0 {
		keys["skills"] = map[string]any{"paths": paths}
	}
	if model := emit.LastSettingsModel(b.Settings); model != "" {
		keys["model"] = model
	}
	permission, dropped := settingsPermission(b.Settings)
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", dropped, permissionUntranslatedReason)
	if len(permission) > 0 {
		keys[permissionKey] = permission
	}
	if len(keys) == 0 {
		return nil
	}
	return sess.MergeJSONFileNested(path, keys, []string{"skills", permissionKey}, dryRun)
}

// skillsPaths returns the `skills.paths` list kilo.jsonc needs so Kilo
// Code scans the configured skills directory, or nil when the directory
// is one Kilo Code already scans. Vendor: "The `skills.paths` key
// accepts absolute paths, `~/` home-relative paths, or paths relative to
// the project root" (packages/kilo-docs/pages/customize/skills.md).
//
// Any path the user already listed is carried over: the merge replaces
// the whole array, so dropping them here would delete their skills from
// the next sync.
func skillsPaths(sess *emit.Session, skills []spec.Entry, skillsDir, path string, dryRun bool) []string {
	dir := filepath.ToSlash(filepath.Clean(skillsDir))
	if len(skills) == 0 || scannedSkillTrees[dir] {
		return nil
	}
	paths := sess.ExistingNestedStrings(path, "skills", "paths", dryRun)
	for _, p := range paths {
		if filepath.ToSlash(filepath.Clean(p)) == dir {
			return paths
		}
	}
	return append(paths, dir)
}

// ruleInstructions returns one `instructions` entry per rule spec: the
// project-relative path RulesDirectory writes it to, scope subdirectory
// included (see the package doc for why this lists explicit paths
// rather than a `.kilo/rules/*.md` glob).
func ruleInstructions(rules []spec.Entry, rulesDir string) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		if r.Name == "" {
			continue
		}
		dir := rulesDir
		if s := r.EffectiveScope(); s != "" {
			dir = filepath.Join(rulesDir, s)
		}
		out = append(out, filepath.ToSlash(filepath.Join(dir, r.Name+".md")))
	}
	return out
}

func buildMCPMap(mcps []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, e := range mcps {
		if e.Name == "" {
			continue
		}
		entry := buildMCPEntry(e)
		if len(entry) == 0 {
			continue
		}
		out[e.Name] = entry
	}
	return out
}

// buildMCPEntry renders one kilo.jsonc `mcp` entry. Stdio specs
// combine `command` + `args` into a single `command` array and set
// `"type": "local"`; HTTP / SSE / remote specs set `"type": "remote"`
// with a `url`/`headers` block. `environment` (not `env`) carries a
// stdio server's environment variables. An entry missing its
// transport's required field (command for stdio, url for remote) is
// dropped: there is nothing for Kilo Code to run or connect to.
//
// A spec's `disabled: true` maps to Kilo Code's own `enabled: false`
// key (B9, target-audit 2026-08-01 follow-up): the documented MCP
// example carries `"enabled": true` alongside type/command/environment/
// timeout, so the vendor concept exists under that name. Kilo Code's
// own default (enabled) needs no explicit key, matching the codex
// adapter's identical convention for its `enabled` field.
func buildMCPEntry(e spec.Entry) map[string]any {
	e.Meta = emit.ResolveMeta(e.Meta, target)
	transport, _ := e.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	out := map[string]any{}

	switch transport {
	case "stdio":
		cmd, _ := e.Meta["command"].(string)
		if cmd == "" {
			return nil
		}
		out["type"] = "local"
		out["command"] = combineCommand(cmd, e.Meta)
		if env := emit.StringMap(e.Meta["env"]); len(env) > 0 {
			out["environment"] = env
		}
	case "http", "sse", "remote":
		url, _ := e.Meta["url"].(string)
		if url == "" {
			return nil
		}
		out["type"] = "remote"
		out["url"] = url
		if oauth, ok := e.Meta["oauth"].(bool); ok && !oauth {
			out["oauth"] = false
		}
		if h := emit.StringMap(e.Meta["headers"]); len(h) > 0 {
			out["headers"] = h
		}
	default:
		return nil
	}

	if disabled, _ := e.Meta["disabled"].(bool); disabled {
		out["enabled"] = false
	}
	if timeout, ok := emit.IntField(e.Meta, "timeout"); ok {
		out["timeout"] = timeout
	}

	return out
}

// combineCommand folds Kilo Code's expected `command: [cmd, arg1,
// ...]` array out of agnostic-ai's separate `command` + `args` fields.
func combineCommand(cmd string, meta map[string]any) []string {
	parts := []string{cmd}
	parts = append(parts, emit.StringSlice(meta["args"])...)
	return parts
}

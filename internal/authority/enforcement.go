package authority

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Documentation the resolutions cite.
const (
	docSettings = "docs/site/content/docs/spec-format/settings.md"
	docAgents   = "docs/site/content/docs/spec-format/agents.md"
	docMCPs     = "docs/site/content/docs/spec-format/mcps.md"
)

func targetDoc(target string) string {
	return "docs/site/content/docs/targets/" + target + ".md"
}

// Enforcement statuses.
const (
	statusNative        = "native-rule"
	statusHook          = "hook"
	statusNotExpressed  = "not-expressed"
	statusNotApplicable = "not-applicable"
)

// effective is how one coding tool carries an operation's declaration.
type effective struct {
	effect     Effect
	resolution string
	status     string
	native     []string
	note       string
	// installs reports a decision a generated file carries, so the
	// file's installed state applies to it.
	installs bool
	// carrier is the generated file expected to carry the decision.
	// agentFile and mcpFile ask for the agent's file or the MCP config
	// to be looked up among the installed files instead.
	carrier   string
	agentFile bool
	mcpFile   bool
	// cause is the narrowing that decided on this tool, when the
	// subject's own spec decided.
	cause string
}

// ruleForm is the shape of a permission rule, which decides whether a
// tool's matching for it is documented.
type ruleForm string

const (
	formWholeTool  ruleForm = "whole-tool"
	formMCPTool    ruleForm = "mcp-tool"
	formMCPServer  ruleForm = "mcp-server"
	formShellExact ruleForm = "shell-exact"
	formShellColon ruleForm = "shell-prefix-colon"
	formShellStar  ruleForm = "shell-prefix-star"
	formShellGlob  ruleForm = "shell-glob"
	formPath       ruleForm = "path"
	formWebDomain  ruleForm = "web-domain"
	formOther      ruleForm = "other"
)

var formPhrases = map[ruleForm]string{
	formMCPServer:  "a server-wide MCP rule",
	formShellExact: "an exact command rule",
	formShellColon: "a command prefix rule",
	formShellStar:  "a command rule with a trailing wildcard",
	formShellGlob:  "a command rule with a wildcard before its end",
	formPath:       "a path pattern",
	formWebDomain:  "a domain rule",
	formOther:      "this rule form",
}

// documentedForms lists, per target, the rule forms whose matching that
// target's documentation in this repository states. Whole-tool rules
// and exact MCP tool names count as documented on every tool that
// writes them; every other form counts only where it is listed here.
//
//   - Claude Code: settings.md documents prefixes (`:*` and a trailing
//     `*`), exact commands, read and edit paths, server-wide MCP rules,
//     and WebFetch(domain:). A mid-command `*` is documented as matching
//     literally in deny, so it is left out.
//   - Kilo Code: kilo.md says each rule becomes one glob matched against
//     the tool's arguments, with prefix, exact, and path examples.
//   - OpenCode: opencode.md shows a command prefix becoming a glob.
//   - Windsurf: windsurf.md says Exec matches only the start of a
//     command, for the `:*` form.
//   - Augment: augment.md says a `:*` prefix becomes a regex anchored at
//     the start and an exact command one anchored at both ends.
//   - Codex: codex.md says ` *` and `:*` rules become prefix rules that
//     match extra arguments.
var documentedForms = map[string][]ruleForm{
	"claude":   {formShellExact, formShellColon, formShellStar, formPath, formMCPServer, formWebDomain},
	"kilo":     {formShellExact, formShellColon, formShellStar, formPath},
	"opencode": {formShellColon, formShellStar},
	"windsurf": {formShellColon},
	"augment":  {formShellColon, formShellExact},
	"codex":    {formShellColon, formShellStar},
}

// formDocumented returns the page that documents how target matches
// form, "" for forms documented everywhere, and whether it is
// documented at all.
func formDocumented(target string, form ruleForm) (string, bool) {
	if form == formWholeTool || form == formMCPTool {
		return "", true
	}
	if !slices.Contains(documentedForms[target], form) {
		return "", false
	}
	if target == "claude" {
		return docSettings, true
	}
	return targetDoc(target), true
}

// classifyRule names the form of a permission rule.
func classifyRule(rule string) ruleForm {
	names, problem := spec.CapabilityTools([]string{rule})
	if problem != "" || len(names) == 0 {
		return formOther
	}
	form := formWholeTool
	for _, name := range names {
		if f := classifyToolName(name); f != formWholeTool {
			form = f
		}
	}
	return form
}

func classifyToolName(name string) ruleForm {
	if strings.HasPrefix(name, spec.MCPToolPrefix) {
		if _, _, ok := spec.SplitMCPPermissionRule(name); ok {
			return formMCPTool
		}
		return formMCPServer
	}
	scope, arg, ok := spec.SplitPermissionRule(name)
	if !ok {
		if strings.Contains(name, "(") {
			return formOther
		}
		return formWholeTool
	}
	switch scope {
	case "Bash":
		p := strings.TrimSpace(arg)
		switch {
		case strings.HasSuffix(p, ":*") && !strings.Contains(strings.TrimSuffix(p, ":*"), "*"):
			return formShellColon
		case strings.HasSuffix(p, "*") && !strings.Contains(strings.TrimSuffix(p, "*"), "*"):
			return formShellStar
		case strings.Contains(p, "*"):
			return formShellGlob
		}
		return formShellExact
	case "Read", "Edit", "Write":
		return formPath
	case "WebFetch":
		if strings.HasPrefix(arg, "domain:") {
			return formWebDomain
		}
	}
	return formOther
}

// permissionFiles is the file each tool's permission rules land in.
var permissionFiles = map[string]string{
	"claude":   ".claude/settings.json",
	"qoder":    ".qoder/settings.json",
	"kilo":     "kilo.jsonc",
	"opencode": "opencode.json",
	"windsurf": ".devin/config.json",
	"augment":  ".augment/settings.json",
	"codex":    ".codex/rules/default.rules",
	"cursor":   ".cursor/cli.json",
	"factory":  ".factory/settings.json",
}

// readonlyHandling lists the tools whose agent files carry readonly:
// true, with what they write (agents.md, "readonly by target").
var readonlyHandling = map[string]string{
	"claude":  "disallowedTools: Write, Edit, NotebookEdit",
	"cursor":  "readonly: true",
	"factory": "tools: read-only",
}

// agentMCPServers lists the tools that write an agent's mcpServers
// names into its file (agents.md, "mcpServers support by target").
var agentMCPServers = map[string]bool{"claude": true, "junie": true, "factory": true, "qoder": true}

// disabledHandling is how each tool receives a server whose spec sets
// disabled: true (mcps.md, "disabled support by target"). A removed
// server is not written at all.
var disabledHandling = map[string]struct {
	native  string
	carrier string
	removed bool
}{
	"antigravity": {native: "disabled: true"},
	"crush":       {native: "disabled: true"},
	"factory":     {native: "disabled: true"},
	"kiro":        {native: "disabled: true"},
	"qoder":       {native: "disabled: true"},
	"windsurf":    {native: "disabled: true"},
	"codex":       {native: "enabled = false", carrier: ".codex/config.toml"},
	"kilo":        {native: `"enabled": false`, carrier: "kilo.jsonc"},
	"opencode":    {native: `"enabled": false`, carrier: "opencode.json"},
	"zed":         {native: `"enabled": false`},
	"copilot":     {native: "disabledMcpServers in .github/copilot/settings.json", carrier: ".github/copilot/settings.json"},
	"claude":      {native: "disabledMcpjsonServers in .claude/settings.json", carrier: ".claude/settings.json"},
	"cursor":      {removed: true},
	"augment":     {removed: true},
	"junie":       {removed: true},
	"trae":        {removed: true},
	"warp":        {removed: true},
}

// effectiveOn resolves how target carries the operation's declaration.
func (bd *builder) effectiveOn(target string, s *subjectInfo, action string, d *declaration) effective {
	switch {
	case d.disabled:
		return disabledOn(target)
	case d.win == nil:
		return effective{effect: EffectDefault, resolution: ResolutionNoRule, status: statusNotApplicable, note: "no rule applies; the tool's own default decides"}
	case d.win.kind == grantNarrowing:
		return bd.narrowedOn(target, s, action, d)
	case d.win.kind == grantProtected:
		return bd.protectedOn(target, d)
	}
	return bd.ruleOn(target, d)
}

func unexpressed(resolution, note string) effective {
	return effective{effect: EffectUnresolved, resolution: resolution, status: statusNotExpressed, note: note}
}

// ruleOn resolves a decision a settings rule won. The effect is claimed
// only when the adapter writes the rule and the tool's matching for its
// form is documented.
func (bd *builder) ruleOn(target string, d *declaration) effective {
	w := d.win
	label := TargetLabel(target)
	switch {
	case bd.opts.Global:
		return unexpressed(ResolutionNotExpressed, "sync --global writes no permission rules; only permissions.default-mode reaches user settings")
	case !bd.writesSettings(target):
		return unexpressed(ResolutionNotExpressed, label+" takes no settings specs, so sync writes no rule for this tool; its own defaults apply")
	case !w.rule.entry.EmitsTo(target):
		return unexpressed(ResolutionNotExpressed, "the settings spec does not reach "+label+", so sync writes no rule for this tool; its own defaults apply")
	}
	t := bd.translatePermission(target, w.list, w.rule.raw, w.rule.entry)
	if !t.Supported || len(t.Native) == 0 {
		return unexpressed(ResolutionNotExpressed, "sync writes no rule for this tool; its own defaults apply")
	}
	out := effective{
		effect:     EffectUnresolved,
		resolution: ResolutionTranslatedUnverified,
		status:     statusNative,
		native:     slices.Clone(t.Native),
		installs:   true,
		carrier:    permissionFiles[target],
	}
	form := classifyRule(w.rule.raw)
	switch {
	case len(t.Widening) > 0:
		out.note = "the written rule grants more than declared: " + strings.Join(t.Widening, "; ")
	case t.Override != "":
		out.note = t.Override + " replaces the written rule, and this view does not read it"
	case target == "claude" && form == formShellGlob:
		out.note = "Claude Code matches a mid-command * in a deny rule literally, so it blocks nothing, and an allow rule also approves options placed at the * (" + docSettings + ")"
	default:
		cite, ok := formDocumented(target, form)
		if !ok {
			out.note = "this repository does not document how " + label + " matches " + formPhrases[form]
			break
		}
		out.effect, out.resolution = d.declared, ResolutionTranslated
		if cite != "" {
			out.note = "matching documented in " + cite
		}
	}
	return out
}

// protectedOn resolves a decision a protected path won, following the
// protected-paths table in settings.md.
func (bd *builder) protectedOn(target string, d *declaration) effective {
	p := d.win.protect
	label := TargetLabel(target)
	switch {
	case bd.opts.Global:
		return unexpressed(ResolutionNotExpressed, "protected paths are relative to a project, so sync --global does not write them")
	case !p.entry.EmitsTo(target):
		return unexpressed(ResolutionNotExpressed, "the settings spec does not reach "+label+", so sync writes no guard for this tool")
	}
	switch target {
	case "claude":
		note := "in permissions." + p.decision + " of .claude/settings.json (" + targetDoc(target) + ")"
		if p.decision == spec.ProtectAsk {
			note += "; an ask rule guards edit tools only"
		}
		return effective{effect: d.declared, resolution: ResolutionTranslated, status: statusNative, native: []string{spec.ProtectEditRule(p.path)}, installs: true, carrier: permissionFiles[target], note: note}
	case "codex", "gemini":
		script := "." + target + "/hooks/agnostic-ai-protect.sh"
		out := effective{effect: EffectDenied, resolution: ResolutionHook, status: statusHook, native: []string{script}, installs: true, carrier: script,
			note: "the generated hook blocks a matching edit (" + targetDoc(target) + ")"}
		if p.decision == spec.ProtectAsk {
			out.note = "the generated hook blocks the edit either way and tells the agent to ask the user, since a " + label + " hook cannot ask (" + targetDoc(target) + ")"
		}
		return out
	case "cursor":
		if p.decision != spec.ProtectDeny {
			return unexpressed(ResolutionNotExpressed, "Cursor does not enforce ask: its CLI has no ask list ("+targetDoc(target)+")")
		}
		native := []string{"Write(" + p.path + ") in .cursor/cli.json"}
		if !strings.HasSuffix(p.path, allFiles) {
			native = append(native, "Write("+p.path+"/**) in .cursor/cli.json")
		}
		return effective{effect: EffectDenied, resolution: ResolutionTranslated, status: statusNative, native: native, installs: true, carrier: permissionFiles[target],
			note: "guards the Cursor CLI only, not the IDE agent (" + targetDoc(target) + ")"}
	}
	return unexpressed(ResolutionNotExpressed, label+" has no native edit guard sync writes, so protected paths are advisory there")
}

// narrowedOn resolves a decision the subject's own spec won. A
// narrowing holds only on a tool that writes it, so each narrowing is
// tried in turn and the first one the tool carries decides.
func (bd *builder) narrowedOn(target string, s *subjectInfo, action string, d *declaration) effective {
	label := TargetLabel(target)
	if !s.entry.EmitsTo(target) {
		out := unexpressed(ResolutionAgentScope, "the agent spec does not reach "+label)
		out.cause = d.win.grant.Note
		return out
	}
	var first effective
	for i, c := range d.narrow {
		eff := bd.narrowOn(target, s, action, c, d.capability)
		eff.cause = c.grant.Note
		if eff.effect == EffectUnavailable {
			return eff
		}
		if i == 0 {
			first = eff
		}
	}
	return first
}

// narrowOn resolves one narrowing on target. The narrowing holds only
// where the adapter translates agent tools for the capability, and, for
// readonly and mcpServers, where agents.md says the tool's agent file
// carries that field.
func (bd *builder) narrowOn(target string, s *subjectInfo, action string, c candidate, capability string) effective {
	label := TargetLabel(target)
	t := bd.translateAgent(target, capability, *s.entry)
	unavailable := func(native []string, note string) effective {
		if !t.Supported {
			return unexpressed(ResolutionAgentScope, note+", but the adapter translates no agent tool for "+capability+", so this view does not claim the narrowing holds")
		}
		return effective{effect: EffectUnavailable, resolution: ResolutionAgentScope, status: statusNative, native: native, installs: true, agentFile: true, note: note}
	}
	switch c.list {
	case "mcpServers":
		if agentMCPServers[target] {
			return unavailable(nil, "the agent file's mcpServers list leaves the server out ("+docAgents+")")
		}
		return unexpressed(ResolutionAgentScope, label+" drops an agent's mcpServers list, so the narrowing does not reach it ("+docAgents+")")
	case "readonly":
		native, ok := readonlyHandling[target]
		switch {
		case target == "codex":
			return unexpressed(ResolutionAgentScope, "Codex ignores sandbox_mode in an agent file, so readonly does not reach it ("+targetDoc(target)+")")
		case !ok:
			return unexpressed(ResolutionAgentScope, label+" does not honor readonly: true ("+docAgents+")")
		case target == "claude" && action == capDelete:
			return unexpressed(ResolutionAgentScope, "Claude Code has no delete tool, so readonly leaves deletion through the shell open ("+docAgents+")")
		}
		return unavailable([]string{native}, "the agent file carries "+native+" ("+docAgents+")")
	}
	switch {
	case t.Override != "":
		return unexpressed(ResolutionAgentScope, t.Override+" replaces the tool list on "+label+", and this view does not read it")
	case t.Supported:
		return unavailable(nil, "the agent file's tool list leaves this out")
	case bd.translateAgent(target, capRead, *s.entry).Supported:
		return unexpressed(ResolutionAgentScope, label+" has no native tool for "+capability+", so leaving it out of the tool list does not close it")
	}
	return unexpressed(ResolutionAgentScope, "this tool reads no per-agent tool list")
}

// disabledOn resolves an operation on a disabled server.
func disabledOn(target string) effective {
	label := TargetLabel(target)
	h, ok := disabledHandling[target]
	switch {
	case !ok:
		return unexpressed(ResolutionNotExpressed, docMCPs+" does not list how "+label+" handles disabled: true")
	case h.removed:
		return effective{effect: EffectUnavailable, resolution: ResolutionDisabled, status: statusNative, native: []string{"server left out of the MCP config"}, installs: true, mcpFile: true,
			note: "sync leaves the server out of " + label + "'s MCP config, so the tool never starts it (" + docMCPs + ")"}
	}
	return effective{effect: EffectUnavailable, resolution: ResolutionDisabled, status: statusNative, native: []string{h.native}, installs: true, carrier: h.carrier, mcpFile: h.carrier == "",
		note: "written as " + h.native + " (" + docMCPs + ")"}
}

// writesSettings reports whether the target's adapter takes settings
// specs at all.
func (bd *builder) writesSettings(target string) bool {
	a, ok := adapters.Get(target)
	return ok && slices.Contains(a.Capabilities(), spec.KindSettings)
}

func (bd *builder) translatePermission(target, list, rule string, entry spec.Entry) adapters.CapabilityTranslation {
	key := target + "\x00" + list + "\x00" + rule + "\x00" + entry.Path
	if t, ok := bd.permCache[key]; ok {
		return t
	}
	t := adapters.TranslatePermissionCapabilityIn(target, list, rule, entry, bd.b.Settings, bd.cfg)
	bd.permCache[key] = t
	return t
}

func (bd *builder) translateAgent(target, rule string, entry spec.Entry) adapters.CapabilityTranslation {
	key := target + "\x00" + rule + "\x00" + entry.Name
	if t, ok := bd.agentCache[key]; ok {
		return t
	}
	t := adapters.TranslateAgentCapabilityIn(target, rule, entry)
	bd.agentCache[key] = t
	return t
}

// permissionSupport says how target takes permission rules.
func (bd *builder) permissionSupport(target string) (string, string) {
	label := TargetLabel(target)
	if bd.opts.Global {
		return "none", "sync --global writes no permission rules; only permissions.default-mode reaches user settings."
	}
	t := adapters.TranslatePermissionCapabilityIn(target, "allow", capShell, spec.Entry{Kind: spec.KindSettings}, bd.b.Settings, bd.cfg)
	switch {
	case bd.writesSettings(target) && t.Supported:
		return "native-rules", ""
	case target == "codex" || target == "gemini":
		return "hooks-only", label + " enforces protected paths through a generated hook; sync writes no permission rules for it."
	}
	return "none", "Sync writes no permission rules or hooks for " + label + "; its own defaults decide."
}

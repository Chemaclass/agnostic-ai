package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// agentSkillsDescriptionLimit is the Agent Skills specification's cap on
// a skill description (agentskills.io/specification: "Max 1024
// characters").
const agentSkillsDescriptionLimit = 1024

// instructionByteCap is a vendor's published limit on one instructions
// file. Past it the tool drops or truncates text, so the budget finding
// names it for that target.
type instructionByteCap struct {
	bytes  int
	format string
}

var instructionByteCaps = map[string]instructionByteCap{
	// developers.openai.com/codex/guides/agents-md: Codex "stops adding
	// files once the combined size reaches the limit defined by
	// `project_doc_max_bytes` (32 KiB by default)".
	"codex": {bytes: 32 * 1024, format: "Codex stops reading AGENTS.md at %d bytes (project_doc_max_bytes); this is %d bytes."},
	// antigravity.google/docs/rules: "Antigravity truncates any single
	// rule file that exceeds 24,000 bytes", and AGENTS.md and GEMINI.md
	// are rule files there.
	"antigravity": {bytes: 24000, format: "Antigravity truncates a rule file past %d bytes; this is %d bytes."},
}

// wordCount is one named source of always-loaded words.
type wordCount struct {
	name  string
	words int
}

// sessionLoad is the text one target loads at the start of every
// session: its instructions file, broken down by the layers that feed
// it, and the parts it loads beside that file.
type sessionLoad struct {
	target string
	// path is where the finding points: the measured file, or the
	// source the user edits when there is none.
	path string
	// file labels the instructions file in the message; empty when the
	// target loads none.
	file       string
	fileWords  int
	fileBytes  int
	fileLayers []wordCount
	parts      []wordCount
}

func (l sessionLoad) total() int {
	n := l.fileWords
	for _, p := range l.parts {
		n += p.words
	}
	return n
}

func (l *sessionLoad) setFile(label, text string, layers []instructionLayer) {
	l.file, l.fileWords, l.fileBytes = label, wordsIn(text), len(text)
	for _, layer := range layers {
		l.fileLayers = append(l.fileLayers, wordCount{layer.Name, wordsIn(layer.Text)})
	}
}

func (l *sessionLoad) add(name string, words int) {
	if words > 0 {
		l.parts = append(l.parts, wordCount{name, words})
	}
}

func wordsIn(text string) int {
	return len(strings.Fields(text))
}

// lintBudgetFindings checks the scope's always-loaded text against the
// budgets its config sets (LINT011, LINT012).
func lintBudgetFindings(scope checkScope) ([]lintFinding, error) {
	var (
		budgets config.LintConfig
		loads   []sessionLoad
		err     error
	)
	source := adapters.AgnosticEntryPointPath
	if scope.global {
		if budgets, err = loadGlobalLint(scope.source); err != nil {
			return nil, err
		}
		source = filepath.Join(scope.source, "AGNOSTIC_AI.md")
		loads, err = globalSessionLoads(scope.source, scope.targets, scope.bundle)
	} else {
		budgets = scope.cfg.Lint
		loads, err = projectSessionLoads(scope.cfg, scope.support, scope.bundle)
	}
	var findings []lintFinding
	if err != nil {
		// sync reports the same error; lint keeps its other findings.
		findings = append(findings, lintFinding{
			Code:     "LINT011",
			Severity: lintWarn,
			Path:     source,
			Message:  fmt.Sprintf("cannot measure the always-loaded instructions: %v", err),
		})
	} else {
		findings = lintInstructionBudget(loads, budgets.InstructionsWordBudget())
	}
	return append(findings, lintDescriptionBudget(scope.bundle, budgets.DescriptionCharBudget())...), nil
}

// projectSessionLoads measures each configured target from the
// entry-point files sync writes, so the count is the file's own.
func projectSessionLoads(cfg *config.Config, support kindSupport, b spec.Bundle) ([]sessionLoad, error) {
	sess := adapters.NewSession()
	sess.StartCapture()
	body, err := resolveAgnosticBody(sess, cfg, false)
	sess.StopCapture()
	if err != nil {
		return nil, err
	}
	files, err := renderEntryPointFiles(cfg, b, cfg.Targets, body)
	if err != nil {
		return nil, err
	}
	fileOf := map[string]entryPointFile{}
	for _, f := range files {
		// A user-owned file holds whatever the user wrote, not this.
		if cfg.IsUnmanaged(f.Path) {
			continue
		}
		for _, t := range f.Readers {
			fileOf[t] = f
		}
	}
	loads := make([]sessionLoad, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		load := sessionLoad{target: t, path: adapters.AgnosticEntryPointPath}
		if f, ok := fileOf[t]; ok {
			load.path = f.Path
			load.setFile(f.Path, f.Content, f.Layers)
		}
		load.add("always-on rule files", alwaysOnRuleWords(cfg, b, t))
		addDescriptions(&load, support, b.For(t))
		loads = append(loads, load)
	}
	return loads, nil
}

// globalSessionLoads measures each target sync --global writes. Every
// target with an instructions file gets the same managed block.
func globalSessionLoads(source string, targets []string, b spec.Bundle) ([]sessionLoad, error) {
	layers, err := globalInstructionLayers(source, b.Rules)
	if err != nil {
		return nil, err
	}
	texts := make([]string, 0, len(layers))
	for _, l := range layers {
		texts = append(texts, l.Text)
	}
	instructions := strings.Join(texts, "\n\n")
	support := globalKindSupport()
	loads := make([]sessionLoad, 0, len(targets))
	for _, t := range targets {
		load := sessionLoad{target: t, path: filepath.Join(source, "AGNOSTIC_AI.md")}
		g := globalTargets[t]
		switch {
		case g.instructions != "":
			load.setFile("instructions", instructions, layers)
		case g.rules != "":
			var words int
			for _, r := range b.Rules {
				words += wordsIn(r.Body)
			}
			load.add("always-on rule files", words)
		}
		addDescriptions(&load, support, b.For(t))
		loads = append(loads, load)
	}
	return loads, nil
}

// addDescriptions counts the skill and agent descriptions a target
// lists in every session so the model can pick one.
func addDescriptions(load *sessionLoad, support kindSupport, b spec.Bundle) {
	if _, ok := support[spec.KindSkill][load.target]; ok {
		load.add("skill descriptions", descriptionWords(b.Skills, load.target))
	}
	if _, ok := support[spec.KindAgent][load.target]; ok {
		load.add("agent descriptions", descriptionWords(b.Agents, load.target))
	}
}

func descriptionWords(entries []spec.Entry, target string) int {
	n := 0
	for _, e := range entries {
		desc, _ := adapters.ResolveMeta(e.Meta, target)["description"].(string)
		n += wordsIn(desc)
	}
	return n
}

// legacyRulesFileScoped names the targets whose legacy rules file keeps
// only always-on rules; scoped rules still get their own files.
var legacyRulesFileScoped = map[string]bool{"copilot": true}

// alwaysOnRuleWords counts the rules a target loads from its own rule
// files in every session. A legacy concatenated rules file and rule
// files `@`-imported into the entry point load whole; otherwise the
// adapter that renders each rule's activation decides.
func alwaysOnRuleWords(cfg *config.Config, b spec.Bundle, target string) int {
	legacyWhole := adapters.HasLegacyRulesFile(cfg, target) && !legacyRulesFileScoped[target]
	whole := legacyWhole || adapters.ImportsRulesIntoEntryPoint(cfg, target)
	n := 0
	for _, r := range adapters.EntryPointRules(b, target).Rules {
		if whole || adapters.AlwaysOnRule(target, r) {
			n += wordsIn(r.Body)
		}
	}
	return n
}

// lintInstructionBudget reports the targets whose always-loaded text
// passes the word budget or the target's published byte cap (LINT011,
// warn). Targets with the same file and the same numbers share one
// line, so a shared AGENTS.md reports once.
func lintInstructionBudget(loads []sessionLoad, budget int) []lintFinding {
	type group struct {
		path, detail string
		total        int
		targets      []string
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, l := range loads {
		limit, capped := instructionByteCaps[l.target]
		overCap := capped && l.file != "" && l.fileBytes > limit.bytes
		if l.total() <= budget && !overCap {
			continue
		}
		detail := describeLoad(l, budget)
		if overCap {
			detail += " " + fmt.Sprintf(limit.format, limit.bytes, l.fileBytes)
		}
		key := l.path + "\x00" + detail
		g, ok := byKey[key]
		if !ok {
			g = &group{path: l.path, detail: detail, total: l.total()}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.targets = append(g.targets, l.target)
	}
	out := make([]lintFinding, 0, len(groups))
	for _, g := range groups {
		verb := "load"
		if len(g.targets) == 1 {
			verb = "loads"
		}
		out = append(out, lintFinding{
			Code:     "LINT011",
			Severity: lintWarn,
			Path:     g.path,
			Message:  fmt.Sprintf("%s %s %d words every session: %s", strings.Join(g.targets, ", "), verb, g.total, g.detail),
		})
	}
	return out
}

func describeLoad(l sessionLoad, budget int) string {
	var parts []string
	if l.file != "" {
		file := fmt.Sprintf("%s %d", l.file, l.fileWords)
		var layers []string
		for _, layer := range l.fileLayers {
			if layer.words > 0 {
				layers = append(layers, fmt.Sprintf("%s %d", layer.name, layer.words))
			}
		}
		if len(layers) > 0 {
			file += " (" + strings.Join(layers, ", ") + ")"
		}
		parts = append(parts, file)
	}
	for _, p := range l.parts {
		parts = append(parts, fmt.Sprintf("%s %d", p.name, p.words))
	}
	return fmt.Sprintf("%s; budget %d (lint.instructions-words).", strings.Join(parts, ", "), budget)
}

// lintDescriptionBudget flags a skill or agent description longer than
// the budget, and a skill description past the Agent Skills limit
// whatever the budget (LINT012, warn). Targets list every description
// in every session, so a long one costs context even when it never
// runs. Each `x-<target>.description` is checked too, since that is the
// text its target lists.
func lintDescriptionBudget(b spec.Bundle, budget int) []lintFinding {
	entries := make([]spec.Entry, 0, len(b.Skills)+len(b.Agents))
	entries = append(entries, b.Skills...)
	entries = append(entries, b.Agents...)
	var out []lintFinding
	for _, e := range entries {
		for _, key := range descriptionKeys(e.Meta) {
			desc := descriptionAt(e.Meta, key)
			n := utf8.RuneCountInString(strings.TrimSpace(desc))
			overBudget := n > budget
			overSpec := e.Kind == spec.KindSkill && n > agentSkillsDescriptionLimit
			if !overBudget && !overSpec {
				continue
			}
			message := fmt.Sprintf("%s %s is %d characters", e.Kind, key, n)
			switch {
			case overBudget && overSpec:
				message += fmt.Sprintf("; budget %d (lint.description-chars). The Agent Skills spec allows at most %d.", budget, agentSkillsDescriptionLimit)
			case overBudget:
				message += fmt.Sprintf("; budget %d (lint.description-chars).", budget)
			default:
				message += fmt.Sprintf("; the Agent Skills spec allows at most %d.", agentSkillsDescriptionLimit)
			}
			out = append(out, lintFinding{Code: "LINT012", Severity: lintWarn, Path: e.Path, Message: message})
		}
	}
	return out
}

// descriptionKeys names every description a spec carries, sorted:
// `description`, then `x-<target>.description` for each override.
func descriptionKeys(meta map[string]any) []string {
	var keys []string
	if _, ok := meta["description"].(string); ok {
		keys = append(keys, "description")
	}
	var overrides []string
	for k, v := range meta {
		if x, ok := v.(map[string]any); ok && strings.HasPrefix(k, "x-") {
			if _, ok := x["description"].(string); ok {
				overrides = append(overrides, k+".description")
			}
		}
	}
	slices.Sort(overrides)
	return append(keys, overrides...)
}

// descriptionAt reads the description descriptionKeys named key.
func descriptionAt(meta map[string]any, key string) string {
	if xKey, ok := strings.CutSuffix(key, ".description"); ok {
		x, _ := meta[xKey].(map[string]any)
		desc, _ := x["description"].(string)
		return desc
	}
	desc, _ := meta[key].(string)
	return desc
}

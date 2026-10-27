package adapters

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// commandSkillPreambleLines is how far the preamble and its blank line
// push the command body down.
const commandSkillPreambleLines = 2

// claudeSkillsReaders also read `.claude/skills`, beside their own tree:
// copilot (copilot.go otherSkillsDirs) and cline (cline.go).
var claudeSkillsReaders = map[string]bool{"cline": true, "copilot": true}

// rulesFileSkillTargets write skills only into an always-loaded rules
// file, where a command would run in every session.
var rulesFileSkillTargets = map[string]bool{"aider": true}

// commandSkillFields are the command fields a skill carries over.
var commandSkillFields = map[string]bool{
	"name": true, "description": true, "disable-model-invocation": true,
	"target": true, "targets": true, "target-exclude": true, "targets-exclude": true,
}

// specFileRenderer is implemented by adapters that render a spec only
// under some config, such as Codex commands behind commands-dir.
type specFileRenderer interface {
	RendersSpecFile(spec.Entry, *config.Config, string) bool
}

// WrittenFromCommand reports whether a SKILL.md is a command sync wrote
// as a skill, so import leaves it to the command spec.
func WrittenFromCommand(skillMD string) bool {
	return strings.Contains(skillMD, emit.CommandSkillPreamble)
}

// WritesCommandsAsSkills reports whether target writes commands as
// skills the user runs by name: it writes skill folders, no commands,
// and keeps a skill out of the model's reach. Where the model could start
// a command on its own, the command stays off with the usual note.
func WritesCommandsAsSkills(cfg *config.Config, target string) bool {
	if nativeCommands(cfg, target) || !renders(cfg, target, spec.KindSkill) {
		return false
	}
	switch manualOnlyKeeper(target) {
	case keepsInSidecar:
		return true
	case keepsInSkillMD:
		// Another writer of the same SKILL.md files makes them agree on a
		// copy without the field.
		return !skillsDirShared(cfg, target)
	}
	return false
}

type manualOnlyPlace int

const (
	dropsManualOnly manualOnlyPlace = iota
	keepsInSkillMD
	keepsInSidecar
)

var manualOnlyKeepers = struct {
	once    sync.Once
	targets map[string]manualOnlyPlace
}{}

// manualOnlyKeeper reports where target's skill output keeps
// disable-model-invocation: in SKILL.md, in a file beside it, or not at
// all. It renders a probe skill, so it follows what each adapter writes.
func manualOnlyKeeper(target string) manualOnlyPlace {
	manualOnlyKeepers.once.Do(func() {
		manualOnlyKeepers.targets = map[string]manualOnlyPlace{}
		plain := spec.Entry{Kind: spec.KindSkill, Name: "probe", Meta: map[string]any{"name": "probe", "description": "Probe."}}
		manual := plain
		manual.Meta = maps.Clone(plain.Meta)
		manual.Meta["disable-model-invocation"] = true
		for _, name := range Names() {
			if rulesFileSkillTargets[name] || !slices.Contains(registry[name].Capabilities(), spec.KindSkill) {
				continue
			}
			md, err := RenderSkillMarkdown(name, manual, false)
			if err != nil {
				continue
			}
			if renderedManualOnly(md) {
				manualOnlyKeepers.targets[name] = keepsInSkillMD
				continue
			}
			with, err := RenderSkillSidecars(name, manual)
			if err != nil {
				continue
			}
			without, err := RenderSkillSidecars(name, plain)
			if err == nil && !maps.Equal(with, without) {
				manualOnlyKeepers.targets[name] = keepsInSidecar
			}
		}
	})
	return manualOnlyKeepers.targets[target]
}

// nativeCommands reports whether target writes commands as commands.
func nativeCommands(cfg *config.Config, target string) bool {
	a, ok := Get(target)
	if !ok || !slices.Contains(a.Capabilities(), spec.KindCommand) {
		return false
	}
	return renders(cfg, target, spec.KindCommand)
}

// renders reports whether target writes specs of kind under cfg.
func renders(cfg *config.Config, target string, kind spec.Kind) bool {
	a, ok := Get(target)
	if !ok {
		return false
	}
	if r, ok := a.(specFileRenderer); ok && cfg != nil {
		return r.RendersSpecFile(spec.Entry{Kind: kind}, cfg, "")
	}
	return true
}

// skillsDirShared reports whether another enabled target writes
// target's skills directory.
func skillsDirShared(cfg *config.Config, target string) bool {
	if cfg == nil {
		return false
	}
	dir := filepath.Clean(emit.VarsFor(cfg, target)[emit.VarSkillsDir])
	for _, t := range cfg.Targets {
		if t != target && filepath.Clean(emit.VarsFor(cfg, t)[emit.VarSkillsDir]) == dir {
			return true
		}
	}
	return false
}

// withCommandsAsSkills returns b with its commands moved into its skills
// when target writes commands as skills. A skill of the same name keeps
// its folder.
func withCommandsAsSkills(b spec.Bundle, cfg *config.Config, target string) spec.Bundle {
	if len(b.Commands) == 0 || !WritesCommandsAsSkills(cfg, target) {
		return b
	}
	// Folder names can differ only in case on a case-insensitive disk.
	// A scoped skill counts too: Codex would list both in its scope.
	skills := map[string]bool{}
	for _, s := range b.Skills {
		skills[strings.ToLower(s.Name)] = true
	}
	argumentHint := slices.Contains(SkillFieldKeepers("argument-hint"), target)
	var clashes, handWritten []string
	dropped := map[string]int{}
	converted := make([]spec.Entry, 0, len(b.Commands))
	for _, c := range b.Commands {
		if skills[strings.ToLower(c.Name)] {
			clashes = append(clashes, c.Name)
			emit.NoteEntryOmitted(target, spec.KindCommand, c.Name)
			continue
		}
		if handWrittenSkill(cfg, target, c.Name) {
			handWritten = append(handWritten, c.Name)
			emit.NoteEntryOmitted(target, spec.KindCommand, c.Name)
			continue
		}
		converted = append(converted, commandSkill(c, argumentHint, dropped))
	}
	for field, count := range dropped {
		emit.NoteFieldNoOp(target, spec.KindCommand, field, count, "the command is written as a skill, which has no such field")
	}
	emit.NoteSurfaceGap(target, spec.KindCommand, len(clashes), "as a skill",
		"the skill spec of the same name takes its folder: "+strings.Join(clashes, ", "))
	emit.NoteSurfaceGap(target, spec.KindCommand, len(handWritten), "as a skill",
		"a skill sync did not write already has its folder: "+strings.Join(handWritten, ", "))
	emit.NoteSurfaceGap(target, spec.KindCommand, len(converted), "as a command",
		"written as a skill the user runs by name")
	noteCommandSkillReaders(cfg, target, converted)
	b.Skills = append(slices.Clone(b.Skills), converted...)
	b.Commands = nil
	return b
}

// handWrittenSkill reports whether a skill folder named name that sync
// did not write sits in a skills directory target reads, which a
// command written as a skill must not replace.
func handWrittenSkill(cfg *config.Config, target, name string) bool {
	dirs := []string{emit.VarsFor(cfg, target)[emit.VarSkillsDir]}
	if agentsSkillsReaders[target] {
		dirs = append(dirs, sharedAgentsSkillsDir)
	}
	if claudeSkillsReaders[target] {
		dirs = append(dirs, ".claude/skills")
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		data, err := emit.ReadFile(filepath.Join(filepath.FromSlash(dir), name, "SKILL.md"))
		if err == nil && !header.Has(string(data)) && !WrittenFromCommand(string(data)) {
			return true
		}
	}
	return false
}

// invocationFields decide whether the model may start a skill. A
// command is the user's to run, so an x-<target> value for one of them
// is left out of the skill.
var invocationFields = []string{"disable-model-invocation", "triggers", "user-invocable"}

func withoutInvocationFields(custom map[string]any, prefix string, dropped map[string]int) map[string]any {
	out := make(map[string]any, len(custom))
	for k, v := range custom {
		if slices.Contains(invocationFields, k) {
			dropped[prefix+"."+k]++
			continue
		}
		out[k] = v
	}
	return out
}

// commandSkill is command c as a top-level skill only the user starts.
// dropped counts the fields left behind.
func commandSkill(c spec.Entry, argumentHint bool, dropped map[string]int) spec.Entry {
	s := c
	s.Kind = spec.KindSkill
	s.Scope = ""
	s.AssetDir = ""
	s.Meta = map[string]any{}
	s.MetaKeys = nil
	keep := func(k string) bool {
		return commandSkillFields[k] || k == "argument-hint" && argumentHint || strings.HasPrefix(k, emit.XPrefix)
	}
	for _, k := range c.MetaKeys {
		if v, ok := c.Meta[k]; ok && keep(k) {
			s.Meta[k] = v
			s.MetaKeys = append(s.MetaKeys, k)
		}
	}
	for k, v := range c.Meta {
		switch {
		case !keep(k):
			dropped[k]++
		case !slices.Contains(s.MetaKeys, k):
			s.Meta[k] = v
			s.MetaKeys = append(s.MetaKeys, k)
		}
	}
	for k, v := range s.Meta {
		if custom, ok := v.(map[string]any); ok && strings.HasPrefix(k, emit.XPrefix) {
			s.Meta[k] = withoutInvocationFields(custom, k, dropped)
		}
	}
	set := func(key string, value any) {
		if _, ok := s.Meta[key]; !ok {
			s.Meta[key] = value
			s.MetaKeys = append(s.MetaKeys, key)
		}
	}
	set("name", c.Name)
	set("description", "The "+c.Name+" command.")
	set("disable-model-invocation", true)
	s.Meta["disable-model-invocation"] = true
	s.Body = emit.CommandSkillPreamble + "\n\n" + c.Body
	// Without frontmatter no line maps the preamble onto the file, so
	// notes name the path alone.
	s.BodyLine = max(c.BodyLine-commandSkillPreambleLines, 0)
	return s
}

// noteCommandSkillReaders notes each other enabled target that also
// reads the skills directory target writes and has no user-run copy of
// its own: one with native commands lists a command twice, and one
// without can let its model start the command. Sync cannot hide that
// copy. Copilot's own .github/skills copy wins over .agents/skills.
func noteCommandSkillReaders(cfg *config.Config, target string, skills []spec.Entry) {
	if cfg == nil || len(skills) == 0 {
		return
	}
	dir := filepath.Clean(emit.VarsFor(cfg, target)[emit.VarSkillsDir])
	if dir == "." {
		return
	}
	for _, t := range cfg.Targets {
		sameDir := filepath.Clean(emit.VarsFor(cfg, t)[emit.VarSkillsDir]) == dir
		reads := sameDir || agentsSkillsReaders[t] && filepath.ToSlash(dir) == sharedAgentsSkillsDir
		if t == target || !reads {
			continue
		}
		ownCopy, native := WritesCommandsAsSkills(cfg, t), nativeCommands(cfg, t)
		var twice, startable []string
		for _, s := range skills {
			switch {
			case ownCopy && s.EmitsTo(t):
			case native && s.EmitsTo(t):
				twice = append(twice, s.Name)
			default:
				startable = append(startable, s.Name)
			}
		}
		where := t + " also reads " + filepath.ToSlash(dir) + ", so "
		if len(twice) > 0 {
			emit.NoteProject(where + "it lists " + strings.Join(twice, ", ") + " both as its own command and as the skill written there for " + target)
		}
		if len(startable) > 0 {
			emit.NoteProject(where + "its model can start " + strings.Join(startable, ", ") + " on its own from the skill written there for " + target)
		}
	}
}

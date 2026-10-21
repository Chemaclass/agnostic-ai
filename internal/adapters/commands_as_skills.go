package adapters

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// commandSkillPreamble opens each command written as a skill. It asks
// for what the manual-only flag does where a tool drops the flag, and
// marks the file so import leaves it to the command spec.
const commandSkillPreamble = "This skill is a command. Run it only when the user calls it by name, " +
	"and treat the text they type after the name as its arguments."

// rulesFileSkillTargets write skills only into an always-loaded rules
// file, where a command would run in every session.
var rulesFileSkillTargets = map[string]bool{"aider": true}

// specFileRenderer is implemented by adapters that render a spec only
// under some config, such as Codex commands behind commands-dir.
type specFileRenderer interface {
	RendersSpecFile(spec.Entry, *config.Config, string) bool
}

// WrittenFromCommand reports whether a SKILL.md is a command sync wrote
// as a skill, so import leaves it to the command spec.
func WrittenFromCommand(skillMD string) bool {
	return strings.Contains(skillMD, commandSkillPreamble)
}

// WritesCommandsAsSkills reports whether target writes commands as
// skills the user runs by name: it writes skill folders, no commands,
// and keeps a skill out of the model's reach. Where the model could start
// a command on its own, the command stays off with the usual note.
func WritesCommandsAsSkills(cfg *config.Config, target string) bool {
	return keepsManualOnly(target) && !nativeCommands(cfg, target)
}

var manualOnlyKeepers = struct {
	once    sync.Once
	targets map[string]bool
}{}

// keepsManualOnly reports whether target's skill output changes with
// disable-model-invocation, in SKILL.md or a file beside it. It renders
// a probe skill, so the list cannot drift from what each adapter writes.
func keepsManualOnly(target string) bool {
	manualOnlyKeepers.once.Do(func() {
		manualOnlyKeepers.targets = map[string]bool{}
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
				manualOnlyKeepers.targets[name] = true
				continue
			}
			with, err := RenderSkillSidecars(name, manual)
			if err != nil {
				continue
			}
			without, err := RenderSkillSidecars(name, plain)
			if err == nil && !maps.Equal(with, without) {
				manualOnlyKeepers.targets[name] = true
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
	if r, ok := a.(specFileRenderer); ok {
		return r.RendersSpecFile(spec.Entry{Kind: spec.KindCommand}, cfg, "")
	}
	return true
}

// withCommandsAsSkills returns b with its commands moved into its skills
// when target writes commands as skills. A skill of the same name keeps
// its folder.
func withCommandsAsSkills(b spec.Bundle, cfg *config.Config, target string) spec.Bundle {
	if len(b.Commands) == 0 || !WritesCommandsAsSkills(cfg, target) {
		return b
	}
	skills := map[string]bool{}
	for _, s := range b.Skills {
		skills[s.Name] = true
	}
	var clashes []string
	converted := make([]spec.Entry, 0, len(b.Commands))
	for _, c := range b.Commands {
		if skills[c.Name] {
			clashes = append(clashes, c.Name)
			emit.NoteEntryOmitted(target, spec.KindCommand, c.Name)
			continue
		}
		converted = append(converted, commandSkill(c))
	}
	emit.NoteSurfaceGap(target, spec.KindCommand, len(clashes), "as a skill",
		"the skill spec of the same name takes its folder: "+strings.Join(clashes, ", "))
	emit.NoteSurfaceGap(target, spec.KindCommand, len(converted), "as a command",
		"written as a skill the user runs by name")
	noteCommandSkillReaders(cfg, target, converted)
	b.Skills = append(slices.Clone(b.Skills), converted...)
	b.Commands = nil
	return b
}

// commandSkill is command c as a skill only the user starts, unless c
// sets disable-model-invocation itself.
func commandSkill(c spec.Entry) spec.Entry {
	s := c
	s.Kind = spec.KindSkill
	s.AssetDir = ""
	s.Meta = make(map[string]any, len(c.Meta)+2)
	for k, v := range c.Meta {
		s.Meta[k] = v
	}
	s.MetaKeys = slices.Clone(c.MetaKeys)
	set := func(key string, value any) {
		if _, ok := s.Meta[key]; ok {
			return
		}
		s.Meta[key] = value
		if !slices.Contains(s.MetaKeys, key) {
			s.MetaKeys = append(s.MetaKeys, key)
		}
	}
	set("name", c.Name)
	set("description", "The "+c.Name+" command.")
	set("disable-model-invocation", true)
	s.Body = commandSkillPreamble + "\n\n" + c.Body
	return s
}

// noteCommandSkillReaders notes each other enabled target that also
// reads the skills directory target writes and does not write the same
// files: one with native commands lists a command twice, and one without
// can let its model start the command. Sync cannot hide that copy.
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
		if t == target || !reads || sameDir && WritesCommandsAsSkills(cfg, t) {
			continue
		}
		var names []string
		for _, s := range skills {
			if s.EmitsTo(t) {
				names = append(names, s.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		effect := "its model can start " + strings.Join(names, ", ") + " on its own from the skill written there for " + target
		if nativeCommands(cfg, t) {
			effect = "it lists " + strings.Join(names, ", ") + " both as its own command and as the skill written there for " + target
		}
		emit.NoteProject(t + " also reads " + filepath.ToSlash(dir) + ", so " + effect)
	}
}

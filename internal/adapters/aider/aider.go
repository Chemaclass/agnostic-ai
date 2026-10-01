// Package aider emits Aider configs.
//
// The project-root `CONVENTIONS.md` is written centrally by `sync`
// as a slim pointer to the source specs (one body shared with every
// other target's entry-point file). When `outputs.aider.rules-file`
// is set, the adapter falls back to the legacy concatenated layout
// at that path so users on older workflows keep their behavior.
//
// When `outputs.aider.conf-file` is set, the adapter additionally
// writes (or merges into) Aider's project config file at that path so
// the conventions document auto-loads. Optional `outputs.aider.model`
// and `outputs.aider.weak-model` keys propagate into the same file.
// Unrelated keys in a pre-existing config are preserved.
//
// Ignore entries write `.aiderignore` in the project root, Aider's own
// default: "Specify the aider ignore file (default: .aiderignore in git
// root)" (aider.chat/docs/config/aider_conf.html). Aider reads it with
// no wiring. Point `outputs.aider.ignore-file` elsewhere and the config
// file's `aiderignore:` key has to name it.
package aider

import (
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target            = "aider"
	defaultOutFile    = "CONVENTIONS.md"
	defaultIgnoreFile = ".aiderignore"
)

var caps = emit.Capabilities{
	SkillFields: emit.SkillFieldCoverage{Markdown: func(spec.Entry) string { return "" }},
	Target:      target,
	Supports:    []spec.Kind{spec.KindAgent, spec.KindSkill, spec.KindRule, spec.KindIgnore},
}

// Adapter emits Aider configs.
type Adapter struct{}

// New returns an Aider adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

func (Adapter) Capabilities() []spec.Kind { return caps.Supports }

// Emit writes the optional legacy concatenated CONVENTIONS.md (only
// when `outputs.aider.rules-file` is set) and, when configured, merges
// Aider's project config file so it auto-loads the conventions
// document. The default CONVENTIONS.md is written centrally by `sync`
// as a slim pointer body.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	outFile := emit.OutputFile(cfg, target, defaultOutFile)
	if rulesFile := emit.OutputRulesFile(cfg, target, ""); rulesFile != "" {
		if err := sess.EmitLegacyRulesFile(b, cfg, target, emit.MergedOpts{
			Title:              "Conventions",
			AgentSectionPrefix: "Agent: ",
		}, dryRun); err != nil {
			return err
		}
		outFile = rulesFile
	} else {
		// Aider agents and skills reach the target only through the
		// legacy merged document. Without rules-file set they stay
		// source-dir only.
		emit.NoteCoverageGap(target, spec.KindAgent, len(b.Agents), "outputs.aider.rules-file")
		emit.NoteCoverageGap(target, spec.KindSkill, len(b.Skills), "outputs.aider.rules-file")
	}
	if err := sess.WriteIgnoreFile(b.Ignores, target, emit.OutputIgnoreFile(cfg, target, defaultIgnoreFile), dryRun); err != nil {
		return err
	}
	return emitConf(
		sess,
		emit.OutputConfFile(cfg, target, ""),
		outFile,
		emit.OutputModel(cfg, target, ""),
		emit.OutputWeakModel(cfg, target, ""),
		dryRun,
	)
}

// emitConf writes (or merges into) Aider's project config at confPath.
// No-op when confPath is empty so existing users see no surprise
// writes. The read entry is appended to any pre-existing `read:` list
// without duplicating; model and weak-model overwrite when set.
//
// The write records what sync set, so dropping aider or conf-file takes
// out only that and keeps the user's keys (#1550). In `read:` it claims
// only the entry it adds: one the user listed first stays theirs. An
// entry it added for an earlier rules path goes (#1562).
func emitConf(sess *emit.Session, confPath, readEntry, model, weakModel string, dryRun bool) error {
	if confPath == "" {
		return nil
	}
	doc := readExistingYAML(confPath, dryRun)
	var claimed []emit.MergedKey
	var released [][]string
	prior := emit.PriorClaimedItems(confPath, []string{"read"})
	addedBySync := func(entry string) bool { return slices.Contains(prior, emit.ContentSum(entry)) }
	list := toStringList(doc["read"])
	if kept := slices.DeleteFunc(slices.Clone(list), func(entry string) bool { return entry != readEntry && addedBySync(entry) }); len(kept) < len(list) {
		list = kept
		doc["read"] = kept
		if len(kept) == 0 {
			delete(doc, "read")
		}
	}
	switch {
	case readEntry != "" && (!slices.Contains(list, readEntry) || addedBySync(readEntry)):
		claimed = append(claimed, emit.MergedKey{Path: []string{"read"}, Items: []string{readEntry}})
	case len(prior) > 0:
		released = append(released, []string{"read"})
	}
	mergeReadEntry(doc, readEntry)
	for _, kv := range [][2]string{{"model", model}, {"weak-model", weakModel}} {
		if kv[1] != "" {
			doc[kv[0]] = kv[1]
			claimed = append(claimed, emit.MergedKey{Path: []string{kv[0]}})
		}
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", confPath, err)
	}
	return sess.WriteMergedYAML(confPath, emit.HeaderBlock(emit.FormatYAML)+string(raw), claimed, released, dryRun)
}

// readExistingYAML loads the user's aider config so emitConf overwrites
// only its managed keys. dryRun returns empty to keep `--dry-run`
// previews pure; capture mode (sync --check, doctor, status) still reads
// the file because its user-authored keys are part input, not a pure
// output, so drift detection and `doctor --fix` must see them (#465).
func readExistingYAML(path string, dryRun bool) map[string]any {
	if dryRun {
		return map[string]any{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil || doc == nil {
		return map[string]any{}
	}
	return doc
}

func mergeReadEntry(doc map[string]any, entry string) {
	if entry == "" {
		return
	}
	list := toStringList(doc["read"])
	for _, s := range list {
		if s == entry {
			return
		}
	}
	doc["read"] = append(list, entry)
}

func toStringList(v any) []string {
	switch x := v.(type) {
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}

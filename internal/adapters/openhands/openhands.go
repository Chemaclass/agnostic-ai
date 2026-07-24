// Package openhands emits configs for All Hands' OpenHands agent.
//
// OpenHands recommends skills as one folder per skill under
// `.agents/skills/<name>/SKILL.md`, the same shared cross-tool tree
// codex, amp, zed, and crush already emit into; the renderer matches
// that output byte-for-byte so the shared tree dedupes.
//
// The project-root AGENTS.md is written centrally by `sync` as a slim
// pointer to the source specs (one body shared with every other
// target's entry-point file). OpenHands reads that file natively for
// project instructions, and rules reach it exclusively through that
// shared entry-point: OpenHands has no per-rule directory, so this
// adapter never writes rules directly.
package openhands

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "openhands"
	defaultSkillsDir = ".agents/skills"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes rules
	// itself: they reach OpenHands through the shared AGENTS.md
	// entry-point sync writes centrally. KindAgent is absent; OpenHands
	// has no agent surface, so the unsupported warning is accurate.
	Supports: []spec.Kind{spec.KindSkill, spec.KindRule},
}

// Adapter emits OpenHands configs.
type Adapter struct{}

// New returns an OpenHands adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/.
// The project-root AGENTS.md (with rule bodies inlined) is written by
// `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	return sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun)
}

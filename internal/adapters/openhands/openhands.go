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
//
// MCP servers merge into `./config.toml` (override via
// outputs.openhands.mcp-file) under a `[mcp]` table with three arrays:
// `stdio_servers` (`[[mcp.stdio_servers]]` tables carrying `name`,
// `command`, `args`, `env`), and `sse_servers` / `shttp_servers` (plain
// URL-string arrays; `shttp_servers` is OpenHands' streamable-HTTP
// transport, the cross-tool spec's `type: http`). OpenHands has no
// `type` field of its own; transport is implied by which array a
// server lands in. The TOML rendering reuses the same field writers
// Codex's `[mcp_servers.<name>]` tables use (see
// internal/adapters/internal/emit/toml.go) rather than a second
// hand-rolled copy.
package openhands

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	target           = "openhands"
	defaultSkillsDir = ".agents/skills"
	defaultMCPFile   = "config.toml"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes rules
	// itself: they reach OpenHands through the shared AGENTS.md
	// entry-point sync writes centrally. KindAgent is absent; OpenHands
	// has no agent surface, so the unsupported warning is accurate.
	Supports: []spec.Kind{spec.KindSkill, spec.KindRule, spec.KindMCP},
}

// Adapter emits OpenHands configs.
type Adapter struct{}

// New returns an OpenHands adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/,
// plus a merged `./config.toml` for MCP servers. The project-root
// AGENTS.md (with rule bodies inlined) is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitMCPConfig sorts mcps into OpenHands' three [mcp] arrays, surfaces
// a coverage note for any transport that maps to none of them (rather
// than guessing a bucket), and writes the merged TOML when at least
// one server rendered.
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	stdio, sse, shttp, unmapped := mcpTransportBuckets(mcps)
	emit.NoteCoverageGap(target, spec.KindMCP, unmapped,
		"no OpenHands [mcp] array for this transport")
	doc := renderConfigTOML(stdio, sse, shttp)
	if doc == "" {
		return nil
	}
	return sess.WriteFile(path, doc, dryRun)
}

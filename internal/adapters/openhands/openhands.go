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
// `command`, `args`, `env`), and `sse_servers` / `shttp_servers`
// (`shttp_servers` is OpenHands' streamable-HTTP transport, the
// cross-tool spec's `type: http`). Each sse/shttp element is a bare URL
// string, or the vendor's documented `{ url, api_key, timeout }` object
// once the entry sets a top-level `api_key` and/or (shttp only) a
// `timeout` meta field; OpenHands has no header-map field for these, so
// a spec's `headers` value never reaches it and surfaces a coverage
// note instead of vanishing silently, and `timeout` on an sse entry
// gets the same treatment since the vendor documents it for shttp only
// (#588). Read config_toml.go's serverValue for the exact mapping this
// defends. OpenHands has no `type` field of its own; transport is
// implied by which array a server lands in. The TOML rendering reuses
// the same field writers Codex's `[mcp_servers.<name>]` tables use (see
// internal/adapters/internal/emit/toml.go) rather than a second
// hand-rolled copy.
//
// An environment spec's `install` field writes `.openhands/setup.sh`
// (override via outputs.openhands.setup-file), the vendor's documented
// project bootstrap script: "You can add a `.openhands/setup.sh` file,
// which will run every time OpenHands begins working with your
// repository" (docs.openhands.dev/openhands/usage/customization/repository).
// `terminals` has no OpenHands surface (the script runs once,
// synchronously, not as a set of long-running processes) and surfaces
// a coverage note. See setup_script.go for the full mapping and the
// merge policy across multiple environment specs.
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
	defaultSetupFile = ".openhands/setup.sh"
)

var caps = emit.Capabilities{
	Target: target,
	// KindRule is declared even though this adapter never writes rules
	// itself: they reach OpenHands through the shared AGENTS.md
	// entry-point sync writes centrally. KindAgent is absent; OpenHands
	// has no agent surface, so the unsupported warning is accurate.
	Supports: []spec.Kind{spec.KindSkill, spec.KindRule, spec.KindMCP, spec.KindEnvironment},
}

// Adapter emits OpenHands configs.
type Adapter struct{}

// New returns an OpenHands adapter.
func New() *Adapter { return &Adapter{} }

// Name returns the target identifier.
func (Adapter) Name() string { return target }

// Emit writes one native skill folder per skill under .agents/skills/,
// plus a merged `./config.toml` for MCP servers and `.openhands/setup.sh`
// for environment specs. The project-root AGENTS.md (with rule bodies
// inlined) is written by `sync`, not here.
func (Adapter) Emit(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	if err := emit.ReportUnsupported(caps, b, cfg.OnUnsupported); err != nil {
		return err
	}
	skillsDir := emit.OutputSkillsDir(cfg, target, defaultSkillsDir)
	if err := sess.WriteSkillFolders(b.Skills, target, skillsDir, dryRun); err != nil {
		return err
	}
	if err := emitSetupScript(sess, b.Environments, cfg, dryRun); err != nil {
		return err
	}
	return emitMCPConfig(sess, b.MCPs, emit.OutputMCPFile(cfg, target, defaultMCPFile), dryRun)
}

// emitMCPConfig sorts mcps into OpenHands' three [mcp] arrays, surfaces
// a coverage note for any transport that maps to none of them (rather
// than guessing a bucket), for any remote entry whose `headers` cannot
// reach OpenHands' single-`api_key` credential field, and for any sse
// entry whose `timeout` has no effect there (shttp-only), and writes
// the merged TOML when at least one server rendered.
func emitMCPConfig(sess *emit.Session, mcps []spec.Entry, path string, dryRun bool) error {
	stdio, sse, shttp, unmapped, headersNoOp, timeoutNoOp := mcpTransportBuckets(mcps)
	emit.NoteCoverageGap(target, spec.KindMCP, unmapped,
		"no OpenHands [mcp] array for this transport")
	emit.NoteFieldNoOp(target, spec.KindMCP, "headers", headersNoOp,
		"OpenHands sse/shttp servers take a single api_key string, not a headers map; set meta.api_key directly")
	emit.NoteFieldNoOp(target, spec.KindMCP, "timeout", timeoutNoOp,
		"OpenHands documents timeout for shttp_servers only, not sse_servers")
	doc := renderConfigTOML(stdio, sse, shttp)
	if doc == "" {
		return nil
	}
	return sess.WriteFile(path, doc, dryRun)
}

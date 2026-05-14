package errs

import "sort"

// Entry is a registry record describing one error code.
type Entry struct {
	Code  Code
	Title string
	Cause string
	Fix   string
}

// registry holds the canonical metadata for every defined code. Keep
// in sync with docs/user/errors.md.
var registry = map[Code]Entry{
	CodeSpecParse: {
		Code:  CodeSpecParse,
		Title: "Spec parse failed",
		Cause: "A spec file could not be parsed. Markdown specs use YAML frontmatter; hooks and MCPs are pure YAML. The error includes the path and (when available) line:col of the offending byte.",
		Fix:   "Open the file at the reported position. Confirm the frontmatter delimiters (`---`) wrap the metadata and that the YAML is well-formed (correct indentation, no tabs, quoted strings where needed).",
	},
	CodeUnsupportedKind: {
		Code:  CodeUnsupportedKind,
		Title: "Spec kind not supported by target",
		Cause: "A spec kind (hook, mcp, command, ...) is present in the bundle but the target adapter does not emit it. Default policy logs a warning; `on-unsupported: error` upgrades it to a hard failure.",
		Fix:   "Either drop the spec, switch the target to one that supports the kind, or set `on-unsupported: warn` (or `silent`) in agnostic-ai.yaml.",
	},
	CodeConfigMissing: {
		Code:  CodeConfigMissing,
		Title: "Config file missing",
		Cause: "Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.",
		Fix:   "Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one.",
	},
	CodeConfigDecode: {
		Code:  CodeConfigDecode,
		Title: "Config decode failed",
		Cause: "The config file was found but could not be parsed as YAML, or its keys do not match the expected schema.",
		Fix:   "Validate against `docs/schemas/config.schema.json`. Check indentation and that list keys (e.g. `targets:`) hold a YAML sequence.",
	},
	CodeOutputCollision: {
		Code:  CodeOutputCollision,
		Title: "Targets emit to the same output path",
		Cause: "Two or more enabled targets would write to the same path (commonly AGENTS.md, shared by codex, amp, warp, opencode and zed). Last-writer-wins would mask drift.",
		Fix:   "Drop one of the colliding targets from `targets:` in agnostic-ai.yaml, or override the colliding path via `outputs.<target>.file`.",
	},
	CodeImportFileUnknown: {
		Code:  CodeImportFileUnknown,
		Title: "Import source name unknown",
		Cause: "The argument passed to `agnostic-ai import` does not match any registered source.",
		Fix:   "Run `agnostic-ai import --help` for the supported list. Spelling counts.",
	},
	CodeSyncTargetUnknown: {
		Code:  CodeSyncTargetUnknown,
		Title: "Unknown sync target",
		Cause: "A target requested via `--target`, `--only`, or the config is not a built-in adapter and no `agnostic-ai-adapter-<name>` binary is on PATH.",
		Fix:   "Check the target name spelling. Built-ins: claude, codex, gemini, cursor, copilot, aider, cline, windsurf, continue, amp, zed, warp, opencode, antigravity. External adapters live on PATH as `agnostic-ai-adapter-<name>`.",
	},
	CodeFlagConflict: {
		Code:  CodeFlagConflict,
		Title: "Mutually exclusive flags",
		Cause: "Two flags whose effects conflict were passed together (e.g. `--only` with `--except`, or `--watch` with `--check`).",
		Fix:   "Pick one. The error message names both flags so you can drop the wrong one.",
	},
}

// Lookup returns the registry entry for code. The boolean is false
// when the code is not registered.
func Lookup(code Code) (Entry, bool) {
	e, ok := registry[code]
	return e, ok
}

// All returns every registered entry, sorted by code, for docs and the
// `explain` listing.
func All() []Entry {
	out := make([]Entry, 0, len(registry))
	for _, e := range registry {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

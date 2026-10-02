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
// in sync with docs/site/content/docs/errors.md.
var registry = map[Code]Entry{
	CodeSpecParse: {
		Code:  CodeSpecParse,
		Title: "Spec parse failed",
		Cause: "A spec file could not be parsed. Markdown specs use YAML frontmatter; hooks and MCPs are pure YAML. The error includes the path and (when available) line:col of the offending byte. A review `@path` include that cannot be read reports here too.",
		Fix:   "Open the file at the reported position. Confirm the frontmatter delimiters (`---`) wrap the metadata and that the YAML is well-formed (correct indentation, no tabs, quoted strings where needed). For an include, create the file or use a path inside the project.",
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
		Fix:   "Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.",
	},
	CodeConfigDecode: {
		Code:  CodeConfigDecode,
		Title: "Config decode failed",
		Cause: "The config file was found but could not be parsed as YAML, or its keys do not match the expected schema. A `requires` value that is not a version constraint fails here too.",
		Fix:   "Rename or remove each unknown key the message names, taking its did-you-mean when one is given. Otherwise validate against `docs/schemas/config.schema.json`. Check indentation and that list keys (e.g. `targets:`) hold a YAML sequence. Run `agnostic-ai doctor` for a full diagnosis.",
	},
	CodeRequiresUnmet: {
		Code:  CodeRequiresUnmet,
		Title: "Installed version outside requires",
		Cause: "The config's `requires` key names the agnostic-ai releases its specs work with: a minimum, one exact release, or a range. The installed binary is outside it. Every command that reads the specs, such as `sync`, `lint`, `validate`, `doctor`, `revert`, and `cleanup`, stops before it reads specs or writes files.",
		Fix:   "Run `agnostic-ai upgrade`, or `agnostic-ai upgrade --version vX.Y.Z` when `requires` pins or bounds a release, which also downgrades a standalone binary. A binary installed into the project by npm, pnpm, Yarn, or Bun comes from that package manager, so run its install after a version bump, or update `requires` when package.json already pins the running release. If the version stays the same, `agnostic-ai upgrade --check` shows which binary runs and any older copy that shadows it on PATH.",
	},
	CodeOutputCollision: {
		Code:  CodeOutputCollision,
		Title: "Targets emit to the same output path",
		Cause: "Two or more enabled targets would write different content to the same path. Last-writer-wins would mask drift.",
		Fix:   "Drop one of the colliding targets from `targets:` in agnostic-ai.yaml, or override the matching `outputs.<target>` path setting, such as `file`, `rules-file`, or `skills-dir`.",
	},
	CodeIgnoreOverwrite: {
		Code:  CodeIgnoreOverwrite,
		Title: "Hand-authored ignore file would lose patterns",
		Cause: "Replacing a target's ignore file without an agnostic-ai header would remove or reorder existing patterns, or add a negation. These changes can make excluded files readable, so sync refuses the overwrite.",
		Fix:   "Run `agnostic-ai import <target>` to copy the imported file's patterns into an ignore spec. When a target reads several ignore files, combine their patterns in the spec and preserve their order before syncing. The error names a risky negation or up to five missing or reordered patterns, with a count for the rest. Deleting the file also clears the error, at the cost of those patterns.",
	},
	CodeImportFileUnknown: {
		Code:  CodeImportFileUnknown,
		Title: "Import source name unknown",
		Cause: "The argument passed to `agnostic-ai import` does not match any registered source.",
		Fix:   "Run `agnostic-ai import --help` for the supported list. Spelling counts.",
	},
	CodeImportWouldReplace: {
		Code:  CodeImportWouldReplace,
		Title: "Import would replace an existing spec",
		Cause: "`import`, `init --from`, or `use` would replace a spec under the source directories with different content that the importing tool never read: a hand-written spec, one edited since the last sync or import, one sync never wrote for that tool, or one another tool's import wrote. No spec was written.",
		Fix:   "Rename the existing spec to keep both and import again, or run `agnostic-ai import <tool> --overwrite` to replace it. The message names each spec and the tool that wanted it.",
	},
	CodeSyncTargetUnknown: {
		Code:  CodeSyncTargetUnknown,
		Title: "Unknown sync target",
		Cause: "A target requested via `--target`, `--only`, or the config is not a built-in adapter and no `agnostic-ai-adapter-<name>` binary is on PATH.",
		Fix:   "Check the spelling, or use the name the error suggests. A target that exists but is not in this run needs adding to `targets` in agnostic-ai.yaml, or to `-t`. Built-ins: https://agnostic-ai.org/docs/targets/. External adapters live on PATH as `agnostic-ai-adapter-<name>`.",
	},
	CodeFlagConflict: {
		Code:  CodeFlagConflict,
		Title: "Mutually exclusive flags",
		Cause: "Two flags whose effects conflict were passed together (e.g. `--only` with `--except`), or a flag was passed without the one it needs (e.g. `--diff` without `--dry-run`).",
		Fix:   "The message names both flags. Drop one when they conflict; add the missing one when a flag needs another.",
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

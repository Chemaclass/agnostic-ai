package codex

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const permissionsUseExecPoliciesReason = "Codex has no per-tool allow/deny/ask key; its rule surface is exec policies, whose prefix_rule patterns are token lists rather than globs, so set outputs.codex.exec-policies for shell rules"

const otherToolPermissionsReason = "Codex has no per-tool allow/deny/ask key and its exec policies match shell commands, so only Bash rules translate"

const (
	defaultExecPoliciesFile = ".codex/rules/default.rules"

	// execPoliciesOverlayPath is the captured YAML representation written
	// by `agnostic-ai import codex` from a pre-existing
	// `.codex/rules/default.rules`. Loaded automatically when neither
	// `outputs.codex.exec-policies` inline nor `exec-policies-file` is
	// set. Round-trip parity with the codex config overlay pattern.
	execPoliciesOverlayPath = ".agnostic-ai/overlays/codex.exec-policies.yaml"

	// execPoliciesHeaderOverlayPath holds the optional file-level comment
	// block lifted off the top of `.codex/rules/default.rules` at import.
	// Re-rendered above the first `prefix_rule(...)` so a hand-authored
	// file header round-trips byte-stably.
	execPoliciesHeaderOverlayPath = ".agnostic-ai/overlays/codex.exec-policies-header.txt"
)

// emitExecPolicies writes resolved native or translated command prefixes.
func emitExecPolicies(sess *emit.Session, cfg *config.Config, policies []config.CodexExecPolicy, dryRun bool) error {
	if len(policies) == 0 {
		return nil
	}
	for i, p := range policies {
		if err := validateExecPolicy(p, i); err != nil {
			return err
		}
	}
	header, err := loadExecPoliciesHeader(cfg, dryRun)
	if err != nil {
		return err
	}
	body := renderExecPoliciesSkylark(policies, header)
	// Starlark uses `#` line comments, the same as YAML, so reuse the YAML
	// header so the provenance banner sits inside a comment block.
	return sess.WriteFile(defaultExecPoliciesFile, emit.WithHeader(body, emit.FormatYAML), dryRun)
}

// loadExecPoliciesHeader reads the file-level comment captured by
// `agnostic-ai import codex` (sidecar to the YAML overlay). Returns ""
// when absent or when the user opted into inline / explicit-file
// policies (the overlay path is not used in those cases). dryRun skips
// disk so `--dry-run` previews stay pure.
func loadExecPoliciesHeader(cfg *config.Config, dryRun bool) (string, error) {
	if dryRun || !shouldUseExecPoliciesOverlay(cfg) {
		return "", nil
	}
	data, err := os.ReadFile(execPoliciesHeaderOverlayPath)
	if emit.IsAbsent(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", execPoliciesHeaderOverlayPath, err)
	}
	return string(data), nil
}

// shouldUseExecPoliciesOverlay reports whether the captured overlay is
// the source of truth for this sync. Mirrors the branch in
// loadExecPolicies: only the no-inline + no-explicit-file case falls
// through to the overlay.
func shouldUseExecPoliciesOverlay(cfg *config.Config) bool {
	out, ok := cfg.Outputs[target]
	if !ok {
		return true
	}
	if out.ExecPolicies != nil {
		return false
	}
	return out.ExecPoliciesFile == ""
}

// loadExecPolicies pulls inline policies from `outputs.codex.exec-policies`
// and appends any read from `outputs.codex.exec-policies-file` (when set).
// When neither is set, falls back to the captured overlay at
// `.agnostic-ai/overlays/codex.exec-policies.yaml` so a project that ran
// `agnostic-ai import codex` against an existing `.codex/rules/default.rules`
// keeps the entries on re-sync without further config.
//
// Codex applies the strictest decision across matching rules.
func loadExecPolicies(cfg *config.Config) ([]config.CodexExecPolicy, error) {
	policies := slices.Clone(cfg.Outputs[target].ExecPolicies)
	filePath := execPoliciesSourceFile(cfg)
	if filePath == "" {
		return policies, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filePath, err)
	}
	var extra []config.CodexExecPolicy
	if err := yaml.Unmarshal(data, &extra); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	policies = append(policies, extra...)
	return policies, nil
}

func execPoliciesSourceFile(cfg *config.Config) string {
	out := cfg.Outputs[target]
	if out.ExecPoliciesFile != "" {
		return out.ExecPoliciesFile
	}
	// An inline list, even an empty one, opts out of the captured overlay.
	if out.ExecPolicies != nil {
		return ""
	}
	if _, err := os.Stat(execPoliciesOverlayPath); err == nil {
		return execPoliciesOverlayPath
	}
	return ""
}

// validateExecPolicy enforces the minimal schema. Index is included in
// the error so users with a long list can find the offender.
func validateExecPolicy(p config.CodexExecPolicy, index int) error {
	if len(p.Pattern) == 0 {
		return fmt.Errorf("exec-policies[%d]: pattern must not be empty", index)
	}
	switch p.Decision {
	case "allow", "forbidden", "prompt":
	default:
		return fmt.Errorf("exec-policies[%d]: decision must be allow|forbidden|prompt, got %q", index, p.Decision)
	}
	return nil
}

// renderExecPoliciesSkylark turns the policy list into the Codex CLI's
// `prefix_rule(...)` DSL. Each policy renders as one block with all
// optional fields (`justification`, `match`) as inline kwargs — the form
// the codex docs show and the form `agnostic-ai import codex` captures
// from hand-authored files, so import → sync stays byte-stable.
//
// `header` is an optional file-level comment block captured from the
// pre-existing `.codex/rules/default.rules` (one logical line per
// element, no `#` prefix). When non-empty it renders as `# <line>`
// comments above the first rule, separated by a blank line so a
// re-import detects it as the file header rather than the first rule's
// justification.
//
// Multi-line justification strings collapse to a single line in the
// emit because the inline kwarg form takes one double-quoted Starlark
// string; embedded newlines are not preserved (use the YAML overlay if
// you need a multi-paragraph justification).
func renderExecPoliciesSkylark(policies []config.CodexExecPolicy, header string) string {
	var b strings.Builder
	if header != "" {
		for _, line := range strings.Split(strings.TrimRight(header, "\n"), "\n") {
			if line == "" {
				b.WriteString("#\n")
				continue
			}
			b.WriteString("# ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	for i, p := range policies {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("prefix_rule(\n")
		b.WriteString("    pattern = ")
		writeStringList(&b, p.Pattern)
		b.WriteString(",\n")
		fmt.Fprintf(&b, "    decision = %q,\n", p.Decision)
		if p.Justification != "" {
			justification := strings.ReplaceAll(strings.TrimSpace(p.Justification), "\n", " ")
			fmt.Fprintf(&b, "    justification = %q,\n", justification)
		}
		if len(p.Match) > 0 {
			b.WriteString("    match = ")
			writeStringList(&b, p.Match)
			b.WriteString(",\n")
		}
		b.WriteString(")\n")
	}
	return b.String()
}

// writeStringList writes a Starlark string list literal:
// `["a", "b", "c"]`. Empty list is `[]`.
func writeStringList(b *strings.Builder, xs []string) {
	if len(xs) == 0 {
		b.WriteString("[]")
		return
	}
	b.WriteByte('[')
	for i, s := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%q", s)
	}
	b.WriteByte(']')
}

// customSettingsNoRouteReason explains why an `x-codex` block on a
// settings spec reaches nothing. Every other settings target merges
// that block into a JSON document it already owns. Codex's project
// config is TOML, rendered from the overlay `import codex` captures
// plus the first-class `outputs.codex.config` fields, so an arbitrary
// JSON-shaped block has no place to land: a nested map has no TOML
// value form here, and a key the overlay already carries would emit
// twice and break the file. Codex has two routes for the same intent,
// and the note names them rather than dropping the block in silence
// (#949).
const customSettingsNoRouteReason = "Codex's .codex/config.toml is TOML rendered from the captured overlay plus outputs.codex.config; set the key there, or run import codex to capture it from the file"

// specsWithCustomSettings counts the settings specs carrying an
// `x-codex` block, so the caller folds them into one coverage note
// rather than one note per key.
func specsWithCustomSettings(settings []spec.Entry) int {
	n := 0
	for _, entry := range settings {
		if len(emit.SettingsCustomKeys([]spec.Entry{entry}, target)) > 0 {
			n++
		}
	}
	return n
}

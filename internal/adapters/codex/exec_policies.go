package codex

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const (
	defaultExecPoliciesFile = ".codex/rules/default.rules"

	// execPoliciesOverlayPath is the captured YAML representation written
	// by `agnostic-ai import codex` from a pre-existing
	// `.codex/rules/default.rules`. Loaded automatically when neither
	// `outputs.codex.exec-policies` inline nor `exec-policies-file` is
	// set. Round-trip parity with the codex config overlay pattern.
	execPoliciesOverlayPath = ".agnostic-ai/overlays/codex.exec-policies.yaml"
)

// emitExecPolicies writes the Skylark-flavored `prefix_rule(...)` file at
// `.codex/rules/default.rules` from the declarative list in
// `outputs.codex.exec-policies` (inline) or `outputs.codex.exec-policies-file`
// (path to a YAML list). The file is the exec-policy DSL Codex CLI reads to
// allow- or forbid-list shell command prefixes for repo automation.
//
// No-op when neither field is set so users who do not opt in get no
// surprise file under `.codex/rules/`.
func emitExecPolicies(cfg *config.Config, dryRun bool) error {
	policies, err := loadExecPolicies(cfg)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}
	for i, p := range policies {
		if err := validateExecPolicy(p, i); err != nil {
			return err
		}
	}
	body := renderExecPoliciesSkylark(policies)
	// Skylark uses `#` line comments — same as YAML — so reuse the YAML
	// header so the provenance banner sits inside a comment block.
	return emit.WriteFile(defaultExecPoliciesFile, emit.WithHeader(body, emit.FormatYAML), dryRun)
}

// loadExecPolicies pulls inline policies from `outputs.codex.exec-policies`
// and appends any read from `outputs.codex.exec-policies-file` (when set).
// When neither is set, falls back to the captured overlay at
// `.agnostic-ai/overlays/codex.exec-policies.yaml` so a project that ran
// `agnostic-ai import codex` against an existing `.codex/rules/default.rules`
// keeps the entries on re-sync without further config.
//
// File-derived entries land after inline entries; ordering matters
// because Codex CLI evaluates rules top-down.
func loadExecPolicies(cfg *config.Config) ([]config.CodexExecPolicy, error) {
	out, hasOut := cfg.Outputs[target]
	var policies []config.CodexExecPolicy
	if hasOut {
		policies = append(policies, out.ExecPolicies...)
	}

	filePath := ""
	if hasOut && out.ExecPoliciesFile != "" {
		filePath = out.ExecPoliciesFile
	} else if len(policies) == 0 {
		// Fall back to the captured overlay only when the user has no
		// explicit declarations. A user with inline entries opts out of
		// the overlay implicitly.
		if _, err := os.Stat(execPoliciesOverlayPath); err == nil {
			filePath = execPoliciesOverlayPath
		}
	}
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

// validateExecPolicy enforces the minimal schema. Index is included in
// the error so users with a long list can find the offender.
func validateExecPolicy(p config.CodexExecPolicy, index int) error {
	if len(p.Pattern) == 0 {
		return fmt.Errorf("exec-policies[%d]: pattern must not be empty", index)
	}
	switch p.Decision {
	case "allow", "forbidden", "ask":
	default:
		return fmt.Errorf("exec-policies[%d]: decision must be allow|forbidden|ask, got %q", index, p.Decision)
	}
	return nil
}

// renderExecPoliciesSkylark turns the policy list into the Codex CLI's
// `prefix_rule(...)` DSL. Each policy renders as one block with all
// optional fields (`justification`, `match`) as inline kwargs — the form
// the codex docs show and the form `agnostic-ai import codex` captures
// from hand-authored files, so import → sync stays byte-stable.
//
// Multi-line justification strings collapse to a single line in the
// emit because the inline kwarg form takes one double-quoted Skylark
// string; embedded newlines are not preserved (use the YAML overlay if
// you need a multi-paragraph justification).
func renderExecPoliciesSkylark(policies []config.CodexExecPolicy) string {
	var b strings.Builder
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

// writeStringList writes a Python/Skylark string list literal:
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

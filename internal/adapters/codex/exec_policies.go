package codex

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const defaultExecPoliciesFile = ".codex/rules/default.rules"

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
// The file-derived entries land after the inline entries; ordering matters
// because Codex CLI evaluates rules top-down.
func loadExecPolicies(cfg *config.Config) ([]config.CodexExecPolicy, error) {
	out, ok := cfg.Outputs[target]
	if !ok {
		return nil, nil
	}
	policies := append([]config.CodexExecPolicy(nil), out.ExecPolicies...)
	if out.ExecPoliciesFile == "" {
		return policies, nil
	}
	data, err := os.ReadFile(out.ExecPoliciesFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", out.ExecPoliciesFile, err)
	}
	var extra []config.CodexExecPolicy
	if err := yaml.Unmarshal(data, &extra); err != nil {
		return nil, fmt.Errorf("parse %s: %w", out.ExecPoliciesFile, err)
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
// `prefix_rule(...)` DSL. Each policy renders as one block. Justification
// appears as a leading `# ...` comment; example matches go below the
// rule as commented examples for human readers.
func renderExecPoliciesSkylark(policies []config.CodexExecPolicy) string {
	var b strings.Builder
	for i, p := range policies {
		if i > 0 {
			b.WriteByte('\n')
		}
		if p.Justification != "" {
			for _, line := range strings.Split(strings.TrimSpace(p.Justification), "\n") {
				b.WriteString("# ")
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
		b.WriteString("prefix_rule(\n")
		b.WriteString("    pattern = ")
		writeStringList(&b, p.Pattern)
		b.WriteString(",\n")
		b.WriteString(fmt.Sprintf("    decision = %q,\n", p.Decision))
		b.WriteString(")\n")
		for _, m := range p.Match {
			b.WriteString("# match: ")
			b.WriteString(m)
			b.WriteByte('\n')
		}
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

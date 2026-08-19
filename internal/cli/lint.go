package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintSeverity classifies the impact of a lint finding.
type lintSeverity int

const (
	lintWarn  lintSeverity = iota // non-zero exit only when --strict
	lintError                     // always non-zero exit
)

func (s lintSeverity) String() string {
	if s == lintError {
		return "error"
	}
	return "warn"
}

// lintFinding is one semantic issue reported by the linter.
type lintFinding struct {
	Code     string
	Severity lintSeverity
	Path     string
	Message  string
}

func (f lintFinding) String() string {
	return fmt.Sprintf("%s [%s] %s: %s", f.Code, f.Severity, f.Path, f.Message)
}

func newLintCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run semantic lint checks on source specs beyond schema validation.",
		Long: "Checks for empty specs, duplicate names, dead specs (kinds not " +
			"supported by any enabled target), hooks whose event ignores their " +
			"matcher, unterminated frontmatter, and frontmatter keys that near-miss " +
			"a key agnostic-ai owns (allowed_tools vs tools). Exit code 1 on " +
			"error-severity findings, or on warn-severity findings when --strict " +
			"is set.",
		Example: `  # Lint all specs
  agnostic-ai lint

  # Treat warnings as errors (useful in CI)
  agnostic-ai lint --strict`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			entries := b.All()
			if len(entries) == 0 {
				cmd.PrintErrln(emptySpecsHint)
				return nil
			}

			findings := collectLintFindings(cfg.Targets, b)

			if len(findings) == 0 {
				cmd.Printf("ok — %d spec(s) clean\n", len(entries))
				return nil
			}

			hasError := false
			hasWarn := false
			for _, f := range findings {
				cmd.Printf("%s\n", f)
				switch f.Severity {
				case lintError:
					hasError = true
				case lintWarn:
					hasWarn = true
				}
			}
			cmd.Printf("\n%d finding(s): %d error(s), %d warning(s)\n",
				len(findings), countSeverity(findings, lintError), countSeverity(findings, lintWarn))

			if hasError || (strict && hasWarn) {
				return fmt.Errorf("lint failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors.")
	return cmd
}

// collectLintFindings runs every rule against a loaded bundle. Both `lint`
// and the LSP call it so the two cannot report different sets: before this
// existed the LSP silently lacked the newest rule.
func collectLintFindings(targets []string, b spec.Bundle) []lintFinding {
	entries := b.All()

	// Shadowed entries lost a same-layer name clash and reach no target.
	// They are absent from All(), so LINT003 only sees the clash when they
	// are folded back in (#582).
	withShadowed := make([]spec.Entry, 0, len(entries)+len(b.Shadowed))
	withShadowed = append(withShadowed, entries...)
	withShadowed = append(withShadowed, b.Shadowed...)

	var findings []lintFinding
	findings = append(findings, lintEmptySpecs(entries)...)
	findings = append(findings, lintDuplicateNames(withShadowed)...)
	findings = append(findings, lintDeadSpecs(entries, targets)...)
	findings = append(findings, lintHookMatcherMisuse(b.Hooks)...)
	findings = append(findings, lintUnterminatedFrontmatter(entries)...)
	findings = append(findings, lintNearMissKeys(entries)...)
	return findings
}

// lintEmptySpecs flags specs with no body and no description (LINT001, warn).
func lintEmptySpecs(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		body := strings.TrimSpace(e.Body)
		desc, _ := e.Meta["description"].(string)
		if body == "" && strings.TrimSpace(desc) == "" {
			out = append(out, lintFinding{
				Code:     "LINT001",
				Severity: lintWarn,
				Path:     e.Path,
				Message:  "empty spec — no body and no description",
			})
		}
	}
	return out
}

// lintDuplicateNames flags two or more specs of the same kind sharing the
// same name, which causes later specs to silently overwrite earlier ones
// during layered loading (LINT003, error).
func lintDuplicateNames(entries []spec.Entry) []lintFinding {
	type key struct {
		kind spec.Kind
		name string
	}
	seen := map[key]string{} // key → first path
	var out []lintFinding
	for _, e := range entries {
		if e.Name == "" {
			continue // missing-name is reported by validate
		}
		k := key{e.Kind, e.Name}
		if prior, ok := seen[k]; ok {
			out = append(out, lintFinding{
				Code:     "LINT003",
				Severity: lintError,
				Path:     e.Path,
				Message: fmt.Sprintf(
					"duplicate %s name %q — also defined in %s; later spec shadows earlier one",
					e.Kind, e.Name, prior,
				),
			})
		} else {
			seen[k] = e.Path
		}
	}
	return out
}

// lintDeadSpecs flags individual specs whose kind is not supported by any
// enabled target (LINT004, warn). Unlike lintOrphanKinds in validate (which
// reports once per kind), this reports per-spec so the user can act on each
// file directly.
func lintDeadSpecs(entries []spec.Entry, targets []string) []lintFinding {
	enabled := setOf(targets...)
	var out []lintFinding
	for _, e := range entries {
		if anyTargetSupports(e.Kind, enabled) {
			continue
		}
		supporters := sortedKeys(targetsSupportingKind[e.Kind])
		msg := fmt.Sprintf(
			"%s spec not consumed by any enabled target; targets that support %ss: %s",
			e.Kind, e.Kind, commaList(supporters),
		)
		out = append(out, lintFinding{
			Code:     "LINT004",
			Severity: lintWarn,
			Path:     e.Path,
			Message:  msg,
		})
	}
	return out
}

// lintHookMatcherMisuse flags hook specs whose event does not consume a
// matcher but still set one. The matcher is silently ignored by the native
// CLI, so the spec author likely intended a different event or should drop
// the matcher (LINT005, warn).
func lintHookMatcherMisuse(hooks []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		if event == "" || matcher == "" {
			continue
		}
		if _, ok := matcherAcceptingEvents[event]; ok {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT005",
			Severity: lintWarn,
			Path:     h.Path,
			Message: fmt.Sprintf(
				"matcher %q set but event %q does not consume a matcher; drop the matcher or use a tool-call event (e.g. PreToolUse, PostToolUse)",
				matcher, event,
			),
		})
	}
	return out
}

// lintUnterminatedFrontmatter flags specs that open a `---` block and never
// close it (LINT006, error).
//
// splitFrontmatter treats such a file as body-only, so the raw YAML survives
// as body text and every adapter writes it through verbatim. Nothing else
// catches this: the spec loads, validate passes, and sync exits 0 while
// emitting files whose frontmatter is structurally broken. Targets that write
// no frontmatter of their own end up with a single unterminated delimiter;
// targets that write their own block end up with a stray third one that opens
// a second block. Either way the agent silently never loads.
//
// A parsed block leaves Meta populated and strips the delimiters, so a body
// that still starts with `---` alongside empty Meta is the signature of the
// unterminated case.
func lintUnterminatedFrontmatter(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		if len(e.Meta) > 0 || !strings.HasPrefix(e.Body, "---") {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT006",
			Severity: lintError,
			Path:     e.Path,
			Message:  "frontmatter opens with `---` but is never closed; add the closing `---` or the block is emitted as body text",
		})
	}
	return out
}

func countSeverity(findings []lintFinding, s lintSeverity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// nearMissKeys maps a frontmatter key that looks like one agnostic-ai
// owns onto the key it was probably meant to be. Passthrough is the
// right default for genuine extensions, but a near miss of an owned key
// is not an extension: it parses, emits, and does nothing. Nine
// phel-lang agents ran with full tool access because `allowed_tools:`
// passed validate, lint --strict, sync --check and doctor for months
// (#617).
var nearMissKeys = map[string]string{
	"allowed_tools":    "tools",
	"allowedTools":     "tools",
	"allowed-tools":    "tools",
	"disallowed_tools": "tools",
	"max_turns":        "max-turns",
	"model_name":       "model",
}

// lintNearMissKeys flags those keys at the top level of a spec's
// frontmatter (LINT007, warn). Warn rather than error because a
// target-native spelling can be legitimate: Junie really does document
// disallowedTools. The message says to move such a key under
// x-<target>, which is correct whether the key was a typo or not. Keys
// already namespaced under x-<target> are deliberate and never flagged.
func lintNearMissKeys(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		keys := make([]string, 0, len(e.Meta))
		for k := range e.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			meant, ok := nearMissKeys[k]
			if !ok {
				continue
			}
			out = append(out, lintFinding{
				Code:     "LINT007",
				Severity: lintWarn,
				Path:     e.Path,
				Message: fmt.Sprintf("`%s:` is not a key agnostic-ai reads, so it has no effect. "+
					"Did you mean `%s:`? If it is a target-native key, move it under `x-<target>:` to keep it.", k, meant),
			})
		}
	}
	return out
}

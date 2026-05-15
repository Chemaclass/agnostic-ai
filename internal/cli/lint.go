package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintSeverity classifies the impact of a lint finding.
type lintSeverity int

const (
	lintWarn  lintSeverity = iota // non-zero exit only when --strict
	lintError                      // always non-zero exit
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
		Long: "Checks for empty specs, hook collisions, duplicate names, and dead " +
			"specs (kinds not supported by any enabled target). Exit code 1 on " +
			"error-severity findings; exit code 2 on warn-severity findings when " +
			"--strict is set.",
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

			var findings []lintFinding
			findings = append(findings, lintEmptySpecs(entries)...)
			findings = append(findings, lintHookCollisions(b.Hooks)...)
			findings = append(findings, lintDuplicateNames(entries)...)
			findings = append(findings, lintDeadSpecs(entries, cfg.Targets)...)

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

// lintHookCollisions flags pairs of hook specs that share the same event and
// matcher, which would result in two hooks running for the same trigger
// (LINT002, error).
func lintHookCollisions(hooks []spec.Entry) []lintFinding {
	type key struct{ event, matcher string }
	seen := map[key]string{} // key → first path
	var out []lintFinding
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		k := key{event, matcher}
		if prior, ok := seen[k]; ok {
			out = append(out, lintFinding{
				Code:     "LINT002",
				Severity: lintError,
				Path:     h.Path,
				Message: fmt.Sprintf(
					"hook collision — event %q matcher %q also defined in %s",
					event, matcher, prior,
				),
			})
		} else {
			seen[k] = h.Path
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

func countSeverity(findings []lintFinding, s lintSeverity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

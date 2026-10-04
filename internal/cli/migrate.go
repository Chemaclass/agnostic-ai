package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// specMigration rewrites one old spec form into its replacement without
// changing what sync writes for the targets a spec already reaches. Plan
// reads the project at root and returns the edits plus a reason for each
// entry it leaves alone; it never writes.
type specMigration struct {
	ID      string
	Group   string
	Release string
	Summary string
	Plan    func(root string) ([]migrationChange, []migrationSkip, error)
}

// migrationChange is one file edit: new content for Path, written to
// NewPath when the migration also renames the file.
type migrationChange struct {
	Path    string
	NewPath string
	Before  string
	After   string
}

type migrationSkip struct {
	Path   string
	Reason string
}

// specMigrations is the registry, in release order. Each entry is
// idempotent: it plans nothing once its old form is gone.
var specMigrations = []specMigration{configFileNameMigration}

type pendingMigration struct {
	specMigration
	changes []migrationChange
	skips   []migrationSkip
}

func newMigrateCmd() *cobra.Command {
	var dryRun, list bool
	var only []string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Rewrite old spec forms into their current replacements.",
		Long: "migrate applies every pending spec migration: a rewrite of an old spec\n" +
			"form, such as a renamed field or file, into its replacement. A migration\n" +
			"never changes what sync writes for the targets a spec already reaches,\n" +
			"so `sync --check` stays clean after it. Old forms keep working, so\n" +
			"running it is never required to sync.",
		Example: `  # Show which migrations apply to this project
  agnostic-ai migrate --list

  # Preview the rewrites
  agnostic-ai migrate --dry-run

  # Apply every pending migration, or one group
  agnostic-ai migrate
  agnostic-ai migrate --only config`,
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := selectMigrations(only)
			if err != nil {
				return err
			}
			pending, err := planMigrations(".", selected)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if list {
				printMigrationList(out, selected, pending)
				return nil
			}
			if len(pending) == 0 {
				_, _ = fmt.Fprintln(out, "no migrations apply")
				return nil
			}
			printMigrationPlan(out, pending, dryRun)
			if dryRun {
				return nil
			}
			lock, err := acquireProjectLock(".", "migrate")
			if err != nil {
				return err
			}
			defer func() { _ = lock.Close() }()
			return applyMigrations(pending)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the rewrites without writing them.")
	cmd.Flags().BoolVar(&list, "list", false, "List every migration and whether it applies here.")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Run only these migration groups (comma-separated).")
	return cmd
}

func selectMigrations(only []string) ([]specMigration, error) {
	if len(only) == 0 {
		return specMigrations, nil
	}
	var groups []string
	for _, m := range specMigrations {
		groups = append(groups, m.Group)
	}
	var out []specMigration
	for _, g := range only {
		if !slices.Contains(groups, g) {
			return nil, fmt.Errorf("--only: no migration group %q; groups: %s", g, strings.Join(slices.Compact(slices.Sorted(slices.Values(groups))), ", "))
		}
	}
	for _, m := range specMigrations {
		if slices.Contains(only, m.Group) {
			out = append(out, m)
		}
	}
	return out, nil
}

func planMigrations(root string, selected []specMigration) ([]pendingMigration, error) {
	var pending []pendingMigration
	for _, m := range selected {
		changes, skips, err := m.Plan(root)
		if err != nil {
			return nil, fmt.Errorf("migration %s: %w", m.ID, err)
		}
		if len(changes)+len(skips) > 0 {
			pending = append(pending, pendingMigration{m, changes, skips})
		}
	}
	return pending, nil
}

func printMigrationList(out io.Writer, selected []specMigration, pending []pendingMigration) {
	applies := map[string]pendingMigration{}
	for _, p := range pending {
		applies[p.ID] = p
	}
	for _, m := range selected {
		state := "does not apply"
		if p, ok := applies[m.ID]; ok {
			state = fmt.Sprintf("%d to rewrite, %d skipped", len(p.changes), len(p.skips))
		}
		_, _ = fmt.Fprintf(out, "%s (%s): %s: %s\n", m.ID, m.Release, m.Summary, state)
	}
}

func printMigrationPlan(out io.Writer, pending []pendingMigration, dryRun bool) {
	verb, rename := "rewrote", "renamed"
	if dryRun {
		verb, rename = "would rewrite", "would rename"
	}
	for _, p := range pending {
		_, _ = fmt.Fprintf(out, "%s: %s\n", p.ID, p.Summary)
		for _, c := range p.changes {
			switch {
			case c.NewPath != "" && c.Before == c.After:
				_, _ = fmt.Fprintf(out, "  %s %s -> %s\n", rename, filepath.ToSlash(c.Path), filepath.ToSlash(c.NewPath))
			default:
				_, _ = fmt.Fprintf(out, "  %s %s\n", verb, filepath.ToSlash(c.target()))
				if dryRun {
					_, _ = fmt.Fprint(out, indent(labeledDiff(filepath.ToSlash(c.Path), filepath.ToSlash(c.target()),
						redactMigrationLines(splitLines(c.Before)), redactMigrationLines(splitLines(c.After)), 200)))
				}
			}
		}
		for _, s := range p.skips {
			_, _ = fmt.Fprintf(out, "  skipped %s: %s\n", filepath.ToSlash(s.Path), s.Reason)
		}
	}
}

func (c migrationChange) target() string {
	if c.NewPath != "" {
		return c.NewPath
	}
	return c.Path
}

func indent(text string) string {
	if text == "" {
		return ""
	}
	return "    " + strings.ReplaceAll(strings.TrimSuffix(text, "\n"), "\n", "\n    ") + "\n"
}

// migrationValueLine is a YAML `key: value` line, in a spec body or in
// frontmatter.
var migrationValueLine = regexp.MustCompile(`^(\s*-?\s*"?([A-Za-z0-9_.-]+)"?\s*:\s*)(\S.*)$`)

// redactMigrationLines hides the values a diff could leak: a value under
// a credential-named key, or one import's detector reads as a credential.
// A ${NAME} reference stays, since it holds no secret.
func redactMigrationLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line
		m := migrationValueLine.FindStringSubmatch(line)
		value := ""
		if m != nil {
			value = strings.Trim(m[3], `"'`)
		}
		switch {
		case m != nil && (mcpCredentialKey(m[2]) || mcpCredentialDetected(value)) && !envRefOnly(value):
			out[i] = m[1] + "<redacted>"
		case m == nil && mcpCredentialDetected(line):
			out[i] = "<redacted>"
		}
	}
	return out
}

var migrationEnvRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*(:-[^}]*)?\}$`)

func envRefOnly(value string) bool { return migrationEnvRef.MatchString(value) }

// applyMigrations writes each change atomically and keeps the file mode.
// A rename writes the new path first and removes the old one after, so an
// interrupted run leaves both, which the loader resolves to the new one.
func applyMigrations(pending []pendingMigration) error {
	for _, p := range pending {
		for _, c := range p.changes {
			if err := writeMigrationChange(c); err != nil {
				return fmt.Errorf("migration %s: %w", p.ID, err)
			}
		}
	}
	return nil
}

func writeMigrationChange(c migrationChange) error {
	info, err := os.Stat(c.Path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	if string(current) != c.Before {
		return fmt.Errorf("%s changed since the plan; run migrate again", filepath.ToSlash(c.Path))
	}
	target := c.target()
	if c.NewPath != "" {
		if _, err := os.Lstat(c.NewPath); err == nil {
			return fmt.Errorf("%s already exists", filepath.ToSlash(c.NewPath))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".agnostic-ai-migrate-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(c.After); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return err
	}
	if c.NewPath != "" {
		return os.Remove(c.Path)
	}
	return nil
}

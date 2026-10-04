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

	"github.com/chemaclass/agnostic-ai/internal/config"
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
	// Note, when set, returns a line printed once after the migration's
	// rewrites, such as what the new form does not cover yet.
	Note func(root string) string
}

// migrationChange is one file edit: new content for Path, written to
// NewPath when the migration also renames the file. Remove deletes Path
// instead, once NewPath, the file that stays, still holds the same bytes,
// such as after an interrupted rename.
type migrationChange struct {
	Path    string
	NewPath string
	Before  string
	After   string
	Remove  bool
}

// migrationSkip is an entry a migration leaves as written. Actionable
// marks one the user should change by hand; the rest are fine as they
// are, so doctor does not ask about them.
type migrationSkip struct {
	Path       string
	Reason     string
	Actionable bool
}

// specMigrations is the registry, in release order. Each entry is
// idempotent: it plans nothing once its old form is gone.
var specMigrations = []specMigration{configFileNameMigration, hooksPortableEventsMigration}

// pendingMigration is a migration that applies here, or whose plan
// failed with planErr.
type pendingMigration struct {
	specMigration
	changes []migrationChange
	skips   []migrationSkip
	planErr error
	note    string
}

// needsUser reports whether the migration rewrites something or asks the
// user to change something by hand.
func (p pendingMigration) needsUser() bool {
	return p.planErr == nil && (len(p.changes) > 0 || slices.ContainsFunc(p.skips, func(s migrationSkip) bool { return s.Actionable }))
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
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			if _, _, err := config.ResolveConfigPath("."); err != nil {
				return err
			}
			selected, err := selectMigrations(cmd.Flags().Changed("only"), only)
			if err != nil {
				return err
			}
			pending := planMigrations(".", selected)
			out := cmd.OutOrStdout()
			if list {
				printMigrationList(out, selected, pending)
				return nil
			}
			quiet := verbosity < levelDefault
			if len(pending) == 0 {
				if !quiet {
					_, _ = fmt.Fprintln(out, "no migrations apply")
				}
				return nil
			}
			if dryRun {
				printMigrationPlan(out, pending, true, quiet)
				return planFailures(pending)
			}
			lock, err := acquireProjectLock(".", "migrate")
			if err != nil {
				return err
			}
			defer func() { _ = lock.Close() }()
			if err := applyMigrations(pending); err != nil {
				return err
			}
			printMigrationPlan(out, pending, false, quiet)
			return planFailures(pending)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the rewrites without writing them.")
	cmd.Flags().BoolVar(&list, "list", false, "List every migration and whether it applies here.")
	cmd.Flags().StringSliceVar(&only, "only", nil, "Run only these migration groups (comma-separated).")
	cmd.MarkFlagsMutuallyExclusive("list", "dry-run")
	return cmd
}

func selectMigrations(set bool, only []string) ([]specMigration, error) {
	if !set {
		return specMigrations, nil
	}
	var groups []string
	for _, m := range specMigrations {
		groups = append(groups, m.Group)
	}
	var out []specMigration
	if len(only) == 0 {
		only = []string{""}
	}
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

// planMigrations plans each selected migration. One whose plan fails is
// kept with its error, so the others still run.
func planMigrations(root string, selected []specMigration) []pendingMigration {
	var pending []pendingMigration
	for _, m := range selected {
		changes, skips, err := m.Plan(root)
		if err != nil || len(changes)+len(skips) > 0 {
			p := pendingMigration{specMigration: m, changes: changes, skips: skips, planErr: err}
			if err == nil && len(changes) > 0 && m.Note != nil {
				p.note = m.Note(root)
			}
			pending = append(pending, p)
		}
	}
	return pending
}

// planFailures is the command's error when a plan failed, after the
// other migrations ran.
func planFailures(pending []pendingMigration) error {
	var ids []string
	for _, p := range pending {
		if p.planErr != nil {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return fmt.Errorf("%d %s could not plan: %s", len(ids), migrationWord(len(ids), "migration", "migrations"), strings.Join(ids, ", "))
}

// planErrorText is a plan error on one line, with the values a parse
// error may quote redacted.
func planErrorText(err error) string { return migrationLine(err.Error()) }

// migrationLine is text for one output line, such as a skip reason that
// quotes a parse error, with the values it may quote redacted.
func migrationLine(text string) string {
	lines := redactMigrationLines(strings.Split(text, "\n"))
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, " ")
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
			if p.planErr != nil {
				state = "cannot plan: " + planErrorText(p.planErr)
			}
		}
		_, _ = fmt.Fprintf(out, "%s (%s): %s: %s\n", m.ID, m.Release, m.Summary, state)
	}
}

// printMigrationPlan prints each rewrite and skip; under -q only the skips,
// since they need the user.
func printMigrationPlan(out io.Writer, pending []pendingMigration, dryRun, quiet bool) {
	verb, rename, remove := "rewrote", "renamed", "removed"
	if dryRun {
		verb, rename, remove = "would rewrite", "would rename", "would remove"
	}
	for _, p := range pending {
		if p.planErr != nil {
			_, _ = fmt.Fprintf(out, "%s: cannot plan: %s\n", p.ID, planErrorText(p.planErr))
			continue
		}
		if quiet {
			for _, s := range p.skips {
				_, _ = fmt.Fprintf(out, "%s: skipped %s: %s\n", p.ID, filepath.ToSlash(s.Path), migrationLine(s.Reason))
			}
			continue
		}
		_, _ = fmt.Fprintf(out, "%s: %s\n", p.ID, p.Summary)
		for _, c := range p.changes {
			switch {
			case c.Remove:
				_, _ = fmt.Fprintf(out, "  %s %s\n", remove, filepath.ToSlash(c.Path))
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
			_, _ = fmt.Fprintf(out, "  skipped %s: %s\n", filepath.ToSlash(s.Path), migrationLine(s.Reason))
		}
		if p.note != "" {
			_, _ = fmt.Fprintf(out, "  note: %s\n", p.note)
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

// migrationValueLine is a YAML `key: value` line or list item, in a spec
// body or in frontmatter. A URL's scheme is not a key.
var migrationValueLine = regexp.MustCompile(`^(\s*-?\s*"?([A-Za-z0-9_.-]+)"?\s*:(?:\s+|$))(.*)$`)

var migrationListItem = regexp.MustCompile(`^(\s*-\s+)(.*)$`)

// redactMigrationLines hides the values a diff could leak. It reads YAML
// line by line, so it errs toward hiding: a value under a credential-named
// key, including the lines of a `|` or `>` block under one; a value import
// reads as a credential; a list item that follows a credential flag; and a
// flow sequence that holds either. A ${NAME} reference stays.
func redactMigrationLines(lines []string) []string {
	out := make([]string, len(lines))
	blockIndent := -1
	afterFlag := false
	for i, line := range lines {
		out[i] = line
		indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || indentWidth > blockIndent {
				out[i] = strings.Repeat(" ", indentWidth) + "<redacted>"
				continue
			}
			blockIndent = -1
		}
		if m := migrationValueLine.FindStringSubmatch(line); m != nil {
			afterFlag = false
			value := strings.Trim(strings.TrimSpace(m[3]), `"'`)
			switch {
			case mcpCredentialKey(m[2]) && strings.HasPrefix(value, "|") || mcpCredentialKey(m[2]) && strings.HasPrefix(value, ">"):
				blockIndent = indentWidth
				out[i] = m[1] + "<redacted>"
			case value == "" || envRefOnly(value):
			case mcpCredentialKey(m[2]) || migrationSecretText(value):
				out[i] = m[1] + "<redacted>"
			}
			continue
		}
		if m := migrationListItem.FindStringSubmatch(line); m != nil {
			item := strings.Trim(strings.TrimSpace(m[2]), `"'`)
			_, _, flagHasValue := mcpCredentialFlag(item)
			isFlag := strings.HasPrefix(item, "--") && mcpCredentialName(strings.SplitN(strings.TrimPrefix(item, "--"), "=", 2)[0])
			switch {
			case envRefOnly(item):
			case afterFlag || migrationSecretText(item) || isFlag && flagHasValue:
				out[i] = m[1] + "<redacted>"
			}
			afterFlag = isFlag && !flagHasValue
			continue
		}
		afterFlag = false
		if migrationSecretText(line) {
			out[i] = "<redacted>"
		}
	}
	return out
}

// migrationSecretText reports a value import's detector reads as a
// credential, reading a flow sequence item by item.
func migrationSecretText(value string) bool {
	if mcpCredentialDetected(value) {
		return true
	}
	if !strings.HasPrefix(value, "[") {
		return false
	}
	items := strings.Split(strings.Trim(value, "[]"), ",")
	for i, item := range items {
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		if mcpCredentialDetected(item) {
			return true
		}
		if _, _, hasValue := mcpCredentialFlag(item); hasValue && !envRefOnly(strings.SplitN(item, "=", 2)[1]) {
			return true
		}
		if strings.HasPrefix(item, "--") && mcpCredentialName(strings.TrimPrefix(item, "--")) && i+1 < len(items) && !envRefOnly(strings.Trim(strings.TrimSpace(items[i+1]), `"'`)) {
			return true
		}
	}
	return false
}

var migrationEnvRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*(:-[^}]*)?\}$`)

func envRefOnly(value string) bool { return migrationEnvRef.MatchString(value) }

// applyMigrations writes each change atomically and keeps the file mode.
// A rename writes the new path first and removes the old one after, so an
// interrupted run leaves both, which the loader resolves to the new one and
// the next run finishes.
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
	if c.Remove {
		kept, err := os.Lstat(c.NewPath)
		if err != nil || !kept.Mode().IsRegular() {
			return fmt.Errorf("%s is gone or no longer a regular file; run migrate again", filepath.ToSlash(c.NewPath))
		}
		if body, err := os.ReadFile(c.NewPath); err != nil || string(body) != c.Before {
			return fmt.Errorf("%s changed since the plan; run migrate again", filepath.ToSlash(c.NewPath))
		}
		return os.Remove(c.Path)
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
	_, werr := tmp.WriteString(c.After)
	if err := errors.Join(werr, tmp.Sync(), tmp.Close()); err != nil {
		return fmt.Errorf("%s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	if c.NewPath != "" {
		// A hard link fails when the target exists, so a file another
		// program created since the check above is never replaced. A
		// filesystem without hard links has no portable no-replace rename,
		// and writing the target in place could leave it half written.
		if err := os.Link(tmp.Name(), target); errors.Is(err, os.ErrExist) {
			return err
		} else if err != nil {
			return fmt.Errorf("%w; rename %s to %s by hand", err, filepath.ToSlash(c.Path), filepath.ToSlash(target))
		}
		// A save to the old file while the copy was written must not be
		// deleted with it.
		if now, err := os.ReadFile(c.Path); err != nil || string(now) != c.Before {
			_ = os.Remove(target)
			return fmt.Errorf("%s changed during the migration; run migrate again", filepath.ToSlash(c.Path))
		}
		return os.Remove(c.Path)
	}
	return os.Rename(tmp.Name(), target)
}

// pendingMigrationHint is the line doctor and upgrade --requires print
// when migrations apply here, or "" when none do. A migration whose plan
// fails is left out. One that only skips needs a manual step when a skip
// is actionable, and says so; other skips need nothing.
func pendingMigrationHint(root string) string {
	var apply, manual []string
	for _, p := range planMigrations(root, specMigrations) {
		switch {
		case !p.needsUser():
		case len(p.changes) > 0:
			apply = append(apply, p.ID)
		default:
			manual = append(manual, p.ID)
		}
	}
	if len(apply)+len(manual) == 0 {
		return ""
	}
	var parts []string
	if len(apply) > 0 {
		parts = append(parts, fmt.Sprintf("%d spec %s (%s)", len(apply), migrationWord(len(apply), "migration applies", "migrations apply"), strings.Join(apply, ", ")))
	}
	if len(manual) > 0 {
		parts = append(parts, fmt.Sprintf("%d spec %s a manual step (%s)", len(manual), migrationWord(len(manual), "migration needs", "migrations need"), strings.Join(manual, ", ")))
	}
	return strings.Join(parts, "; ") + ". Preview: agnostic-ai migrate --dry-run"
}

func migrationWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

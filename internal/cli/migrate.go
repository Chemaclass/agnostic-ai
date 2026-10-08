package cli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// specMigration rewrites one old spec form into its replacement without
// changing what sync writes for the targets a spec already reaches. Plan
// reads the scope and returns the edits plus a reason for each entry it
// leaves alone; it never writes.
type specMigration struct {
	ID      string
	Group   string
	Release string
	Summary string
	// ProjectOnly marks an old form the global home never had, such as
	// the legacy config file name.
	ProjectOnly bool
	Plan        func(s migrationScope) ([]migrationChange, []migrationSkip, error)
	// Note, when set, returns a line printed once after the migration's
	// rewrites, such as what the new form does not cover yet.
	Note func(s migrationScope) string
}

// migrationScope is the spec tree a migration reads and writes: the
// project at root, or the global home at root that sync --global reads.
type migrationScope struct {
	root   string
	global bool
	// project, when set, holds the project at root once loaded, so the
	// migrations of one plan read it once instead of each loading it.
	project *loadedProject
}

// loadedProject is a project loaded once, plus each of its layers loaded
// on its own once. Its methods are safe for concurrent use: doctor plans
// migrations while lint reads the same project.
type loadedProject struct {
	once   sync.Once
	cfg    *config.Config
	bundle spec.Bundle
	err    error

	mu     sync.Mutex
	layers map[string]spec.Bundle
}

// projectMigrationScope is the project at root, loaded on first use.
func projectMigrationScope(root string) migrationScope {
	return migrationScope{root: root, project: &loadedProject{}}
}

// loadedMigrationScope is the project at root, already loaded as cfg and b.
func loadedMigrationScope(root string, cfg *config.Config, b spec.Bundle) migrationScope {
	p := &loadedProject{cfg: cfg, bundle: b}
	p.once.Do(func() {})
	return migrationScope{root: root, project: p}
}

// loadProject loads the project at root, once per scope when it holds
// one.
func (s migrationScope) loadProject() (*config.Config, spec.Bundle, error) {
	p := s.project
	if p == nil {
		return loadProject(s.root)
	}
	p.once.Do(func() { p.cfg, p.bundle, p.err = loadProject(s.root) })
	return p.cfg, p.bundle, p.err
}

// loadLayer loads one layer on its own, once per scope when it holds a
// project.
func (s migrationScope) loadLayer(layer spec.Layer) (spec.Bundle, error) {
	p := s.project
	if p == nil {
		return spec.LoadLayered([]spec.Layer{layer})
	}
	key := layer.Name + "\x00" + layer.Root
	p.mu.Lock()
	defer p.mu.Unlock()
	if b, ok := p.layers[key]; ok {
		return b, nil
	}
	b, err := spec.LoadLayered([]spec.Layer{layer})
	if err != nil {
		return b, err
	}
	if p.layers == nil {
		p.layers = map[string]spec.Bundle{}
	}
	p.layers[key] = b
	return b, nil
}

// migrationChange is one file edit: new content for Path, written to
// NewPath when the migration also renames the file. Remove deletes Path
// instead, once NewPath, the file that stays, still holds the same bytes,
// such as after an interrupted rename. realPath is the file Path resolved
// to when the registry checked it, so a symlink retargeted since then
// fails instead of writing somewhere unchecked.
type migrationChange struct {
	Path     string
	NewPath  string
	Before   string
	After    string
	Remove   bool
	realPath string
}

// migrationSkip is an entry a migration leaves as written. Actionable
// marks one the user should change by hand; the rest are fine as they
// are, so doctor does not ask about them. Pack names the pack that holds
// the file, which --list tells the user to update.
type migrationSkip struct {
	Path       string
	Reason     string
	Actionable bool
	Pack       string
}

// packSkip is a skip for a file in a pack, which migrate never rewrites.
func packSkip(path, pack string) migrationSkip {
	return migrationSkip{Path: path, Pack: pack, Reason: "is in pack " + pack + ", which migrate never rewrites; update the pack once its author migrates it"}
}

// specMigrations is the registry, in release order. Each entry is
// idempotent: it plans nothing once its old form is gone.
var specMigrations = []specMigration{configFileNameMigration, hooksPortableEventsMigration, secretsMCPLiteralsMigration, capabilitiesAgentToolsMigration, capabilitiesSettingsPermissionsMigration, capabilitiesSkillToolsMigration}

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
	var dryRun, list, global bool
	var only []string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Rewrite old spec forms into their current replacements.",
		Long: "migrate applies every pending spec migration: a rewrite of an old spec\n" +
			"form, such as a renamed field or file, into its replacement. A migration\n" +
			"never changes what sync writes for the targets a spec already reaches,\n" +
			"so `sync --check` stays clean after it, except where a literal MCP\n" +
			"credential becomes a ${NAME} reference. Old forms keep working, so\n" +
			"running it is never required to sync.",
		Example: `  # Show which migrations apply to this project
  agnostic-ai migrate --list

  # Preview the rewrites
  agnostic-ai migrate --dry-run

  # Apply every pending migration, or one group
  agnostic-ai migrate
  agnostic-ai migrate --only config

  # Rewrite the global specs sync --global reads
  agnostic-ai migrate --global`,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := resolveMigrationScope(global)
			if err != nil {
				return err
			}
			selected, err := selectMigrations(cmd.Flags().Changed("only"), only)
			if err != nil {
				return err
			}
			if global {
				selected = slices.DeleteFunc(slices.Clone(selected), func(m specMigration) bool { return m.ProjectOnly })
			}
			pending := planMigrations(scope, selected)
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
			if !global {
				lock, err := acquireProjectLock(".", "migrate")
				if err != nil {
					return err
				}
				defer func() { _ = lock.Close() }()
			}
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
	cmd.Flags().BoolVar(&global, "global", false, "Rewrite the global specs in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai) and its local/ layer, checked against the targets sync --global writes.")
	cmd.MarkFlagsMutuallyExclusive("list", "dry-run")
	return cmd
}

// resolveMigrationScope is the global home with --global, else the
// project in the working directory.
func resolveMigrationScope(global bool) (migrationScope, error) {
	if global {
		source, err := globalSourceRoot()
		if err != nil {
			return migrationScope{}, err
		}
		return migrationScope{root: source, global: true}, nil
	}
	if err := refuseGlobalHome(".", globalHomeMigrateRemedy); err != nil {
		return migrationScope{}, err
	}
	if _, _, err := config.ResolveConfigPath("."); err != nil {
		return migrationScope{}, err
	}
	return projectMigrationScope("."), nil
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

// loadSpecs loads the specs the scope syncs and the layers they come
// from: the project with its packs and local/ layer, or the global home
// and its local/ layer.
func (s migrationScope) loadSpecs() (spec.Bundle, []spec.Layer, error) {
	if s.global {
		return s.loadGlobalSpecs()
	}
	cfg, b, err := s.loadProject()
	if err != nil {
		return spec.Bundle{}, nil, err
	}
	layers, err := resolveLayers(s.root, cfg)
	return withoutBuiltinEntries(b), withoutBuiltinLayers(layers), err
}

// loadGlobalSpecs loads the global layers as sync --global reads them,
// after the home config's requires holds.
func (s migrationScope) loadGlobalSpecs() (spec.Bundle, []spec.Layer, error) {
	if err := requireGlobalVersion(s.root, nil); err != nil {
		return spec.Bundle{}, nil, err
	}
	names, err := loadGlobalBuiltins(s.root, nil)
	if err != nil {
		return spec.Bundle{}, nil, err
	}
	layers, err := globalLayers(s.root, names)
	if err != nil {
		return spec.Bundle{}, nil, err
	}
	layers = withoutBuiltinLayers(layers)
	b, err := spec.LoadLayered(layers)
	return b, layers, err
}

// planMigrations plans each selected migration. One whose plan fails is
// kept with its error, so the others still run.
func planMigrations(s migrationScope, selected []specMigration) []pendingMigration {
	var pending []pendingMigration
	for _, m := range selected {
		changes, skips, err := m.Plan(s)
		if err == nil {
			var outside []migrationSkip
			changes, outside = s.keepInSpecRoots(changes)
			skips = append(skips, outside...)
		}
		if err != nil || len(changes)+len(skips) > 0 {
			p := pendingMigration{specMigration: m, changes: changes, skips: skips, planErr: err}
			if err == nil && len(changes) > 0 && m.Note != nil {
				p.note = m.Note(s)
			}
			pending = append(pending, p)
		}
	}
	return pending
}

// keepInSpecRoots moves each change whose file resolves outside the
// scope's spec roots, such as a symlink into a pack, to the skips. The
// registry checks this once, so no migration can write there.
func (s migrationScope) keepInSpecRoots(changes []migrationChange) ([]migrationChange, []migrationSkip) {
	roots, packs := s.specRoots()
	var kept []migrationChange
	var skips []migrationSkip
	for _, c := range changes {
		if skip, outside := s.outsideSpecRoots(c, roots, packs); outside {
			skips = append(skips, skip)
			continue
		}
		if c.NewPath == "" {
			c.realPath, _ = migrationRealPath(c.Path)
		}
		kept = append(kept, c)
	}
	return kept, skips
}

// specRoots resolves the directories a migration may write in, and the
// directory of each pack by name. Packs sit inside the project, but
// migrate never rewrites them. A global home kept as a project's source
// dir holds its packs under packs/.
func (s migrationScope) specRoots() ([]string, map[string]string) {
	local := filepath.Join(s.root, defaultProjectUser)
	packDirs := []string{filepath.Join(s.root, packsDir)}
	if s.global {
		local = filepath.Join(s.root, "local")
		packDirs = append(packDirs, filepath.Join(s.root, filepath.Base(packsDir)))
	}
	var roots []string
	for _, dir := range []string{s.root, local} {
		if real, err := migrationRealPath(dir); err == nil {
			roots = append(roots, real)
		}
	}
	packs := map[string]string{}
	for _, dir := range packDirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if real, err := migrationRealPath(filepath.Join(dir, e.Name())); err == nil {
				packs[e.Name()] = real
			}
		}
	}
	return roots, packs
}

// migrationRealPath is path made absolute with every symlink resolved,
// so a working directory reached through a symlink still compares equal.
func migrationRealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// outsideSpecRoots returns the skip for a change whose file, or the
// directory a rename writes into, resolves into a pack or outside roots.
func (s migrationScope) outsideSpecRoots(c migrationChange, roots []string, packs map[string]string) (migrationSkip, bool) {
	paths := []string{c.Path}
	if c.NewPath != "" && !c.Remove {
		paths = append(paths, filepath.Dir(c.NewPath))
	}
	where := "the project"
	if s.global {
		where = "the global home"
	}
	for _, path := range paths {
		real, err := migrationRealPath(path)
		if err != nil {
			return migrationSkip{Path: c.Path, Reason: "cannot resolve its real path: " + err.Error()}, true
		}
		for _, name := range slices.Sorted(maps.Keys(packs)) {
			if pathWithin(packs[name], real) {
				return packSkip(c.Path, name), true
			}
		}
		if !slices.ContainsFunc(roots, func(root string) bool { return pathWithin(root, real) }) {
			return migrationSkip{Path: c.Path, Reason: "resolves outside " + where}, true
		}
	}
	return migrationSkip{}, false
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
			if packs := skippedPacks(p.skips); len(packs) > 0 {
				state += fmt.Sprintf("; update %s %s", migrationWord(len(packs), "pack", "packs"), strings.Join(packs, ", "))
			}
			if p.planErr != nil {
				state = "cannot plan: " + planErrorText(p.planErr)
			}
		}
		_, _ = fmt.Fprintf(out, "%s (%s): %s: %s\n", m.ID, m.Release, m.Summary, state)
	}
}

func skippedPacks(skips []migrationSkip) []string {
	var packs []string
	for _, s := range skips {
		if s.Pack != "" {
			packs = append(packs, s.Pack)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(packs)))
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
var migrationValueLine = regexp.MustCompile(`^(\s*-?\s*["']?([A-Za-z0-9_.-]+)["']?\s*:(?:\s+|$))(.*)$`)

var migrationListItem = regexp.MustCompile(`^(\s*-\s+)(.*)$`)

// migrationNodeProps is the anchor and tag a YAML value may start with,
// such as `&a` or `!!seq`, which leave the value itself on the next lines.
var migrationNodeProps = regexp.MustCompile(`^(?:[&!]\S*(?:\s+|$))+`)

// migrationValueKeys hold values a diff prints only as a reference: each
// value under env: or headers:, and each item of args:.
var migrationValueKeys = map[string]bool{"env": true, "headers": true, "args": true}

var migrationURL = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// redactMigrationLines hides the values a diff could leak. It reads YAML
// line by line, so it errs toward hiding: every value under env: or
// headers:, every item of args:, and every URL, including a flow value
// that runs over the next lines; a value under a credential-named key,
// including the lines of a `|` or `>` block under one; a value import
// reads as a credential; a list item that follows a credential flag; and
// a flow sequence that holds either. A ${NAME} reference, with or without
// `Bearer `, and a `!literal` tag stay.
func redactMigrationLines(lines []string) []string {
	out := make([]string, len(lines))
	blockIndent, valuesIndent := -1, -1
	afterFlag := false
	for i, line := range lines {
		out[i] = line
		trimmed := strings.TrimSpace(line)
		indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))
		if blockIndent >= 0 {
			if trimmed == "" || indentWidth > blockIndent {
				out[i] = strings.Repeat(" ", indentWidth) + "<redacted>"
				continue
			}
			blockIndent = -1
		}
		if valuesIndent >= 0 {
			// A block sequence may sit at its key's own indent.
			if trimmed == "" || indentWidth > valuesIndent || indentWidth == valuesIndent && strings.HasPrefix(trimmed, "-") {
				out[i] = redactedValueLine(line, indentWidth)
				continue
			}
			valuesIndent = -1
		}
		if m := migrationValueLine.FindStringSubmatch(line); m != nil {
			afterFlag = false
			tag, value := migrationLineValue(m[3])
			switch {
			case migrationValueKeys[m[2]]:
				// The value, or the rest of a flow value, may sit on the
				// lines below.
				valuesIndent = strings.Index(line, m[2])
				if value != "" && !migrationRefOnly(value) {
					out[i] = m[1] + tag + "<redacted>"
				}
			case mcpCredentialKey(m[2]) && (value == "" || strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")):
				blockIndent = indentWidth
				if value != "" {
					out[i] = m[1] + tag + "<redacted>"
				}
			case value == "" || migrationRefOnly(value):
			case mcpCredentialKey(m[2]) || migrationSecretText(value) || migrationURLText(value):
				out[i] = m[1] + tag + "<redacted>"
			}
			continue
		}
		if m := migrationListItem.FindStringSubmatch(line); m != nil {
			item := strings.Trim(strings.TrimSpace(m[2]), `"'`)
			_, _, flagHasValue := mcpCredentialFlag(item)
			isFlag := strings.HasPrefix(item, "--") && mcpCredentialName(strings.SplitN(strings.TrimPrefix(item, "--"), "=", 2)[0])
			switch {
			case migrationRefOnly(item):
			case afterFlag || migrationSecretText(item) || migrationURLText(item) || isFlag && flagHasValue:
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

// migrationLineValue splits a YAML value into its `!literal` tag, which a
// diff keeps, and the value without quotes, anchors, other tags, or a
// trailing-only comment.
func migrationLineValue(text string) (tag, value string) {
	tag, raw := cutLiteralTag(strings.TrimSpace(text))
	raw = migrationNodeProps.ReplaceAllString(raw, "")
	if strings.HasPrefix(raw, "#") {
		return tag, ""
	}
	return tag, strings.Trim(raw, `"'`)
}

// migrationRefOnly reports a ${NAME} reference, alone or after `Bearer `,
// with no default that could hold a value.
func migrationRefOnly(value string) bool {
	return envRefOnly(strings.TrimPrefix(value, "Bearer ")) && !strings.Contains(value, ":-")
}

// redactedValueLine is a line under env:, headers:, or args: with its
// value hidden unless it is a reference.
func redactedValueLine(line string, indentWidth int) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	if m := migrationValueLine.FindStringSubmatch(line); m != nil {
		tag, value := migrationLineValue(m[3])
		if value == "" || migrationRefOnly(value) {
			return line
		}
		return m[1] + tag + "<redacted>"
	}
	if m := migrationListItem.FindStringSubmatch(line); m != nil {
		if migrationRefOnly(strings.Trim(strings.TrimSpace(m[2]), `"'`)) {
			return line
		}
		return m[1] + "<redacted>"
	}
	return strings.Repeat(" ", indentWidth) + "<redacted>"
}

// migrationURLText reports a URL, or a flow collection that holds one.
func migrationURLText(value string) bool {
	if migrationURL.MatchString(value) {
		return true
	}
	return (strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{")) && strings.Contains(value, "://")
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

// cutLiteralTag splits a leading `!literal ` tag off a YAML value.
func cutLiteralTag(value string) (tag, rest string) {
	if rest, ok := strings.CutPrefix(value, spec.LiteralTag+" "); ok {
		return spec.LiteralTag + " ", strings.TrimSpace(rest)
	}
	return "", value
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
	if c.NewPath == "" {
		// Writing the file a symlink resolves to keeps the symlink.
		if target, err = migrationRealPath(c.Path); err != nil {
			return err
		}
		if c.realPath != "" && target != c.realPath {
			return fmt.Errorf("%s now resolves to %s, not the file the plan checked; run migrate again", filepath.ToSlash(c.Path), filepath.ToSlash(target))
		}
	} else if _, err := os.Lstat(c.NewPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.ToSlash(c.NewPath))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
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
func pendingMigrationHint(s migrationScope) string {
	var apply, manual []string
	for _, p := range planMigrations(s, specMigrations) {
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

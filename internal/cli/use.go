package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <tool>...",
		Short: "Start using an AI tool with what the project already has, in one step.",
		Long: "Sets the project up when it has no agnostic-ai.yaml yet, importing the instructions, " +
			"skills, agents, hooks, and MCP servers of the tools it already uses. Adds each named tool " +
			"to targets, imports that tool's own existing config first, syncs, and shows what the " +
			"tool now reads.",
		Example: `  # Switching to Codex this month
  agnostic-ai use codex

  # A team on several tools
  agnostic-ai use claude codex cursor`,
		Args:      cobra.MinimumNArgs(1),
		ValidArgs: adapters.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseGlobalHome(".", globalHomeSpecsRemedy); err != nil {
				return err
			}
			tools, err := knownTools(args)
			if err != nil {
				return err
			}
			added, err := useTools(cmd, tools)
			if err != nil {
				return err
			}
			listedBefore := readStateFile(".").Listed
			// Always sync, a no-op when nothing changed, so a run that
			// stopped halfway finishes on the next try.
			if err := runSyncPass(".", nil, false, false, false, false, "", 0); err != nil {
				return err
			}
			if len(added) == 0 {
				summaryf("%s %s already in use; edit .agnostic-ai/ and run agnostic-ai sync to change what it reads\n", tick(), strings.Join(tools, ", "))
				return nil
			}
			// The sync lists a tool the first time it writes for it, so use
			// lists only the added tools it did not.
			listedNow := readStateFile(".").Listed
			added = slices.DeleteFunc(added, func(t string) bool {
				return slices.Contains(listedNow, t) && !slices.Contains(listedBefore, t)
			})
			if len(added) == 0 || verbosity < levelDefault {
				return nil
			}
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			printToolReads(logOut, cfg, b, added)
			return nil
		},
	}
}

// knownTools checks each name against the built-in targets, with a
// did-you-mean for a likely typo.
func knownTools(args []string) ([]string, error) {
	var tools []string
	for _, a := range args {
		if !slices.Contains(adapters.Names(), a) {
			if s := suggest.Name(a, adapters.Names()); s != "" {
				return nil, errs.Coded(errs.CodeSyncTargetUnknown, "unknown tool %q (did you mean %s?)", a, s)
			}
			return nil, errs.Coded(errs.CodeSyncTargetUnknown, "unknown tool %q; see https://agnostic-ai.org/docs/targets/", a)
		}
		if !slices.Contains(tools, a) {
			tools = append(tools, a)
		}
	}
	return tools, nil
}

// useTools sets the project up or extends it so it emits for tools, and
// returns the tools it added. A new project enables the tools it detects
// too and imports what they already have. An existing one imports each
// added tool's own config before the sync would write over it.
func useTools(cmd *cobra.Command, tools []string) ([]string, error) {
	path, _, err := config.ResolveConfigPath(".")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && errs.CodeOf(err) != errs.CodeConfigMissing {
			return nil, err
		}
		if err := refuseNestedProject(); err != nil {
			return nil, err
		}
		return tools, startProject(cmd, tools)
	}
	cfg, err := config.Load(".")
	if err != nil {
		return nil, err
	}
	var added []string
	for _, t := range tools {
		if !slices.Contains(cfg.Targets, t) {
			added = append(added, t)
		}
	}
	// Tools an interrupted run added before importing them count as new.
	pending := readStateFile(".").PendingImports
	importing := slices.Clone(added)
	for _, t := range pending {
		if slices.Contains(cfg.Targets, t) && !slices.Contains(importing, t) {
			importing = append(importing, t)
		}
	}
	if len(added) > 0 {
		// The local file's targets win over the committed list, so a
		// tool added to the committed list would never sync.
		if localSetsTargets() {
			return nil, fmt.Errorf("%s sets targets, which win over %s; add %s there", config.LocalOverrideFileName, filepath.Base(path), strings.Join(added, ", "))
		}
		if err := refuseUnmanagedImport(cfg, added, false); err != nil {
			return nil, err
		}
		if err := setPendingImports(importing); err != nil {
			return nil, err
		}
		if err := config.PersistTargets(".", append(slices.Clone(cfg.Targets), added...)); err != nil {
			return nil, fmt.Errorf("add %s to targets: %w", strings.Join(added, ", "), err)
		}
		summaryf("→ added %s to targets in %s\n", strings.Join(added, ", "), filepath.Base(path))
		if cfg, err = config.Load("."); err != nil {
			return nil, err
		}
	}
	// Only an added tool's own config is imported, so a rerun never
	// replaces spec edits with native config imported before. Another
	// target's hand-written instructions file would stop the sync, so it
	// is imported too.
	var sources []string
	for _, t := range cfg.Targets {
		if slices.Contains(importing, t) && hasOwnConfig(cfg, t) || uncapturedInstructions(cfg, t) {
			sources = append(sources, t)
		}
	}
	failed, err := importToolConfig(cfg, sources)
	failedNew := intersect(failed, importing)
	if err != nil {
		err = leaveOut(cfg, failedNew, err)
	}
	if perr := setPendingImports(stillConfigured(failedNew)); perr != nil {
		return nil, errors.Join(err, perr)
	}
	if err != nil {
		return nil, err
	}
	return added, nil
}

// uncapturedInstructions reports whether target's instructions file holds
// hand-written text AGNOSTIC_AI.md does not have, which sync stops on. An
// unmanaged file stays its tool's own, so it is never imported.
func uncapturedInstructions(cfg *config.Config, target string) bool {
	path := adapters.EntryPointPath(cfg, target)
	if path == "" || cfg.IsUnmanaged(path) {
		return false
	}
	captured := ""
	if data, err := os.ReadFile(adapters.AgnosticEntryPointPath); err == nil {
		captured = header.Strip(string(data))
	}
	held, err := heldInstructions(captured)
	if err != nil {
		return false
	}
	uncaptured, err := handWrittenUncaptured(path, held)
	return err == nil && uncaptured
}

// refuseUnmanagedImport stops `use` from adding a tool whose own config
// exists while its instructions file is in sync.unmanaged: the importer
// would copy that file to every tool, and skipping the import would let
// the sync write over the tool's other files.
// A new project has no config for `import` to run against yet, so its
// remedy starts with `init`.
func refuseUnmanagedImport(cfg *config.Config, added []string, newProject bool) error {
	detected := detectExistingTargets(".")
	first := ""
	if newProject {
		first = "run agnostic-ai init, then "
	}
	for _, t := range added {
		path := adapters.EntryPointPath(cfg, t)
		if path != "" && cfg.IsUnmanaged(path) && slices.Contains(detected, t) {
			return fmt.Errorf("%s is in sync.unmanaged, so use cannot import %s without copying it to every tool; "+
				"%sagnostic-ai import %s, remove what only %s should read from .agnostic-ai/AGNOSTIC_AI.md, and add %s to targets",
				path, t, first, t, t, t)
		}
	}
	return nil
}

// localSetsTargets reports whether agnostic-ai.local.yaml sets targets.
func localSetsTargets() bool {
	data, err := os.ReadFile(config.LocalOverrideFileName)
	if err != nil {
		return false
	}
	// Key presence, not value: `targets: null` still replaces the list.
	var local map[string]any
	if yaml.Unmarshal(data, &local) != nil {
		return false
	}
	_, ok := local["targets"]
	return ok
}

// refuseNestedProject stops `use` from starting a second project inside
// one an enclosing directory already holds, up to the Git root, or up to
// the filesystem root outside Git.
func refuseNestedProject() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, _ := filepath.EvalSymlinks(cwd)
	top := ""
	if root, err := gitRevParse(cwd, "--show-toplevel"); err == nil {
		top, _ = filepath.EvalSymlinks(root)
	}
	for dir != top && filepath.Dir(dir) != dir {
		dir = filepath.Dir(dir)
		if _, _, err := config.ResolveConfigPath(dir); err == nil {
			return fmt.Errorf("this directory is inside the agnostic-ai project at %s; run agnostic-ai use there", dir)
		}
	}
	return nil
}

// startProject writes agnostic-ai.yaml for the detected tools plus
// tools, then imports everything the project already has.
func startProject(cmd *cobra.Command, tools []string) (err error) {
	if localSetsTargets() {
		return fmt.Errorf("%s sets targets, which win over %s; add %s there", config.LocalOverrideFileName, config.ConfigFileName, strings.Join(tools, ", "))
	}
	// A start that leaves no config takes back the state file it made, so
	// a later init and first sync see a project with no ledger.
	if _, serr := os.Stat(stateFilePath(".")); errors.Is(serr, os.ErrNotExist) {
		defer func() {
			if _, cerr := os.Stat(config.ConfigFileName); err != nil && errors.Is(cerr, os.ErrNotExist) {
				_ = os.Remove(stateFilePath("."))
			}
		}()
	}
	detected := detectExistingTargets(".")
	targets := slices.Clone(detected)
	for _, t := range tools {
		if !slices.Contains(targets, t) {
			targets = append(targets, t)
		}
	}
	gitignoreOn, err := promptGitignoreEnable(cmd.InOrStdin())
	if err != nil {
		return err
	}
	if err := setPendingImports(targets); err != nil {
		return err
	}
	if err := scaffoldSilently(scaffoldOptions{
		Root:             ".",
		Targets:          targets,
		GitignoreEnabled: gitignoreOn,
		Version:          cmd.Root().Version,
	}); err != nil {
		return err
	}
	summaryf("→ created %s with targets: %s\n", config.ConfigFileName, strings.Join(targets, ", "))
	if !stdinIsTerminal(cmd.InOrStdin()) {
		summaryf("  generated files are git-ignored, so each clone needs agnostic-ai sync (set gitignore.enabled: false in %s to commit them)\n", config.ConfigFileName)
	}
	cfg, err := config.Load(".")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// agnostic-ai.local.yaml can list an instructions file as unmanaged
	// before the project exists.
	if err := refuseUnmanagedImport(cfg, targets, true); err != nil {
		return errors.Join(err, os.Remove(config.ConfigFileName), setPendingImports(nil))
	}
	var sources []string
	for _, t := range targets {
		if hasOwnConfig(cfg, t) {
			sources = append(sources, t)
		}
	}
	failed, err := importToolConfig(cfg, sources)
	if err != nil {
		err = leaveOut(cfg, failed, err)
	}
	if perr := setPendingImports(stillConfigured(failed)); perr != nil {
		return errors.Join(err, perr)
	}
	if err != nil {
		return err
	}
	return nil
}

// leaveOut takes the tools whose import failed back out of targets, so a
// retry of `use` for them imports again, while the tools that imported
// stay and are not imported over later spec edits. With no target left,
// it removes the config, so a retry starts the project again.
func leaveOut(cfg *config.Config, failed []string, err error) error {
	if len(failed) == 0 {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(cfg.Targets), func(t string) bool {
		return slices.Contains(failed, t)
	})
	if len(kept) == 0 {
		if rerr := os.Remove(config.ConfigFileName); rerr != nil {
			return errors.Join(err, rerr)
		}
		return fmt.Errorf("%w (removed %s; fix this and run agnostic-ai use again)", err, config.ConfigFileName)
	}
	if rerr := config.PersistTargets(".", kept); rerr != nil {
		return errors.Join(err, rerr)
	}
	names := strings.Join(failed, " ")
	return fmt.Errorf("%w (left %s out of targets; fix this and run agnostic-ai use %s)", err, strings.Join(failed, ", "), names)
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// hasOwnConfig reports whether the project already holds config for
// target that sync would otherwise write over: a marker init detects, or
// its instructions file written by hand. A tool whose instructions file
// is in sync.unmanaged has none.
func hasOwnConfig(cfg *config.Config, target string) bool {
	path := adapters.EntryPointPath(cfg, target)
	// The importer reads the instructions file even when unmanaged, so a
	// tool whose file stays its own is not imported at all.
	if path != "" && cfg.IsUnmanaged(path) {
		return false
	}
	if slices.Contains(detectExistingTargets("."), target) {
		return true
	}
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) != "" && !header.Has(string(data))
}

// importToolConfig imports each tool that has an importer, one at a
// time, and returns the ones that failed. A tool without one, such as
// jules, reads the root AGENTS.md, which is folded in for every run.
func importToolConfig(cfg *config.Config, tools []string) (failed []string, err error) {
	var sources []string
	for _, t := range tools {
		if _, rulesDir := rulesDirImporters[t]; rulesDir || slices.Contains(importSourceNames, t) {
			sources = append(sources, t)
		}
	}
	// A hand-written root AGENTS.md no source above reads, such as a
	// Codex setup with no .codex/ folder, is folded in as `import all`
	// does; text already held is left alone, and an unmanaged one stays
	// its tool's own.
	defer func() {
		if err == nil && !cfg.IsUnmanaged(claudeAgentsMainFile) {
			_, err = foldRootAgentsMainFile(".")
		}
	}()
	if len(sources) == 0 {
		return nil, nil
	}
	importNextStepsOff = true
	defer func() { importNextStepsOff = false }()
	setImportRunSources(sources)
	defer setImportRunSources(nil)
	run := func(failed *[]string) error {
		return withImportTree(".", func() error {
			return withLocalImportGuard(".", cfg, func() error {
				for _, s := range sources {
					if len(sources) > 1 {
						_, _ = fmt.Fprintf(os.Stdout, "→ importing from %s\n", s)
					}
					if err := runImport(".", s, cfg); err != nil {
						_, _ = fmt.Fprintf(os.Stderr, "! %s: %v\n", s, err)
						*failed = append(*failed, s)
					}
				}
				if len(*failed) > 0 {
					return fmt.Errorf("import failed for: %s", strings.Join(*failed, ", "))
				}
				return nil
			})
		})
	}
	err = runGuardedImport(false, importOverwriteRemedy, func() error { return run(&failed) })
	if errs.CodeOf(err) == errs.CodeImportWouldReplace {
		return sources, err
	}
	if err != nil && len(failed) == 0 {
		failed = sources
	}
	return failed, err
}

// printToolReads shows what each tool now reads from .agnostic-ai/: its
// instructions file, then each kind of spec it got, with where it lives.
func printToolReads(w io.Writer, cfg *config.Config, b spec.Bundle, tools []string) {
	for _, t := range tools {
		mine := b.For(t)
		_, _ = fmt.Fprintf(w, "%s %s now reads, from .agnostic-ai/:\n", tick(), t)
		if path := adapters.EntryPointPath(cfg, t); path != "" {
			_, _ = fmt.Fprintf(w, "    %-16s %s\n", "instructions", path)
		}
		// Where the tool keeps each kind, when its adapter says so.
		where := map[string]string{}
		for _, a := range adapters.NativeArtifactsFor(t, cfg) {
			if _, seen := where[strings.ToLower(a.Label)]; !seen {
				where[strings.ToLower(a.Label)] = a.Location
			}
		}
		// A kind the adapter does not declare is skipped with a warning,
		// so the tool does not read it. A plugin adapter declares none and
		// gets the whole bundle.
		var supports []spec.Kind
		if a, err := adapters.Resolve(t); err == nil {
			supports = a.Capabilities()
		}
		for _, kind := range []string{"Rules", "Skills", "Agents", "Commands", "Hooks", "MCP servers"} {
			entries := entriesFor(mine, kind)
			if len(entries) == 0 || supports != nil && !slices.Contains(supports, kindOf(kind)) {
				continue
			}
			label := fmt.Sprintf("%d %s", len(entries), countLabel(kind, len(entries)))
			_, _ = fmt.Fprintf(w, "    %-16s %-22s %s\n", label, where[strings.ToLower(kind)], entryNames(entries))
		}
	}
	_, _ = fmt.Fprintln(w, "  edit .agnostic-ai/ and run agnostic-ai sync to change what every tool reads")
}

// countLabel lowercases a native artifact label for a count, keeping an
// acronym such as MCP, and drops the plural for one.
func countLabel(label string, n int) string {
	if !strings.HasPrefix(label, "MCP") {
		label = strings.ToLower(label)
	}
	if n == 1 {
		label = strings.TrimSuffix(label, "s")
	}
	return label
}

// kindOf returns the spec kind a native artifact label lists.
func kindOf(label string) spec.Kind {
	switch strings.ToLower(label) {
	case "skills":
		return spec.KindSkill
	case "agents":
		return spec.KindAgent
	case "rules":
		return spec.KindRule
	case "commands":
		return spec.KindCommand
	case "hooks":
		return spec.KindHook
	case "mcp servers":
		return spec.KindMCP
	}
	return ""
}

// entriesFor returns the specs a native artifact label lists.
func entriesFor(b spec.Bundle, label string) []spec.Entry {
	switch strings.ToLower(label) {
	case "skills":
		return b.Skills
	case "agents":
		return b.Agents
	case "rules":
		return b.Rules
	case "commands":
		return b.Commands
	case "hooks":
		return b.Hooks
	case "mcp servers":
		return b.MCPs
	}
	return nil
}

func entryNames(entries []spec.Entry) string {
	const shown = 3
	names := make([]string, 0, shown)
	for i, e := range entries {
		if i == shown {
			return strings.Join(names, ", ") + fmt.Sprintf(", +%d", len(entries)-shown)
		}
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

// stopOnPendingImports stops a sync while `use` has tools in targets
// whose own config it has not imported yet. A pending tool no longer in
// targets has nothing for the sync to write over.
func stopOnPendingImports(root string, cfg *config.Config) error {
	var pending []string
	for _, t := range readStateFile(root).PendingImports {
		if slices.Contains(cfg.Targets, t) {
			pending = append(pending, t)
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("agnostic-ai use stopped before importing the config of %s; run agnostic-ai use %s before syncing",
			strings.Join(pending, ", "), strings.Join(pending, " "))
	}
	return nil
}

// stillConfigured returns the failed tools a rollback did not take out
// of targets, which stay pending so sync keeps off their native files.
// With the config gone, none are.
func stillConfigured(failed []string) []string {
	if len(failed) == 0 {
		return nil
	}
	cfg, err := config.Load(".")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errs.CodeOf(err) == errs.CodeConfigMissing {
			return nil
		}
		return failed
	}
	return intersect(failed, cfg.Targets)
}

// setPendingImports records the tools whose import has not finished in
// the state file, which keeps every other field.
func setPendingImports(tools []string) error {
	// Writing over a ledger that does not parse would lose it, and the
	// marker this writes is what keeps a sync off unimported config.
	state, err := readStateFileStrict(".")
	if err != nil {
		return fmt.Errorf("%w; fix or delete it, then run agnostic-ai use again", err)
	}
	if slices.Equal(state.PendingImports, tools) {
		return nil
	}
	state.PendingImports = tools
	return replaceStateFile(".", state)
}

// replaceStateFile writes state as the state file under root whole.
func replaceStateFile(root string, state syncStateFile) error {
	p := stateFilePath(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("%s: %w", filepath.Dir(p), err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", p, err)
	}
	// A rename replaces the file whole, so a stop mid-write never leaves
	// a state that reads as nothing pending.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".sync-state-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", p, err)
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Sync(), tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", p, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("chmod %s: %w", p, err)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replace %s: %w", p, err)
	}
	return nil
}

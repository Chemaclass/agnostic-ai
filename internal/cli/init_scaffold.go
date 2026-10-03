package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// renderConfig builds agnostic-ai.yaml for the given targets list, in
// the order provided. A base other than the default writes `sources:`
// with paths nested under it; base="." writes paths at the project
// root. gitignoreEnabled adds `gitignore: { enabled: true }` so `sync`
// writes the managed block listing every adapter-emitted path. version
// pins the schema comment to that release.
func renderConfig(base string, targets []string, gitignoreEnabled bool, version string) string {
	var sb strings.Builder
	sb.WriteString("# yaml-language-server: $schema=" + schemaURL(version) + "\n")
	sb.WriteString("version: 1\n")
	if !isDefaultBase(base) {
		prefix := ""
		if base != "." {
			prefix = filepath.ToSlash(base) + "/"
		}
		sb.WriteString("\nsources:\n")
		for _, k := range scaffoldKinds {
			fmt.Fprintf(&sb, "  %s: %s%s\n", k, prefix, k)
		}
	}
	sb.WriteString("\ntargets:\n")
	for _, t := range targets {
		fmt.Fprintf(&sb, "  - %s\n", t)
	}
	sb.WriteString("\n" + onUnsupportedScaffold)
	if gitignoreEnabled {
		sb.WriteString("\ngitignore:\n  enabled: true\n")
	}
	return sb.String()
}

// schemaURL is the config schema of the release named by version, or
// the one on main for a build that is not a release.
func schemaURL(version string) string {
	ref := "main"
	if release, err := normalizeReleaseVersion(version); err == nil {
		ref = "v" + release
	}
	return "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/" + ref + "/docs/schemas/config.schema.json"
}

// onUnsupportedScaffold keeps warn, since most targets lack some kind
// and error would fail the first sync of `init --all --demo`, and names
// error so a new project learns the stricter setting.
const onUnsupportedScaffold = "# Set to error to fail sync when a target cannot represent a spec.\non-unsupported: warn\n"

// isDefaultBase reports whether base is the source directory config
// assumes when agnostic-ai.yaml has no `sources:`.
func isDefaultBase(base string) bool {
	return base == "" || filepath.ToSlash(filepath.Clean(base)) == config.SourceBaseDir
}

// scaffoldOptions groups the knobs scaffold uses to materialize a fresh
// project. Bundled in a struct so call sites (the init command and a
// dozen tests) self-document via named fields rather than a positional
// list of bools.
type scaffoldOptions struct {
	// Root is the project directory that receives agnostic-ai.yaml and
	// the .gitignore lines. Almost always ".".
	Root string
	// Base is the parent directory for the source-folder tree
	// (agents/, skills/, ...). Empty defaults to defaultBaseDir;
	// "." writes the folders at Root.
	Base string
	// Targets is written verbatim to the targets: block. Callers must
	// supply at least one entry.
	Targets []string
	// Preset, when set, seeds idiomatic specs for a stack ("go",
	// "ts-react", "python"). Composes with Demo.
	Preset string
	// Demo seeds example specs: a minimal one per source folder,
	// plus the memory-curator skill.
	Demo bool
	// DryRun prints the planned filesystem changes without touching
	// disk.
	DryRun bool
	// GitignoreEnabled persists gitignore.enabled: true into the
	// rendered config so subsequent `sync` runs maintain the managed
	// .gitignore block.
	GitignoreEnabled bool
	// Version is the running build's version, which pins the schema URL.
	Version string
}

// scaffoldKinds is the source-folder set a scaffold with a custom base
// declares and creates, in `sources:` order. The default base creates
// only the folders demo or preset specs seed; `new` and `import` create
// the rest on first use.
var scaffoldKinds = []string{"agents", "skills", "rules", "hooks", "mcps", "commands", "settings", "reviews", "environments", "ignore"}

// scaffold creates agnostic-ai.yaml at Root and the source-folder tree
// under Base. See scaffoldOptions for the per-field contract.
func scaffold(opts scaffoldOptions) error {
	if opts.Base == "" {
		opts.Base = defaultBaseDir
	}
	cfgPath := filepath.Join(opts.Root, config.ConfigFileName)
	if !opts.DryRun {
		if err := ensureNoExistingConfig(opts.Root, cfgPath); err != nil {
			return err
		}
	}
	if opts.DryRun {
		return scaffoldDryRun(opts, cfgPath)
	}
	return scaffoldWrite(opts, cfgPath)
}

// ensureNoExistingConfig refuses to overwrite an existing project. The
// legacy filename gets a tailored rename hint.
func ensureNoExistingConfig(root, cfgPath string) error {
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("%s already exists", config.ConfigFileName)
	}
	if _, err := os.Stat(filepath.Join(root, config.LegacyConfigFileName)); err == nil {
		return fmt.Errorf("%s already exists (legacy name; rename to %s)",
			config.LegacyConfigFileName, config.ConfigFileName)
	}
	return nil
}

// scaffoldDryRun prints the filesystem changes scaffold would make
// without touching disk.
func scaffoldDryRun(opts scaffoldOptions, cfgPath string) error {
	fmt.Printf("create: %s\n", cfgPath)
	for _, k := range declaredSourceDirs(opts.Base) {
		fmt.Printf("mkdir:  %s\n", filepath.Join(opts.Root, opts.Base, k))
	}
	baseDir := filepath.Join(opts.Root, opts.Base)
	if opts.Demo {
		if err := listDemoFiles(opts.Root, baseDir); err != nil {
			return err
		}
	}
	if opts.Preset != "" {
		if err := listPresetFiles(baseDir, opts.Preset); err != nil {
			return err
		}
	}
	return nil
}

// scaffoldWrite materializes the scaffold on disk and prints the
// post-scaffold guidance.
func scaffoldWrite(opts scaffoldOptions, cfgPath string) error {
	if err := writeScaffold(opts, cfgPath); err != nil {
		return err
	}
	if opts.Demo {
		summaryf("seeded example specs, one per source folder plus the memory-curator skill. delete or edit to taste.\n")
	}
	if opts.Preset != "" {
		summaryf("seeded preset %q. review and tune the rules to match your house style.\n", opts.Preset)
	}
	printNextSteps(opts.Root, opts.Base, opts.Targets, opts.Demo || opts.Preset != "")
	return nil
}

// declaredSourceDirs names the source folders the scaffold creates up
// front: the ones a custom base writes into `sources:`, since validate
// warns about a declared folder that is missing. The default base
// declares none.
func declaredSourceDirs(base string) []string {
	if isDefaultBase(base) {
		return nil
	}
	return scaffoldKinds
}

// scaffoldSilently writes the scaffold without the guidance, for the
// project copy `init --from --dry-run` imports into.
func scaffoldSilently(opts scaffoldOptions) error {
	if opts.Base == "" {
		opts.Base = defaultBaseDir
	}
	return writeScaffold(opts, filepath.Join(opts.Root, config.ConfigFileName))
}

// writeScaffold writes the config, the source folders, the managed
// .gitignore block, and any demo or preset specs.
func writeScaffold(opts scaffoldOptions, cfgPath string) error {
	baseDir := filepath.Join(opts.Root, opts.Base)
	for _, k := range declaredSourceDirs(opts.Base) {
		if err := os.MkdirAll(filepath.Join(baseDir, k), 0o755); err != nil {
			return err
		}
	}
	cfgBody := renderConfig(opts.Base, opts.Targets, opts.GitignoreEnabled, opts.Version)
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	// Seed the managed block with the fixed agnostic-ai ignores
	// (local-override config, sync state, packs dir). These must never be
	// committed regardless of `gitignore.enabled`, so init writes them
	// even when sync will not manage the generated-output lines. sync
	// later refreshes the same block with the emitted artifact paths.
	gicfg := &config.Config{}
	if err := updateGitignore(opts.Root, gicfg, buildManagedBlock(gicfg, nil, nil)); err != nil {
		return err
	}
	if opts.Demo {
		if err := writeDemoFiles(opts.Root, baseDir); err != nil {
			return err
		}
	}
	if opts.Preset != "" {
		if err := writePresetFiles(baseDir, opts.Preset); err != nil {
			return err
		}
	}
	return nil
}

// listDemoFiles prints the paths that writeDemoFiles would create.
func listDemoFiles(root, baseDir string) error {
	return fs.WalkDir(demoFS, "initdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		dst, _ := demoDestination(root, baseDir, path)
		fmt.Printf("create: %s\n", dst)
		return nil
	})
}

// demoDestination maps an embedded demo file to its path on disk and
// mode. Hook scripts land in .agnostic-ai/scripts/ whatever the base,
// since sync reads shared script bodies only from there, and stay
// executable because the demo hook runs its script directly.
func demoDestination(root, baseDir, embedded string) (string, fs.FileMode) {
	rel := strings.TrimPrefix(embedded, "initdata/")
	if script, ok := strings.CutPrefix(rel, "scripts/"); ok {
		return filepath.Join(root, agnosticScriptsDir, filepath.FromSlash(script)), 0o755
	}
	return filepath.Join(baseDir, filepath.FromSlash(rel)), 0o644
}

// listPresetFiles prints the paths that writePresetFiles would create.
func listPresetFiles(baseDir, preset string) error {
	return fs.WalkDir(presetFS, filepath.Join("initdata/presets", preset), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.Join("initdata/presets", preset), path)
		if err != nil {
			return err
		}
		fmt.Printf("create: %s\n", filepath.Join(baseDir, filepath.FromSlash(rel)))
		return nil
	})
}

// printNextSteps emits the post-scaffold guidance. seeded reports
// whether --demo or --preset wrote any source specs, in which case the
// suggested next step is `sync` (the source dirs are not empty).
// Otherwise the suggested next step is `import <target>`. Detected
// CLI configs under root are surfaced as concrete `import` follow-ups.
// A closing tip points at `completion` so first-time users discover
// shell tab-completion (never auto-installed; instructions only).
func printNextSteps(root, base string, targets []string, seeded bool) {
	summaryf("✓ initialized agnostic-ai project at %s\n", baseLabel(base))
	if len(targets) > 0 {
		summaryf("  enabled: %s\n", strings.Join(targets, ", "))
	}
	summaryf("\n")
	summaryf("next steps:\n")
	if seeded {
		summaryf("  agnostic-ai sync --plan       # preview what changes\n")
		summaryf("  agnostic-ai sync              # emit to your configured targets\n")
	} else {
		summaryf("  agnostic-ai new rule <name>   # write your first spec\n")
		summaryf("  agnostic-ai import <target>   # or mirror an existing CLI's config into specs\n")
		summaryf("  agnostic-ai sync              # emit to your configured targets\n")
	}
	if detected, _ := detectImportSources(root); len(detected) > 0 {
		summaryf("\n")
		summaryf("detected existing config:\n")
		for i, d := range detected {
			if i >= 3 {
				summaryf("  (and %d more)\n", len(detected)-3)
				break
			}
			summaryf("  agnostic-ai import %s\n", d)
		}
	}
	summaryf("\n")
	summaryf("tip: enable shell tab-completion with `agnostic-ai completion <shell>` (bash, zsh, fish, powershell).\n")
}

// baseLabel renders the user-facing label for the scaffold root. "."
// means the legacy root-level layout, so show the project root instead
// of a bare dot; any other base gets a trailing slash to read as a dir.
func baseLabel(base string) string {
	if base == "" || base == "." {
		return "./"
	}
	return filepath.ToSlash(base) + "/"
}

// writeDemoFiles mirrors every file under initdata/ into baseDir,
// preserving the kind subfolder, and hook scripts into root's
// .agnostic-ai/scripts/. Existing files are left untouched so a rerun
// against a partially populated tree never clobbers user content.
func writeDemoFiles(root, baseDir string) error {
	return fs.WalkDir(demoFS, "initdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		dst, mode := demoDestination(root, baseDir, path)
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		data, err := demoFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		return nil
	})
}

package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// renderConfig builds agnostic-ai.yaml with source paths nested under
// base and the given targets list. base="." writes paths at the
// project root. Targets are emitted in the order provided.
// gitignoreEnabled adds `gitignore: { enabled: true }` so `sync` writes
// the managed block listing every adapter-emitted path.
func renderConfig(base string, targets []string, gitignoreEnabled bool) string {
	prefix := ""
	if base != "" && base != "." {
		prefix = filepath.ToSlash(base) + "/"
	}
	var sb strings.Builder
	sb.WriteString("# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json\n")
	sb.WriteString("version: 1\n\n")
	sb.WriteString("sources:\n")
	fmt.Fprintf(&sb, "  agents: %sagents\n", prefix)
	fmt.Fprintf(&sb, "  skills: %sskills\n", prefix)
	fmt.Fprintf(&sb, "  rules: %srules\n", prefix)
	fmt.Fprintf(&sb, "  hooks: %shooks\n", prefix)
	fmt.Fprintf(&sb, "  mcps: %smcps\n", prefix)
	fmt.Fprintf(&sb, "  commands: %scommands\n", prefix)
	fmt.Fprintf(&sb, "  settings: %ssettings\n", prefix)
	fmt.Fprintf(&sb, "  reviews: %sreviews\n", prefix)
	fmt.Fprintf(&sb, "  environments: %senvironments\n", prefix)
	sb.WriteString("\ntargets:\n")
	for _, t := range targets {
		fmt.Fprintf(&sb, "  - %s\n", t)
	}
	sb.WriteString("\non-unsupported: warn\n")
	if gitignoreEnabled {
		sb.WriteString("\ngitignore:\n  enabled: true\n")
	}
	return sb.String()
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
	// Demo seeds one minimal example spec per source folder.
	Demo bool
	// DryRun prints the planned filesystem changes without touching
	// disk.
	DryRun bool
	// GitignoreEnabled persists gitignore.enabled: true into the
	// rendered config so subsequent `sync` runs maintain the managed
	// .gitignore block.
	GitignoreEnabled bool
}

// scaffoldKinds is the source-folder set every scaffold creates. Order
// does not matter on disk but is preserved for stable dry-run output.
var scaffoldKinds = []string{"agents", "skills", "rules", "hooks", "mcps", "commands", "settings", "reviews", "environments"}

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
	for _, k := range scaffoldKinds {
		fmt.Printf("mkdir:  %s\n", filepath.Join(opts.Root, opts.Base, k))
	}
	baseDir := filepath.Join(opts.Root, opts.Base)
	if opts.Demo {
		if err := listDemoFiles(baseDir); err != nil {
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
	baseDir := filepath.Join(opts.Root, opts.Base)
	for _, k := range scaffoldKinds {
		if err := os.MkdirAll(filepath.Join(baseDir, k), 0o755); err != nil {
			return err
		}
	}
	cfgBody := renderConfig(opts.Base, opts.Targets, opts.GitignoreEnabled)
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	// Seed the managed block with the fixed agnostic-ai ignores
	// (local-override config, sync state, packs dir). These must never be
	// committed regardless of `gitignore.enabled`, so init writes them
	// even when sync will not manage the generated-output lines. sync
	// later refreshes the same block with the emitted artifact paths.
	gicfg := &config.Config{}
	if err := updateGitignore(opts.Root, gicfg, buildManagedBlock(gicfg, nil)); err != nil {
		return err
	}
	if opts.Demo {
		if err := writeDemoFiles(baseDir); err != nil {
			return err
		}
	}
	if opts.Preset != "" {
		if err := writePresetFiles(baseDir, opts.Preset); err != nil {
			return err
		}
	}
	if opts.Demo {
		summaryf("seeded one example spec per source folder. delete or edit to taste.\n")
	}
	if opts.Preset != "" {
		summaryf("seeded preset %q. review and tune the rules to match your house style.\n", opts.Preset)
	}
	printNextSteps(opts.Root, opts.Base, opts.Targets, opts.Demo || opts.Preset != "")
	return nil
}

// listDemoFiles prints the paths that writeDemoFiles would create.
func listDemoFiles(baseDir string) error {
	return fs.WalkDir(demoFS, "initdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("initdata", path)
		if err != nil {
			return err
		}
		fmt.Printf("create: %s\n", filepath.Join(baseDir, filepath.FromSlash(rel)))
		return nil
	})
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
func printNextSteps(root, base string, targets []string, seeded bool) {
	summaryf("✓ initialized agnostic-ai project at %s\n", baseLabel(base))
	if len(targets) > 0 {
		summaryf("  enabled: %s\n", strings.Join(targets, ", "))
	}
	summaryf("\n")
	summaryf("next steps:\n")
	if seeded {
		summaryf("  agnostic-ai sync --check      # preview what will be written\n")
		summaryf("  agnostic-ai sync              # emit to your configured targets\n")
	} else {
		summaryf("  agnostic-ai import <target>   # mirror an existing CLI's config into specs\n")
		summaryf("  agnostic-ai sync              # emit to your configured targets\n")
	}
	detected := detectExistingTargets(root)
	if len(detected) == 0 {
		return
	}
	summaryf("\n")
	summaryf("detected existing config:\n")
	for i, d := range detected {
		if i >= 3 {
			summaryf("  (and %d more)\n", len(detected)-3)
			return
		}
		summaryf("  agnostic-ai import %s\n", d)
	}
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
// preserving the kind subfolder. Existing files are left untouched so a
// rerun against a partially populated tree never clobbers user content.
func writeDemoFiles(baseDir string) error {
	return fs.WalkDir(demoFS, "initdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("initdata", path)
		if err != nil {
			return err
		}
		dst := filepath.Join(baseDir, filepath.FromSlash(rel))
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
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		return nil
	})
}

package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// defaultBaseDir is the default parent directory for scaffolded source
// folders (agents, skills, rules, hooks, mcps).
const defaultBaseDir = ".agnostic-ai"

// demoFS holds one minimal sample spec per source kind. Used by
// `init --demo` to seed a fresh project so the user can run `sync`
// immediately and see what each adapter produces.
//
//go:embed initdata/agents/* initdata/skills/* initdata/rules/* initdata/hooks/* initdata/mcps/*
var demoFS embed.FS

// presetFS holds stack-flavored starter spec packs. `init --preset go`
// (or `--preset ts-react`, `--preset python`) seeds the project with the
// preset's specs, in addition to whatever `--demo` would write. Each
// preset lives at `initdata/presets/<name>/<kind>/...`.
//
//go:embed all:initdata/presets
var presetFS embed.FS

const presetRoot = "initdata/presets"

func newInitCmd() *cobra.Command {
	var demo, all bool
	var preset string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		Long: "Creates agnostic-ai.yaml plus source folders. " +
			"Default base dir is .agnostic-ai/. Pass a positional argument " +
			"to override (use \".\" for the legacy root-level layout). " +
			"When stdin is a terminal, init prompts for which targets to enable; " +
			"pipe a comma-separated list to skip the prompt, or pass --all / -a " +
			"to enable every supported target without prompting. " +
			"Pass --demo to seed each source folder with a minimal example spec. " +
			"Pass --preset <name> to seed idiomatic specs for a stack (go, ts-react, python).",
		Example: `  # Default: scaffold under .agnostic-ai/, prompt for targets when TTY
  agnostic-ai init

  # Skip the prompt, enable every supported target
  agnostic-ai init --all

  # Non-interactive: pipe the target list
  echo "claude,codex" | agnostic-ai init

  # Seed each source folder with one minimal example spec
  agnostic-ai init --demo

  # Seed idiomatic specs for a stack
  agnostic-ai init --preset go
  agnostic-ai init --preset ts-react

  # Legacy root-level layout (agents/, skills/, rules/, ... at project root)
  agnostic-ai init .

  # Custom base directory
  agnostic-ai init config/ai`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := defaultBaseDir
			if len(args) == 1 {
				base = args[0]
			}
			if preset != "" {
				if err := validatePresetName(preset); err != nil {
					return err
				}
			}
			targets := allTargetNames()
			if !all {
				picked, err := selectTargetsForSync(cmd.InOrStdin(), cmd.ErrOrStderr(), detectExistingTargets("."))
				if err != nil {
					return err
				}
				if len(picked) > 0 {
					targets = picked
				}
			}
			return scaffold(".", base, demo, preset, targets)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false,
		"Seed each source folder with a minimal example spec.")
	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Skip the target picker and enable every supported target.")
	cmd.Flags().StringVar(&preset, "preset", "",
		"Seed stack-flavored starter specs (go, ts-react, python). Composes with --demo and --all.")
	_ = cmd.RegisterFlagCompletionFunc("preset", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return availablePresets(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// availablePresets returns the sorted list of preset names embedded in
// the binary. Drives both tab completion and the unknown-preset error
// message.
func availablePresets() []string {
	entries, err := presetFS.ReadDir(presetRoot)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func validatePresetName(name string) error {
	for _, p := range availablePresets() {
		if p == name {
			return nil
		}
	}
	return fmt.Errorf("unknown preset %q. Available: %s", name, strings.Join(availablePresets(), ", "))
}

// renderConfig builds agnostic-ai.yaml with source paths nested under
// base and the given targets list. base="." writes paths at the
// project root. Targets are emitted in the order provided.
func renderConfig(base string, targets []string) string {
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
	sb.WriteString("\ntargets:\n")
	for _, t := range targets {
		fmt.Fprintf(&sb, "  - %s\n", t)
	}
	sb.WriteString("\non-unsupported: warn\n")
	return sb.String()
}

// scaffold creates agnostic-ai.yaml at root and the source-folder tree
// under base. targets is written verbatim to the targets: block;
// callers must supply at least one entry.
func scaffold(root, base string, demo bool, preset string, targets []string) error {
	cfgPath := filepath.Join(root, config.ConfigFileName)
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("%s already exists", config.ConfigFileName)
	}
	if _, err := os.Stat(filepath.Join(root, config.LegacyConfigFileName)); err == nil {
		return fmt.Errorf("%s already exists (legacy name; rename to %s)",
			config.LegacyConfigFileName, config.ConfigFileName)
	}
	if base == "" {
		base = defaultBaseDir
	}
	kinds := []string{"agents", "skills", "rules", "hooks", "mcps"}
	for _, k := range kinds {
		if err := os.MkdirAll(filepath.Join(root, base, k), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(cfgPath, []byte(renderConfig(base, targets)), 0o644); err != nil {
		return err
	}
	if err := ensureLineInGitignore(root, config.LocalOverrideFileName); err != nil {
		return err
	}
	if demo {
		if err := writeDemoFiles(filepath.Join(root, base)); err != nil {
			return err
		}
	}
	if preset != "" {
		if err := writePresetFiles(filepath.Join(root, base), preset); err != nil {
			return err
		}
	}
	summaryf("scaffold complete. edit %s then run `agnostic-ai sync`.\n", scaffoldHint(base, kinds))
	if demo {
		summaryf("seeded one example spec per source folder. delete or edit to taste.\n")
	}
	if preset != "" {
		summaryf("seeded preset %q. review and tune the rules to match your house style.\n", preset)
	}
	summaryf("import existing AI CLI config with `agnostic-ai import <source>`.\n")
	return nil
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

// writePresetFiles mirrors every file under initdata/presets/<name>
// into baseDir, preserving the kind subfolder. Existing files are left
// untouched, so layering --preset on top of --demo or an existing tree
// never clobbers user content.
func writePresetFiles(baseDir, name string) error {
	root := presetRoot + "/" + name
	return fs.WalkDir(presetFS, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(baseDir, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		data, err := presetFS.ReadFile(path)
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

func scaffoldHint(base string, kinds []string) string {
	list := strings.Join(kinds, ",")
	if base == "" || base == "." {
		return fmt.Sprintf("{%s}/", list)
	}
	return fmt.Sprintf("%s/{%s}/", filepath.ToSlash(base), list)
}

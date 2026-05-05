package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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

func newInitCmd() *cobra.Command {
	var demo, interactive bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold an agnostic-ai project in the current directory.",
		Long: "Creates agnostic.config.yaml plus source folders. " +
			"Default base dir is .agnostic-ai/. Pass a positional argument " +
			"to override (use \".\" for the legacy root-level layout). " +
			"Pass --demo to seed each source folder with a minimal example spec. " +
			"Pass -i / --interactive to pick which targets land in the config.",
		Example: `  # Default: scaffold under .agnostic-ai/
  agnostic-ai init

  # Pick which targets to enable
  agnostic-ai init -i

  # Seed each source folder with one minimal example spec
  agnostic-ai init --demo

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
			targets := allTargetNames()
			if interactive {
				picked, err := selectTargets(cmd.InOrStdin(), cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				targets = picked
			}
			return scaffold(".", base, demo, targets)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false,
		"Seed each source folder with a minimal example spec.")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false,
		"Prompt for which targets to enable instead of writing all.")
	return cmd
}

// renderConfig builds agnostic.config.yaml with source paths nested
// under base and the given targets list. base="." writes paths at the
// project root. Targets are emitted in the order provided.
func renderConfig(base string, targets []string) string {
	prefix := ""
	if base != "" && base != "." {
		prefix = filepath.ToSlash(base) + "/"
	}
	var sb strings.Builder
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

// scaffold creates agnostic.config.yaml at root and the source-folder
// tree under base. targets is written verbatim to the targets: block;
// callers must supply at least one entry.
func scaffold(root, base string, demo bool, targets []string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
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
	if demo {
		if err := writeDemoFiles(filepath.Join(root, base)); err != nil {
			return err
		}
	}
	fmt.Printf("scaffold complete. edit %s then run `agnostic-ai sync`.\n", scaffoldHint(base, kinds))
	if demo {
		fmt.Println("seeded one example spec per source folder. delete or edit to taste.")
	}
	fmt.Println("import existing AI CLI config with `agnostic-ai import <source>`.")
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

func scaffoldHint(base string, kinds []string) string {
	list := strings.Join(kinds, ",")
	if base == "" || base == "." {
		return fmt.Sprintf("{%s}/", list)
	}
	return fmt.Sprintf("%s/{%s}/", filepath.ToSlash(base), list)
}

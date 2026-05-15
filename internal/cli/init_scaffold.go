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
	fmt.Fprintf(&sb, "  commands: %scommands\n", prefix)
	sb.WriteString("\ntargets:\n")
	for _, t := range targets {
		fmt.Fprintf(&sb, "  - %s\n", t)
	}
	sb.WriteString("\non-unsupported: warn\n")
	return sb.String()
}

// scaffold creates agnostic-ai.yaml at root and the source-folder tree
// under base. targets is written verbatim to the targets: block;
// callers must supply at least one entry. When dryRun is true, no files
// are written and the planned paths are printed instead.
func scaffold(root, base string, demo bool, preset string, targets []string, dryRun bool) error {
	cfgPath := filepath.Join(root, config.ConfigFileName)
	if !dryRun {
		if _, err := os.Stat(cfgPath); err == nil {
			return fmt.Errorf("%s already exists", config.ConfigFileName)
		}
		if _, err := os.Stat(filepath.Join(root, config.LegacyConfigFileName)); err == nil {
			return fmt.Errorf("%s already exists (legacy name; rename to %s)",
				config.LegacyConfigFileName, config.ConfigFileName)
		}
	}
	if base == "" {
		base = defaultBaseDir
	}
	kinds := []string{"agents", "skills", "rules", "hooks", "mcps", "commands"}
	if dryRun {
		fmt.Printf("create: %s\n", cfgPath)
		for _, k := range kinds {
			fmt.Printf("mkdir:  %s\n", filepath.Join(root, base, k))
		}
		if demo {
			if err := listDemoFiles(filepath.Join(root, base)); err != nil {
				return err
			}
		}
		if preset != "" {
			if err := listPresetFiles(filepath.Join(root, base), preset); err != nil {
				return err
			}
		}
		return nil
	}
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
	if err := ensureLineInGitignore(root, ".agnostic-ai/.sync-state"); err != nil {
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
	if demo {
		summaryf("seeded one example spec per source folder. delete or edit to taste.\n")
	}
	if preset != "" {
		summaryf("seeded preset %q. review and tune the rules to match your house style.\n", preset)
	}
	printNextSteps(root, base, targets, demo || preset != "")
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

package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const cloudProjectReadiness = "https://agnostic-ai.org/docs/cli-reference/project/#project-readiness"

type cloudEnvironmentGuide struct {
	Name            string   `json:"name"`
	Settings        []string `json:"settings"`
	InstallLocation string   `json:"install_location,omitempty"`
	Notes           []string `json:"notes"`
}

var cloudEnvironmentGuides = map[string]cloudEnvironmentGuide{
	"claude": {
		Name:            "Claude Code",
		Settings:        []string{"Environment variables", "Network secrets", "Allowed domains"},
		InstallLocation: "Setup script",
		Notes: []string{
			"Anthropic-hosted sessions load the repository hook for single-repo sessions outside a project's cloud threads.",
			"Anyone who can use the environment can read its plain variables. A network secret is attached by the proxy to matching requests.",
		},
	},
	"cursor": {
		Name:     "Cursor",
		Settings: []string{"Saved environment", "Environment variables and secrets"},
		Notes: []string{
			"Point the saved environment at environment.json to use the file. Multi-repo runs and Self-Hosted Machines skip it.",
			"Cursor runs install when it creates a Build and reuses the prepared disk.",
		},
	},
	"copilot": {
		Name:     "GitHub Copilot",
		Settings: []string{"Agents secrets", "Agents variables", "Firewall"},
		Notes: []string{
			"The setup steps workflow must be on the default branch and applies to sessions on Actions.",
			"Cloud sandboxes receive Agents secrets and skip the workflow and Agents variables.",
		},
	},
	"codex": {
		Name:            "Codex Cloud",
		Settings:        []string{"Environment variables", "HTTP secrets", "Additional allowed domains"},
		InstallLocation: "Install script",
		Notes:           []string{"The Codex app's environment.toml configures local worktrees."},
	},
	"openhands": {
		Name:     "OpenHands",
		Settings: []string{"Secrets"},
		Notes:    []string{"OpenHands runs setup.sh when it starts working on the repository."},
	},
	"amp": {
		Name:     "Amp",
		Settings: []string{"Secrets and Environment Variables", "Pre-setup Script"},
		Notes:    []string{"Amp runs the primary repository's executable .agents/setup when it prepares an orb."},
	},
	"ellipsis": {
		Name:     "Ellipsis",
		Settings: []string{"Secrets"},
		Notes:    []string{"Commit the selected environment on the default branch."},
	},
}

type cloudEnvironmentFile struct {
	Path           string `json:"path"`
	Status         string `json:"status"`
	InspectionOnly bool   `json:"inspection_only,omitempty"`
}

type cloudEnvironmentTarget struct {
	Target    string                 `json:"target"`
	Files     []cloudEnvironmentFile `json:"repo_files"`
	Commit    []string               `json:"gitignore_commit,omitempty"`
	Install   string                 `json:"install,omitempty"`
	Variables []cloudVariableSetting `json:"variables,omitempty"`
	Domains   []cloudDomainSetting   `json:"allowed_domains,omitempty"`
	cloudEnvironmentGuide
}

type cloudEnvironmentReport struct {
	Version   string                   `json:"version"`
	Command   string                   `json:"command"`
	Targets   []cloudEnvironmentTarget `json:"targets"`
	Readiness string                   `json:"project_readiness"`
	// ManagedIgnores is gitignore.enabled. Without it, GitignoreEdits
	// lists the lines that make the ignored setup files trackable.
	ManagedIgnores bool            `json:"managed_ignores"`
	GitignoreEdits []gitignoreEdit `json:"gitignore_edits,omitempty"`
}

func newDoctorEnvironmentCmd() *cobra.Command {
	var targetFilter []string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Print the repository files and UI settings for cloud agents.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			targets, err := filterTargets(cfg.Targets, targetFilter, nil)
			if err != nil {
				return err
			}
			report, err := describeCloudEnvironments(cfg, bundle, targets)
			if err != nil {
				return err
			}
			if jsonOut {
				return writeIndentedJSON(cmd, report)
			}
			printCloudEnvironments(cmd, report)
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&targetFilter, "target", "t", nil, "Cloud targets to report (default: all enabled cloud targets)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for machine consumption")
	registerTargetCompletion(cmd)
	return cmd
}

func describeCloudEnvironments(cfg *config.Config, bundle spec.Bundle, targets []string) (cloudEnvironmentReport, error) {
	defer adapters.SetAsideNotes()()
	report := cloudEnvironmentReport{
		Version: "1", Command: "doctor environment", Readiness: cloudProjectReadiness,
		Targets: []cloudEnvironmentTarget{}, ManagedIgnores: cfg.Gitignore.Enabled,
	}
	var ignored []string
	sess := adapters.NewSession()
	for _, target := range targets {
		guide, ok := cloudEnvironmentGuides[target]
		if !ok {
			continue
		}
		entry := cloudEnvironmentTarget{Target: target, cloudEnvironmentGuide: guide}
		if guide.InstallLocation != "" {
			entry.Install = cloudEnvironmentInstall(bundle.Environments, target)
		}
		var invalid bool
		entry.Variables, entry.Domains, invalid = cloudEnvironmentRequirements(bundle.Environments, target)
		if invalid {
			entry.Notes = append(slices.Clone(entry.Notes), "Some environment requirements are invalid. Run agnostic-ai lint for the fields to fix.")
		}
		files, err := cloudEnvironmentFiles(sess, cfg, bundle, target)
		if err != nil {
			if !invalid {
				return report, err
			}
			files = []cloudEnvironmentFile{}
			entry.Notes = append(entry.Notes, "Fix the environment requirements to preview the repository setup files.")
		}
		entry.Files = files
		for _, file := range files {
			if file.Status != "ignored" || file.InspectionOnly {
				continue
			}
			if !slices.Contains(ignored, file.Path) {
				ignored = append(ignored, file.Path)
			}
			if len(entry.Commit) == 0 {
				entry.Commit = cloudSetupCommitKinds(target)
			}
		}
		report.Targets = append(report.Targets, entry)
	}
	if !report.ManagedIgnores && len(ignored) != 0 {
		edits, err := planGitignoreEdits(".", ignored)
		if err != nil {
			return report, err
		}
		report.GitignoreEdits = edits
	}
	return report, nil
}

// cloudSetupCommitKinds returns the gitignore.commit entries for the
// setup files target's cloud agent reads: the `cloud` preset's kinds
// for target other than instructions, each scoped to target.
func cloudSetupCommitKinds(target string) []string {
	var out []string
	for _, kind := range config.CloudCommitKinds(target) {
		if kind != config.GitignoreInstructions {
			out = append(out, target+":"+kind)
		}
	}
	return out
}

func cloudEnvironmentInstall(envs []spec.Entry, target string) string {
	var install string
	for _, env := range envs {
		if !env.EmitsTo(target) {
			continue
		}
		if value, ok := adapters.ResolveMeta(env.Meta, target)["install"]; ok {
			if target == "claude" && value == false {
				value = env.Meta["install"]
			}
			install, _ = value.(string)
		}
	}
	return strings.TrimRight(install, "\r\n")
}

func cloudEnvironmentFiles(sess *adapters.Session, cfg *config.Config, bundle spec.Bundle, target string) ([]cloudEnvironmentFile, error) {
	if target == "codex" {
		return []cloudEnvironmentFile{}, nil
	}
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return nil, err
	}
	files, err := captureAdapterFiles(sess, adapter, bundleOfKind(bundle, "environments", target), cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", target, err)
	}
	output := cfg.Outputs[target]
	inspectionOnly := map[string]bool{}
	add := func(path, native string) {
		inspectionOnly[path] = path != native
		inspectionOnly[native] = false
	}
	for _, file := range files {
		path := filepath.ToSlash(filepath.Clean(file.Path))
		switch target {
		case "claude":
			if filepath.Base(path) == claudehooks.CloudInstallScript {
				add(path, ".claude/hooks/"+claudehooks.CloudInstallScript)
				add(filepath.ToSlash(filepath.Join(filepath.Dir(filepath.Dir(path)), "settings.json")), ".claude/settings.json")
			}
		case "cursor":
			const native = ".cursor/environment.json"
			if path == filepath.ToSlash(filepath.Clean(cmp.Or(output.EnvironmentFile, native))) {
				add(path, native)
			}
		case "copilot":
			if path == filepath.ToSlash(filepath.Clean(copilot.SetupStepsPath(cfg))) {
				add(path, copilot.NativeSetupStepsPath)
			}
		case "openhands":
			const setup = ".openhands/setup.sh"
			if path == filepath.ToSlash(filepath.Clean(cmp.Or(output.SetupFile, setup))) {
				add(path, setup)
			}
		case "amp":
			const setup, services = ".agents/setup", ".amp/services.yaml"
			switch path {
			case filepath.ToSlash(filepath.Clean(cmp.Or(output.SetupFile, setup))):
				add(path, setup)
			case filepath.ToSlash(filepath.Clean(cmp.Or(output.EnvironmentFile, services))):
				add(path, services)
			}
		case "ellipsis":
			if strings.HasPrefix(path, ".ellipsis/environments/") || output.EnvironmentFile != "" && path == filepath.ToSlash(filepath.Clean(output.EnvironmentFile)) {
				add(path, path)
			}
		}
	}
	for _, path := range nativeCloudEnvironmentPaths(target) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			path = filepath.ToSlash(path)
			add(path, path)
		}
	}
	report := make([]cloudEnvironmentFile, 0, len(inspectionOnly))
	for _, path := range slices.Sorted(maps.Keys(inspectionOnly)) {
		report = append(report, cloudEnvironmentFile{Path: path, Status: cloudEnvironmentFileStatus(path), InspectionOnly: inspectionOnly[path]})
	}
	return report, nil
}

func nativeCloudEnvironmentPaths(target string) []string {
	switch target {
	case "claude":
		const path = ".claude/settings.json"
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var settings claudehooks.Settings
		if json.Unmarshal(data, &settings) == nil && len(settings.Hooks["SessionStart"]) != 0 {
			return []string{path}
		}
	case "cursor":
		return []string{".cursor/environment.json"}
	case "copilot":
		return []string{copilot.NativeSetupStepsPath}
	case "openhands":
		return []string{".openhands/setup.sh"}
	case "amp":
		return []string{".agents/setup", ".amp/services.yaml"}
	case "ellipsis":
		paths, _ := filepath.Glob(".ellipsis/environments/*.yaml")
		return paths
	}
	return nil
}

func cloudEnvironmentFileStatus(path string) string {
	tracked, gitErr := gitquery.Tracked(".", path)
	if tracked {
		return "tracked"
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "missing"
	}
	if gitquery.Ignored(".", path) {
		return "ignored"
	}
	if gitErr != nil {
		return "Git status unavailable"
	}
	return "untracked"
}

func printCloudEnvironments(cmd *cobra.Command, report cloudEnvironmentReport) {
	if len(report.Targets) == 0 {
		cmd.Println("No enabled cloud targets.")
	}
	var kinds []string
	for _, target := range report.Targets {
		for _, kind := range target.Commit {
			if !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		}
	}
	for _, target := range report.Targets {
		cmd.Println(target.Name)
		if len(target.Files) == 0 {
			cmd.Println("  No repository setup files.")
		}
		for _, file := range target.Files {
			status := file.Status
			if file.InspectionOnly {
				status += ", inspection only"
			}
			cmd.Printf("  %s (%s)\n", file.Path, status)
		}
		switch {
		case len(target.Commit) == 0:
		case !report.ManagedIgnores:
			if len(report.GitignoreEdits) != 0 {
				cmd.Println("  To commit the ignored files, see Ignore file changes below.")
			}
		case len(kinds) > 1:
			cmd.Printf("  Set gitignore.commit: [%s], which includes %s, and commit the files.\n", config.GitignoreCommitCloud, strings.Join(target.Commit, ", "))
		default:
			cmd.Printf("  Set gitignore.commit: [%s] and commit the files.\n", strings.Join(target.Commit, ", "))
		}
		cmd.Printf("  Configure %s in the UI.\n", strings.Join(target.Settings, ", "))
		if target.Install != "" {
			cmd.Printf("  Paste into %s\n", target.InstallLocation)
			for _, line := range strings.Split(target.Install, "\n") {
				cmd.Printf("    %s\n", line)
			}
		}
		if len(target.Variables) != 0 {
			cmd.Println("  Variables and secrets")
			for _, variable := range target.Variables {
				cmd.Printf("    %s (%s)\n", variable.Name, variable.Location)
			}
		}
		if len(target.Domains) != 0 {
			cmd.Println("  Allowed domains")
			for _, domain := range target.Domains {
				cmd.Printf("    %s (%s)\n", domain.Domain, domain.Location)
			}
		}
		for _, note := range target.Notes {
			cmd.Printf("  %s\n", note)
		}
		cmd.Println()
	}
	printGitignoreEdits(cmd, report.GitignoreEdits)
	cmd.Printf("Project readiness checks\n  %s\n", report.Readiness)
}

func printGitignoreEdits(cmd *cobra.Command, edits []gitignoreEdit) {
	if len(edits) == 0 {
		return
	}
	cmd.Println("Ignore file changes")
	cmd.Println("  Sync does not manage your ignore files. Make these edits, then commit the files above. Line numbers refer to the file as it is now.")
	for _, edit := range edits {
		switch {
		case edit.Replace && len(edit.Add) == 0:
			cmd.Printf("  %s line %d: delete\n    %s\n", edit.Source, edit.Line, edit.Pattern)
			continue
		case edit.Replace:
			cmd.Printf("  %s line %d: replace\n    %s\n  with\n", edit.Source, edit.Line, edit.Pattern)
		case edit.Line != 0:
			cmd.Printf("  %s after line %d (%s): add\n", edit.Source, edit.Line, edit.Pattern)
		default:
			cmd.Printf("  %s at the end: add\n", edit.Source)
		}
		for _, line := range edit.Add {
			cmd.Printf("    %s\n", line)
		}
	}
	cmd.Println()
}

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// approvalRoutes names how each tool carries out a loosening mode. A
// tool or mode missing here has no safe route and falls back to ask.
var approvalRoutes = map[string]map[string]string{
	"claude": {config.ApprovalsReview: "Claude Code auto mode", config.ApprovalsYolo: "PermissionRequest hook"},
	"codex":  {config.ApprovalsReview: "Codex Auto-review", config.ApprovalsYolo: "PermissionRequest hook"},
}

type approvalsStatusOutput struct {
	Dir   string                `json:"dir"`
	Tools []approvalsToolStatus `json:"tools"`
}

type approvalsToolStatus struct {
	Tool string `json:"tool"`
	// Mode is what the settings ask for; Effective is what the tool does.
	Mode      string `json:"mode"`
	Effective string `json:"effective"`
	From      string `json:"from"`
	Route     string `json:"route,omitempty"`
}

func newApprovalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approvals",
		Short: "Show or stop automatic answers to permission prompts",
		Long: "Reads the approvals setting: ask (default) shows each tool's prompt, review lets the tool's own reviewer " +
			"decide, and yolo approves one call at a time unless an always-ask rule matches. review and yolo are read " +
			"only from the home config ($AGNOSTIC_AI_HOME/agnostic-ai.yaml and its local/ file).",
	}
	cmd.AddCommand(newApprovalsStatusCmd(), newApprovalsAskCmd())
	return cmd
}

func newApprovalsStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the approvals mode and route per tool for the working directory",
		Long: "Prints, for each tool sync --global writes, the approvals mode for the working directory, the setting " +
			"that chose it, and how the tool carries it out. A tool with no route for the mode falls back to ask.",
		Example: `  agnostic-ai approvals status
  agnostic-ai approvals status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := readApprovalsStatus(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if asJSON {
				return writeIndentedJSON(cmd, status)
			}
			printApprovalsStatus(cmd, status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the status as JSON")
	return cmd
}

func newApprovalsAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask",
		Short: "Show every permission prompt again, from the next request on",
		Long: "Writes approvals.mode: ask to $AGNOSTIC_AI_HOME/local/agnostic-ai.yaml. That block replaces the shared " +
			"one, so no tool or project keeps review or yolo; its always-ask patterns stay. It needs no sync.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			before, err := readApprovalsStatus(io.Discard)
			if err != nil {
				return err
			}
			source, err := globalSourceRoot()
			if err != nil {
				return err
			}
			path := globalConfigPaths(source)[1]
			if err := writeApprovalsAsk(path); err != nil {
				return err
			}
			cmd.Printf("approvals: ask for every tool and project. Wrote %s.\n", path)
			if slices.ContainsFunc(before.Tools, func(s approvalsToolStatus) bool { return s.Effective == config.ApprovalsReview }) {
				cmd.Println("A running session in a native reviewer mode keeps it until you switch modes in the tool (Shift+Tab in Claude Code).")
			}
			return nil
		},
	}
}

func readApprovalsStatus(warn io.Writer) (approvalsStatusOutput, error) {
	source, err := globalSourceRoot()
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	targets, err := loadGlobalTargets(source, io.Discard)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	if targets == nil {
		targets = globalTargetNames()
	}
	dir, err := os.Getwd()
	if err != nil {
		return approvalsStatusOutput{}, fmt.Errorf("resolve working directory: %w", err)
	}
	projectAsk, err := config.ProjectApprovalsAsk(dir)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	userHome, err := globalUserHome()
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	for tool := range home.Tools {
		if !slices.Contains(adapters.Names(), tool) {
			if _, err := fmt.Fprintf(warn, "warning: approvals.tools.%s: no such tool; it is ignored\n", tool); err != nil {
				return approvalsStatusOutput{}, fmt.Errorf("write approvals warning: %w", err)
			}
		}
	}
	policy := config.ApprovalsPolicy{Home: home, UserHome: userHome, Env: os.Getenv(config.EnvApprovals), ProjectAsk: projectAsk}
	out := approvalsStatusOutput{Dir: dir, Tools: make([]approvalsToolStatus, 0, len(targets))}
	for _, tool := range targets {
		decision := policy.Resolve(tool, dir)
		status := approvalsToolStatus{Tool: tool, Mode: decision.Mode, Effective: config.ApprovalsAsk, From: decision.From}
		if route := approvalRoutes[tool][decision.Mode]; route != "" {
			status.Effective, status.Route = decision.Mode, route
		}
		out.Tools = append(out.Tools, status)
	}
	return out, nil
}

func printApprovalsStatus(cmd *cobra.Command, status approvalsStatusOutput) {
	cmd.Printf("Approvals in %s:\n", status.Dir)
	width := 0
	for _, s := range status.Tools {
		width = max(width, len(s.Tool))
	}
	for _, s := range status.Tools {
		line := fmt.Sprintf("  %-*s  %s, from %s", width, s.Tool, s.Mode, s.From)
		switch {
		case s.Route != "":
			line += "; through the " + s.Route
		case s.Mode != config.ApprovalsAsk:
			line += "; no route for " + s.Mode + ", falls back to ask"
		}
		cmd.Println(line)
	}
}

// writeApprovalsAsk sets approvals.mode: ask in the home config at path,
// keeping its other keys, comments, and always-ask patterns.
func writeApprovalsAsk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the file is not a mapping of keys", path)
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "mode"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: config.ApprovalsAsk},
	}}
	found := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "approvals" {
			continue
		}
		if old := root.Content[i+1]; old.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(old.Content); j += 2 {
				if old.Content[j].Value == "always-ask" {
					block.Content = append(block.Content, old.Content[j], old.Content[j+1])
				}
			}
		}
		root.Content[i+1], found = block, true
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "approvals"}, block)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// projectApprovalsIssues reports approvals keys in the project files at
// root that would loosen approvals. The home folder is skipped: there the
// same file is the home config, which may loosen.
func projectApprovalsIssues(root string) ([]config.ApprovalsIssue, error) {
	if source, err := globalSourceRoot(); err == nil && sameRealDir(root, source) {
		return nil, nil
	}
	return config.ProjectApprovalsIssues(root)
}

func sameRealDir(a, b string) bool {
	ra, errA := filepath.Abs(a)
	rb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	ra, errA = filepath.EvalSymlinks(ra)
	rb, errB = filepath.EvalSymlinks(rb)
	return errA == nil && errB == nil && ra == rb
}

func approvalsValidationIssues(root string) []validationIssue {
	issues, err := projectApprovalsIssues(root)
	if err != nil {
		return []validationIssue{{Path: config.ConfigFileName, Field: "approvals", Message: err.Error()}}
	}
	out := make([]validationIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, validationIssue{Path: filepath.Base(issue.Path), Field: "approvals", Message: issue.Message})
	}
	return out
}

// lintProjectApprovals reports the same keys as LINT064.
func lintProjectApprovals(root string) []lintFinding {
	var out []lintFinding
	for _, issue := range approvalsValidationIssues(root) {
		out = append(out, lintFinding{Code: "LINT064", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
)

func newDoctorRTKCmd() *cobra.Command {
	var command string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "rtk",
		Short: "Preview RTK rewrites and declared Claude approval rules",
		Long: "Calls the installed RTK version and rewrite processor, but does not execute the supplied command. " +
			"Compares simple declared Bash rules in planned project, local, and user Claude settings. " +
			"Managed settings, plugins, session flags, and live approval outcomes remain unknown. No permissions are changed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(command) == "" || len(command) > 8192 {
				return errors.New("--command must contain between 1 and 8192 bytes")
			}
			report, err := collectRTKReport(command)
			if err != nil {
				return err
			}
			return printRTKReport(cmd.OutOrStdout(), report, asJSON)
		},
	}
	cmd.Flags().StringVar(&command, "command", "", "Raw command to preview, never executed")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the same diagnostic as JSON")
	return cmd
}

type rtkReport struct {
	Command       string      `json:"command"`
	Replacement   string      `json:"replacement,omitempty"`
	Executable    string      `json:"executable,omitempty"`
	Version       string      `json:"version,omitempty"`
	Status        string      `json:"status"`
	ProcessorExit *int        `json:"processor_exit"`
	Runtime       string      `json:"runtime"`
	Original      rtkDecision `json:"original"`
	Rewritten     rtkDecision `json:"rewritten"`
	Changed       bool        `json:"changed"`
	Hooks         []string    `json:"hooks"`
	Warnings      []string    `json:"warnings"`
}

type rtkDecision struct {
	Decision string   `json:"decision"`
	Rules    []string `json:"rules"`
}

type rtkSettings struct {
	Path        string             `json:"-"`
	Permissions rtkPermissionRules `json:"permissions"`
	Hooks       map[string][]struct {
		Matcher  string `json:"matcher"`
		Handlers []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

type rtkPermissionRules map[string][]string

func (rules *rtkPermissionRules) UnmarshalJSON(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	*rules = rtkPermissionRules{}
	for _, name := range []string{"deny", "ask", "allow"} {
		if field, ok := fields[name]; ok {
			var entries []string
			if err := json.Unmarshal(field, &entries); err != nil {
				return fmt.Errorf("permissions.%s: %w", name, err)
			}
			(*rules)[name] = entries
		}
	}
	return nil
}

func collectRTKReport(command string) (rtkReport, error) {
	report := rtkReport{Command: command, Status: "missing", Runtime: "unknown", Hooks: []string{}, Warnings: []string{}, Original: rtkDecision{Decision: "unknown", Rules: []string{}}, Rewritten: rtkDecision{Decision: "unknown", Rules: []string{}}}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		return report, err
	}
	adapter, err := adapters.Resolve("claude")
	if err != nil {
		return report, err
	}
	files, err := captureEmit(adapter, bundle.For("claude"), cfg)
	if err != nil {
		return report, err
	}
	settingsPath := claude.SettingsFilePath(cfg)
	var sources []rtkSettings
	for _, file := range files {
		if file.Path == settingsPath {
			source, err := decodeRTKSettings(settingsPath, []byte(file.Content))
			if err != nil {
				return report, err
			}
			sources = append(sources, source)
		}
	}
	userDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if userDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return report, fmt.Errorf("locate Claude user settings: %w", err)
		}
		userDir = filepath.Join(home, ".claude")
	}
	for _, path := range []string{filepath.Join(filepath.Dir(settingsPath), "settings.local.json"), filepath.Join(userDir, "settings.json")} {
		body, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return report, fmt.Errorf("%s: %w", path, err)
		}
		source, err := decodeRTKSettings(path, body)
		if err != nil {
			return report, err
		}
		sources = append(sources, source)
	}
	report.Original = rtkDeclaredDecision(command, sources)
	for _, source := range sources {
		for _, group := range source.Hooks["PreToolUse"] {
			if group.Matcher != "Bash" && group.Matcher != "" && group.Matcher != "*" {
				continue
			}
			for _, handler := range group.Handlers {
				if (handler.Type == "command" || handler.Type == "") && rtkDirectHandler(handler.Command) {
					report.Hooks = append(report.Hooks, source.Path)
				}
			}
		}
	}
	if len(report.Hooks) > 1 {
		report.Warnings = append(report.Warnings, "multiple known RTK handlers are configured; check project/global ownership")
	}
	path, err := exec.LookPath("rtk")
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return report, fmt.Errorf("locate RTK: %w", err)
		}
		report.Warnings = append(report.Warnings, "RTK is not installed; no rewrite is available")
		return report, nil
	}
	report.Executable = path
	version, err := runRTKDiagnostic(path, "--version")
	if err != nil {
		return report, err
	}
	report.Version = strings.TrimSpace(string(version))
	exitCode := 0
	report.ProcessorExit = &exitCode
	output, err := runRTKDiagnostic(path, "rewrite", command)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return report, err
		}
		exitCode = exit.ExitCode()
		switch {
		case exit.ExitCode() == 1 && len(output) == 0:
			report.Status = "unchanged"
			report.Rewritten = report.Original
			return report, nil
		case exit.ExitCode() == 2 && len(output) == 0:
			report.Status = "processor-denied"
			report.Warnings = append(report.Warnings, "RTK's own rule inspection denied the command; no replacement was returned")
			return report, nil
		case exit.ExitCode() == 3 && len(output) > 0:
			report.Warnings = append(report.Warnings, "RTK's own rule inspection requests approval; this is not a live host decision")
		default:
			return report, err
		}
	}
	report.Replacement = strings.TrimSpace(string(output))
	if report.Replacement == "" {
		return report, errors.New("RTK rewrite succeeded without a replacement")
	}
	if report.Replacement == command {
		report.Status = "unchanged"
		report.Rewritten = report.Original
		return report, nil
	}
	report.Status = "rewritten"
	report.Rewritten = rtkDeclaredDecision(report.Replacement, sources)
	report.Changed = report.Original.Decision != "unknown" && report.Rewritten.Decision != "unknown" && report.Original.Decision != report.Rewritten.Decision
	if report.Changed {
		report.Warnings = append(report.Warnings, "the rewrite changes the matching declared approval decision")
	}
	if report.Original.Decision == "unknown" || report.Rewritten.Decision == "unknown" {
		report.Warnings = append(report.Warnings, "declared approval comparison is unknown; inspect effective host rules")
	}
	return report, nil
}

func rtkDirectHandler(command string) bool {
	command = strings.TrimSpace(command)
	if command == "command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude" || command == "rtk hook claude" {
		return true
	}
	return strings.HasPrefix(command, "rtk rewrite ")
}

func decodeRTKSettings(path string, body []byte) (rtkSettings, error) {
	source := rtkSettings{Path: path}
	clean, _ := adapters.StripJSONC(body)
	if err := json.Unmarshal(clean, &source); err != nil {
		return source, fmt.Errorf("parse %s: %w", path, err)
	}
	return source, nil
}

func runRTKDiagnostic(path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var output rtkDiagnosticOutput
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err != nil {
		return output.Bytes(), fmt.Errorf("RTK diagnostic %s: %w", args[0], err)
	}
	return output.Bytes(), nil
}

type rtkDiagnosticOutput struct{ bytes.Buffer }

func (out *rtkDiagnosticOutput) Write(data []byte) (int, error) {
	if len(data) > 65536-out.Len() {
		return 0, errors.New("RTK diagnostic output exceeds 64 KiB")
	}
	return out.Buffer.Write(data)
}

var rtkPlainCommand = regexp.MustCompile(`^[a-zA-Z0-9_./:-]+( [a-zA-Z0-9_./:-]+)*$`)

func rtkDeclaredDecision(command string, sources []rtkSettings) rtkDecision {
	result := rtkDecision{Decision: "unknown", Rules: []string{}}
	if !rtkPlainCommand.MatchString(command) {
		return result
	}
	executable, _, _ := strings.Cut(command, " ")
	switch path.Base(executable) {
	case "timeout", "time", "nice", "nohup", "stdbuf", "command", "builtin", "noglob", "xargs":
		return result
	}
	uncertain := false
	for _, decision := range []string{"deny", "ask", "allow"} {
		var matched []string
		for _, source := range sources {
			for _, rule := range source.Permissions[decision] {
				match, known := rtkRuleMatches(rule, command)
				if !known {
					uncertain = true
				}
				if match {
					matched = append(matched, source.Path+": "+rule)
				}
			}
		}
		if len(matched) > 0 {
			result.Rules = matched
			if !uncertain {
				result.Decision = decision
			}
			return result
		}
	}
	return result
}

func rtkRuleMatches(rule, command string) (bool, bool) {
	if rule == "Bash" || rule == "Bash(*)" {
		return true, true
	}
	if !strings.HasPrefix(rule, "Bash(") {
		tool, _, _ := strings.Cut(rule, "(")
		if strings.Contains(tool, "*") {
			matches, err := path.Match(tool, "Bash")
			if matches || err != nil {
				return false, false
			}
		}
		return false, true
	}
	if !strings.HasSuffix(rule, ")") {
		return false, false
	}
	pattern := strings.TrimSuffix(strings.TrimPrefix(rule, "Bash("), ")")
	if strings.HasPrefix(pattern, "run_in_background:") {
		return false, false
	}
	for _, suffix := range []string{":*", " *"} {
		if strings.HasSuffix(pattern, suffix) {
			prefix := strings.TrimSuffix(pattern, suffix)
			if !rtkPlainCommand.MatchString(prefix) {
				return false, false
			}
			return command == prefix || strings.HasPrefix(command, prefix+" "), true
		}
	}
	if !rtkPlainCommand.MatchString(pattern) {
		return false, false
	}
	return command == pattern, true
}

func printRTKReport(out io.Writer, report rtkReport, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	_, err := fmt.Fprintf(out, "RTK: %s (%s)\nOriginal: %s\nReplacement: %s\nDeclared approval: %s -> %s\nRuntime approval: %s (managed settings, plugins, session flags, and activation are not established)\n", report.Status, report.Version, report.Command, report.Replacement, report.Original.Decision, report.Rewritten.Decision, report.Runtime)
	if err != nil {
		return err
	}
	for _, decision := range []rtkDecision{report.Original, report.Rewritten} {
		for _, rule := range decision.Rules {
			if _, err := fmt.Fprintln(out, "  "+rule); err != nil {
				return err
			}
		}
	}
	for _, hook := range report.Hooks {
		if _, err := fmt.Fprintln(out, "RTK handler: "+hook); err != nil {
			return err
		}
	}
	for _, warning := range report.Warnings {
		if _, err := fmt.Fprintln(out, "Warning: "+warning); err != nil {
			return err
		}
	}
	return nil
}

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// approvalRouteFor and approvalsGOOS pick each tool's route; tests
// replace them.
var (
	approvalRouteFor = approvals.RouteFor
	approvalsGOOS    = runtime.GOOS
)

type approvalsStatusOutput struct {
	Dir   string                `json:"dir"`
	Tools []approvalsToolStatus `json:"tools"`
	// Companion is approvals.companion: the companion shows Approve and
	// Deny for a request left to the person.
	Companion bool `json:"companion"`
	// reviewerWritten is set when a native reviewer key sync --scope user
	// wrote is still on.
	reviewerWritten bool
}

type approvalsToolStatus struct {
	Tool string `json:"tool"`
	// Mode is what the settings ask for; Effective is what the tool does.
	Mode      string `json:"mode"`
	Effective string `json:"effective"`
	From      string `json:"from"`
	// Route is what carries out Effective, when it loosens.
	Route string `json:"route,omitempty"`
	// Yolo and Review name the tool's route for each mode, or ask.
	Yolo   string `json:"yolo"`
	Review string `json:"review"`
	// Reason says why Mode falls back to ask.
	Reason     string `json:"reason,omitempty"`
	MinVersion string `json:"min_version,omitempty"`
	// Notes name what keeps the tool's native reviewer from matching Mode.
	Notes []string `json:"notes,omitempty"`
}

func newApprovalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approvals",
		Short: "Show or stop automatic answers to permission prompts",
		Long: "Reads the approvals setting: ask (default) shows each tool's prompt, review lets the tool's own reviewer " +
			"decide, and yolo approves one call at a time unless an always-ask rule matches. review and yolo are read " +
			"only from the home config ($AGNOSTIC_AI_HOME/agnostic-ai.yaml and its local/ file).",
	}
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	cmd.AddCommand(newApprovalsStatusCmd(), newApprovalsAskCmd(), newApprovalsLogCmd())
	return cmd
}

func newApprovalsStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the approvals mode and route per tool for the working directory",
		Long: "Prints, for each tool sync --scope user writes, the approvals mode for the working directory, the setting " +
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
			// A broken config must not block the switch off, so a failed
			// read only drops the reviewer note.
			before, _ := readApprovalsStatus(io.Discard)
			source, err := userSourceRoot()
			if err != nil {
				return err
			}
			path := userConfigPaths(source)[1]
			if err := writeApprovalsAsk(path); err != nil {
				return err
			}
			cmd.Printf("approvals: ask for every tool and project. Wrote %s.\n", path)
			if before.reviewerWritten {
				cmd.Println("Run sync --scope user to remove the native reviewer keys it wrote to the tools' user settings.")
			}
			if before.reviewerWritten || slices.ContainsFunc(before.Tools, func(s approvalsToolStatus) bool { return s.Effective == config.ApprovalsReview }) {
				cmd.Println("A running session in a native reviewer mode keeps it until you switch modes in the tool (Shift+Tab in Claude Code).")
			}
			return nil
		},
	}
}

func readApprovalsStatus(warn io.Writer) (approvalsStatusOutput, error) {
	source, err := userSourceRoot()
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	targets, err := loadUserTargets(source, io.Discard)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	if targets == nil {
		targets = userTargetNames()
	}
	dir, err := os.Getwd()
	if err != nil {
		return approvalsStatusOutput{}, fmt.Errorf("resolve working directory: %w", err)
	}
	projectAsk, err := config.ProjectApprovalsAsk(dir)
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	userHome, err := userHomeDir()
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
	state, err := loadUserState(filepath.Join(source, "state", "global.json"))
	if err != nil {
		return approvalsStatusOutput{}, err
	}
	policy := config.ApprovalsPolicy{Home: home, UserHome: userHome, Env: os.Getenv(config.EnvApprovals), ProjectAsk: projectAsk}
	reviewerWritten := false
	out := approvalsStatusOutput{Dir: dir, Tools: make([]approvalsToolStatus, 0, len(targets)), Companion: home.Companion}
	for _, tool := range targets {
		decision := policy.Resolve(tool, dir)
		status := approvalsToolStatusFor(tool, decision)
		rs, native, err := readReviewerState(tool, userHome, state)
		if err != nil {
			status.Notes = append(status.Notes, err.Error())
		} else if native {
			homeMode, _ := homeToolMode(home, tool)
			status.Notes = append(status.Notes, rs.notes(tool, decision.Mode, homeMode)...)
			if status.Effective == config.ApprovalsReview && !rs.on {
				status.Effective, status.Route = config.ApprovalsAsk, ""
				status.Reason = "the reviewer is not on in the tool's user settings"
			}
			reviewerWritten = reviewerWritten || rs.written
		}
		if tool == "kiro" {
			written := kiroReviewStatus(&status, userHome, decision.Mode, state)
			reviewerWritten = reviewerWritten || written
		}
		out.Tools = append(out.Tools, status)
	}
	out.reviewerWritten = reviewerWritten
	return out, nil
}

func approvalsToolStatusFor(tool string, decision config.ApprovalDecision) approvalsToolStatus {
	route := approvalRouteFor(tool)
	status := approvalsToolStatus{
		Tool:       tool,
		Mode:       decision.Mode,
		Effective:  config.ApprovalsAsk,
		From:       decision.From,
		Yolo:       approvalsMechanismName(route.Mechanism(config.ApprovalsYolo, approvalsGOOS)),
		Review:     approvalsMechanismName(route.Mechanism(config.ApprovalsReview, approvalsGOOS)),
		Reason:     route.FallbackReason(decision.Mode, approvalsGOOS),
		MinVersion: route.MinVersion,
	}
	mechanism := route.Mechanism(decision.Mode, approvalsGOOS)
	if mechanism == nil {
		return status
	}
	if mechanism.Name == approvals.MechanismPermissionHook {
		if reason := approvalHookProblem(tool); reason != "" {
			status.Reason = reason
			return status
		}
	}
	status.Effective, status.Route = decision.Mode, mechanism.Name
	if tool == "claude" && decision.Mode == config.ApprovalsYolo {
		status.Notes = append(status.Notes, claudeUnreadRulesNote)
	}
	return status
}

// claudeUnreadRulesNote names the Claude Code ask rules the hook cannot
// see, whose prompt a hook allow would answer.
const claudeUnreadRulesNote = "ask rules passed with --settings or --allowedTools are not read; put them in approvals.always-ask too"

func approvalsMechanismName(m *approvals.Mechanism) string {
	if m == nil {
		return config.ApprovalsAsk
	}
	return m.Name
}

// approvalHookCommand is the text every approvals hook command holds.
const approvalHookCommand = "agnostic-ai hook approve"

// approvalHookProblem says why tool's approvals hook cannot answer yet:
// sync --scope user has not written it, or Codex has not trusted it.
func approvalHookProblem(tool string) string {
	const notSynced = "the hook is not installed; add approvals to builtins in the home config and run sync --scope user"
	if tool == "kiro" {
		return kiroHookProblem(notSynced)
	}
	var path string
	switch tool {
	case "claude":
		path, _ = claudeUserSettingsPath()
	case "codex":
		path, _ = codex.UserHooksPath()
	case "qoder":
		if home, err := userHomeDir(); err == nil {
			path = filepath.Join(home, ".qoder", "settings.json")
		}
	case "qwen":
		if home, err := userHomeDir(); err == nil {
			if dir, ok := qwenUserDir(home); ok {
				path = filepath.Join(dir, "settings.json")
			} else {
				return "QWEN_HOME is not an absolute path, so the hook cannot find the Qwen Code settings"
			}
		}
	}
	if path == "" {
		return notSynced
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return notSynced
	}
	var file struct {
		DisableAllHooks bool `json:"disableAllHooks"`
		Hooks           struct {
			PermissionRequest []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PermissionRequest"`
		} `json:"hooks"`
	}
	stripped, _ := preview.StripJSONC(body)
	if err := json.Unmarshal(stripped, &file); err != nil {
		return fmt.Sprintf("%s does not parse: %v", path, err)
	}
	installed := false
	for _, group := range file.Hooks.PermissionRequest {
		for _, handler := range group.Hooks {
			installed = installed || strings.Contains(handler.Command, approvalHookCommand)
		}
	}
	if !installed {
		return notSynced
	}
	if file.DisableAllHooks {
		return path + " sets disableAllHooks"
	}
	if tool == "qwen" {
		return qwenAskProblem()
	}
	if tool == "qoder" {
		return qoderAskProblem()
	}
	if tool != "codex" {
		return ""
	}
	findings, err := codex.HookTrustFindings(path, body)
	if err != nil {
		return "cannot check that Codex trusts the hook: " + err.Error()
	}
	for _, f := range findings {
		if f.Event == "PermissionRequest" && strings.Contains(f.Hook, approvalHookCommand) {
			return "Codex has not trusted the hook; open /hooks in Codex to trust it"
		}
	}
	return ""
}

// kiroHookProblem says why the Kiro approvals hook cannot answer: no
// enabled PermissionRequest hook in ~/.kiro/hooks runs it, or a Kiro
// rule asks, which turns the hook off.
func kiroHookProblem(notSynced string) string {
	home, err := userHomeDir()
	if err != nil {
		return notSynced
	}
	root, err := kiroApprovalRoot(home)
	if err != nil {
		return err.Error()
	}
	dir := filepath.Join(root, "hooks")
	files, err := os.ReadDir(dir)
	if err != nil {
		return notSynced
	}
	installed := false
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			continue
		}
		var file struct {
			Hooks []struct {
				Trigger string `json:"trigger"`
				Enabled *bool  `json:"enabled"`
				Action  struct {
					Command string `json:"command"`
				} `json:"action"`
			} `json:"hooks"`
		}
		if json.Unmarshal(body, &file) != nil {
			continue
		}
		for _, h := range file.Hooks {
			if strings.EqualFold(h.Trigger, "PermissionRequest") && (h.Enabled == nil || *h.Enabled) && strings.Contains(h.Action.Command, approvalHookCommand) {
				installed = true
			}
		}
	}
	if !installed {
		return notSynced
	}
	cwd, _ := os.Getwd()
	ask, err := kiroRulesAsk(root, cwd)
	switch {
	case err != nil:
		return "cannot read the Kiro rules: " + err.Error()
	case ask:
		return "a Kiro permissions file or agent profile has an ask or evaluate rule, or approve-for-me, whose prompt a hook allow could answer; the hook asks every time"
	}
	return ""
}

func kiroApprovalRoot(home string) (string, error) {
	if os.Getenv("KIRO_HOME") == "" {
		return filepath.Join(home, ".kiro"), nil
	}
	root := userTargets["kiro"].overrideRoot(home)
	if root == "" {
		return "", errors.New("cannot locate the Kiro rules under KIRO_HOME")
	}
	return root, nil
}

func kiroRulesAsk(root, cwd string) (bool, error) {
	files, err := approvals.KiroAskRuleFiles(root, cwd)
	if err != nil {
		return false, err
	}
	return approvals.KiroHasAskRule(files)
}

func readQwenApprovalSettings(dir, cwd string) (approvals.JSONSettings, error) {
	files, err := approvals.QwenSettingsFiles(dir, cwd, runtime.GOOS, os.Getenv)
	if err != nil {
		return approvals.JSONSettings{}, err
	}
	return approvals.ReadJSONSettings(files)
}

// qwenAskProblem says why the Qwen Code hook cannot approve: hooks are
// off, a permissions.ask rule turns it off, or Qwen Code runs in a mode
// in which the hook stays silent.
func qwenAskProblem() string {
	for _, env := range []string{"QWEN_CODE_SAFE_MODE", "QWEN_CODE_SIMPLE"} {
		if os.Getenv(env) != "" {
			return env + " is set, which turns off every Qwen Code hook"
		}
	}
	home, err := userHomeDir()
	if err != nil {
		return "cannot find the home folder: " + err.Error()
	}
	dir, _ := qwenUserDir(home)
	cwd, _ := os.Getwd()
	settings, err := readQwenApprovalSettings(dir, cwd)
	switch {
	case err != nil:
		return "cannot read the Qwen Code settings: " + err.Error()
	case settings.HooksOff:
		return "a Qwen Code settings file sets disableAllHooks"
	case settings.Ask:
		return "a Qwen Code settings file lists a permissions.ask rule, whose prompt a hook allow could answer; the hook asks every time"
	case settings.MovedEnv:
		return "a Qwen Code settings file's env block sets HOME, AGNOSTIC_AI_HOME, CLAUDE_CONFIG_DIR or USERPROFILE, which would move the policy the hook reads; the hook asks every time"
	case qwenDotEnvMoves(cwd):
		return "a .env or .qwen/.env file here or above sets HOME, AGNOSTIC_AI_HOME, CLAUDE_CONFIG_DIR or USERPROFILE, which Qwen Code passes to its hooks; the hook asks every time"
	case !qwenHookMode(settings.ApprovalMode):
		mode := settings.ApprovalMode
		if mode == "" {
			mode = "auto in a trusted folder"
		}
		return "Qwen Code runs in " + mode + " mode, where the hook stays silent; set tools.approvalMode to default or auto-edit"
	}
	return ""
}

// qwenHookMode reports whether a tools.approvalMode value, spelled any
// way Qwen Code accepts it, is a mode in which the hook answers prompts.
func qwenHookMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "default", "auto-edit", "auto_edit", "autoedit":
		return true
	}
	return false
}

// qoderAskProblem says when a Qoder CLI permissions.ask rule, whose
// prompt a hook allow could answer, turns the hook off.
func qoderAskProblem() string {
	home, err := userHomeDir()
	if err != nil {
		return "cannot find the home folder: " + err.Error()
	}
	cwd, _ := os.Getwd()
	files, ok := qoderSettingsFiles(home, cwd)
	if !ok {
		return "cannot read the Qoder settings from a relative QODER_PROJECT_DIR"
	}
	settings, err := approvals.ReadJSONSettings(files)
	mode := strings.ToLower(settings.DefaultPermissionMode)
	switch {
	case err != nil:
		return "cannot read the Qoder settings: " + err.Error()
	case settings.Ask:
		return "a Qoder settings file lists a permissions.ask rule, whose prompt a hook allow could answer; the hook asks every time"
	case mode != "" && mode != "default" && mode != "accept_edits" && mode != "acceptedits":
		return "Qoder runs in " + settings.DefaultPermissionMode + " mode, where the hook stays silent; set general.defaultPermissionMode to default or accept_edits"
	}
	return ""
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
		case s.Reason != "":
			line += "; falls back to ask: " + s.Reason
		}
		cmd.Println(line)
		for _, note := range s.Notes {
			cmd.Println("    note: " + note)
		}
	}
	if status.Companion {
		cmd.Println("Companion buttons: on, for Claude Code and Codex requests left to you")
	} else {
		cmd.Println("Companion buttons: off (approvals.companion)")
	}
}

// writeApprovalsAsk sets approvals.mode: ask in the home config at path,
// keeping its other keys, comments, and always-ask patterns.
func writeApprovalsAsk(path string) error {
	return editApprovalsFile(path, approvalsAskYAML)
}

// editApprovalsFile replaces the home config at path with edit's result.
func editApprovalsFile(path string, edit func([]byte) ([]byte, error)) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	out, err := edit(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := atomicfile.Write(path, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// approvalsAskYAML is data with approvals.mode: ask. The block keeps
// only its always-ask patterns, so no tool or project keeps review or
// yolo.
func approvalsAskYAML(data []byte) ([]byte, error) {
	return editApprovalsYAML(data, "mode", config.ApprovalsAsk, func(old *yaml.Node) *yaml.Node {
		block := approvalsMapping("mode", config.ApprovalsAsk)
		if old != nil && old.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(old.Content); j += 2 {
				if old.Content[j].Value == "always-ask" {
					block.Content = append(block.Content, old.Content[j], old.Content[j+1])
				}
			}
		}
		return block
	})
}

// approvalsSetYAML is data with one scalar key of the approvals block
// set to value, a string or a bool. The other keys stay.
func approvalsSetYAML(data []byte, key string, value any) ([]byte, error) {
	return editApprovalsYAML(data, key, value, func(old *yaml.Node) *yaml.Node {
		if old == nil || old.Kind != yaml.MappingNode {
			return approvalsMapping(key, value)
		}
		for j := 0; j+1 < len(old.Content); j += 2 {
			if old.Content[j].Value == key {
				node := approvalsScalar(value)
				node.LineComment, node.HeadComment, node.FootComment = old.Content[j+1].LineComment, old.Content[j+1].HeadComment, old.Content[j+1].FootComment
				old.Content[j+1] = node
				return old
			}
		}
		set := approvalsMapping(key, value)
		old.Content = append(old.Content, set.Content...)
		return old
	})
}

func approvalsScalar(value any) *yaml.Node {
	if on, ok := value.(bool); ok {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(on)}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fmt.Sprint(value)}
}

func approvalsMapping(key string, value any) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		approvalsScalar(value),
	}}
}

// editApprovalsYAML is data with its approvals block replaced by what
// block makes of the old one, nil when there is none. A file without an
// approvals block gets one appended as text, so its layout stays. key
// and value name the change in a refusal.
func editApprovalsYAML(data []byte, key string, value any, block func(old *yaml.Node) *yaml.Node) ([]byte, error) {
	byHand := fmt.Sprintf("set approvals.%s: %v by hand", key, value)
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("holds more than one YAML document; " + byHand)
	}
	var root *yaml.Node
	if len(doc.Content) > 0 && doc.Content[0].Tag != "!!null" {
		root = doc.Content[0]
		if root.Kind != yaml.MappingNode {
			return nil, errors.New("is not a mapping of keys")
		}
	}
	index := -1
	if root != nil {
		merged := false
		for i := 0; i+1 < len(root.Content); i += 2 {
			key := root.Content[i]
			if key.Value == "approvals" {
				index = i + 1
			}
			merged = merged || key.Tag == "!!merge"
		}
		if index < 0 && merged {
			return nil, errors.New("merges keys with a YAML merge key; " + byHand)
		}
	}
	if index < 0 && (root == nil || root.Style&yaml.FlowStyle == 0) {
		if endsDocument(data) {
			return nil, errors.New("ends its YAML document with ...; " + byHand)
		}
		text := string(data)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return []byte(text + fmt.Sprintf("approvals:\n  %s: %v\n", key, value)), nil
	}
	if index < 0 {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "approvals"}, block(nil))
	} else {
		old := root.Content[index]
		if old.Anchor != "" {
			return nil, fmt.Errorf("approvals carries the YAML anchor &%s; %s", old.Anchor, byHand)
		}
		if old.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("approvals is the YAML alias *%s; %s", old.Value, byHand)
		}
		if old.Kind != yaml.MappingNode {
			// A broken block must not block the change: it is replaced.
			old = nil
		} else {
			for j := 0; j+1 < len(old.Content); j += 2 {
				if old.Content[j].Tag == "!!merge" {
					return nil, errors.New("approvals merges keys with a YAML merge key; " + byHand)
				}
			}
		}
		root.Content[index] = block(old)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), nil
}

// endsDocument reports whether data holds a YAML document end marker:
// text appended after it would start a document nobody reads.
func endsDocument(data []byte) bool {
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimRight(line, " \t\r") == "..." {
			return true
		}
	}
	return false
}

// projectApprovalsIssues reports approvals keys in the project files at
// root that would loosen approvals. The home folder is skipped: there the
// same file is the home config, which may loosen.
func projectApprovalsIssues(root string) ([]config.ApprovalsIssue, error) {
	if source, err := userSourceRoot(); err == nil && sameRealDir(root, source) {
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

// qwenDotEnvMoves reports a .env file that would move the policy, or
// one that does not read.
func qwenDotEnvMoves(cwd string) bool {
	moved, err := approvals.QwenDotEnvMovesPolicy(cwd)
	return err != nil || moved
}

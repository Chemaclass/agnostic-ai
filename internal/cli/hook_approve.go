package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
)

// claudeManagedDir is where Claude Code's managed settings live here.
var claudeManagedDir = approvals.ClaudeManagedDir(runtime.GOOS)

// maxApprovePayload bounds the hook input; a larger one asks.
const maxApprovePayload = 8 << 20

func newHookApproveCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Answer a permission prompt from inside a PermissionRequest hook",
		Long: "Reads a PermissionRequest hook payload on stdin. When the approvals mode for the tool and the session's " +
			"folder is yolo and no always-ask rule matches, it prints the tool's reply that allows this one call and logs " +
			"the decision. In every other case it prints nothing, so the tool shows its own prompt. It always exits 0, " +
			"never denies, and writes nothing to stderr. For claude it also asks on a match with an ask rule in the " +
			"Claude Code user, project, local or managed settings files. For kiro it asks whenever a Kiro permissions " +
			"file or agent profile holds an ask or evaluate rule or the approve-for-me preset.",
		Example: `  # Hook command sync --global writes for Claude Code
  command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook approve --target claude`,
		Args:               cobra.ArbitraryArgs,
		FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
		SilenceErrors:      true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxApprovePayload+1))
			if err != nil || len(raw) > maxApprovePayload {
				return nil
			}
			if target == "" {
				target = os.Getenv(adapters.HookTargetEnv)
			}
			if reply := hookApproveReply(target, raw, time.Now()); reply != nil {
				_, _ = cmd.OutOrStdout().Write(reply)
			}
			return nil
		},
	}
	// A flag that does not parse must not reach stderr: leave the prompt.
	cmd.SetFlagErrorFunc(func(*cobra.Command, error) error { return nil })
	cmd.Flags().StringVarP(&target, "target", "t", "", "Tool that runs the hook: claude, codex or kiro (default $"+adapters.HookTargetEnv+")")
	return cmd
}

// approvePayload is the part of a PermissionRequest payload the hook reads.
type approvePayload struct {
	Event          string         `json:"hook_event_name"`
	Session        string         `json:"session_id"`
	Cwd            string         `json:"cwd"`
	PermissionMode string         `json:"permission_mode"`
	Call           string         `json:"tool_name"`
	Input          map[string]any `json:"tool_input"`
}

// hookApproveReply returns the allow reply for one call, or nil to leave
// the tool's prompt. Every failure, a panic included, returns nil.
func hookApproveReply(target string, raw []byte, now time.Time) (reply []byte) {
	defer func() {
		if recover() != nil {
			reply = nil
		}
	}()
	allow, ok := hookrun.PermissionAllowReply(target)
	if !ok || approvals.RouteFor(target).Mechanism(config.ApprovalsYolo, runtime.GOOS) == nil {
		return nil
	}
	var p approvePayload
	if err := json.Unmarshal(raw, &p); err != nil || p.Input == nil || p.Call == "" {
		return nil
	}
	if p.Event != "" && p.Event != "PermissionRequest" {
		return nil
	}
	if !filepath.IsAbs(p.Cwd) {
		return nil
	}
	source, err := globalSourceRoot()
	if err != nil || !filepath.IsAbs(source) {
		return nil
	}
	home, err := config.LoadHomeApprovals(source)
	if err != nil {
		return nil
	}
	projectAsk, err := config.ProjectApprovalsAsk(p.Cwd)
	if err != nil {
		return nil
	}
	userHome, err := globalUserHome()
	if err != nil || !filepath.IsAbs(userHome) {
		return nil
	}
	policy := config.ApprovalsPolicy{Home: home, UserHome: userHome, Env: os.Getenv(config.EnvApprovals), ProjectAsk: projectAsk}
	mode := policy.Resolve(target, p.Cwd).Mode
	if mode != config.ApprovalsYolo {
		return nil
	}
	request := approvals.Request{Call: p.Call, Input: p.Input, PermissionMode: p.PermissionMode, Cwd: p.Cwd}
	native := approvals.NativeRequest(target, request)
	decision := approvals.Decision{Outcome: approvals.Asked, Rule: approvals.RuleToolSettings}
	if settings, ok := approveToolSettings(target, p.Cwd, userHome); ok {
		projectDir := os.Getenv("CLAUDE_PROJECT_DIR")
		decision = approvals.Decide(native, approvals.Policy{
			Mode:         mode,
			ProjectRoot:  approveProjectRoot(p.Cwd),
			Home:         userHome,
			AgnosticHome: source,
			AlwaysAsk:    append(append([]string(nil), home.AlwaysAsk...), settings.Ask...),
			RuleRoots:    append([]string{p.Cwd, projectDir}, settings.RuleRoots...),
			Resolve:      approvals.ResolveLinks,
		})
	}
	log := approvals.Log{Path: filepath.Join(source, "local", "approvals", "decisions.jsonl")}
	logged := native
	if logged.Call == "" {
		logged = request
	}
	entry := approvals.NewEntry(target, p.Session, mode, logged, decision, now)
	entry.Call = request.Call
	if err := log.Append(entry); err != nil {
		// An approval must leave a line in the log; without one, ask.
		return nil
	}
	if decision.Outcome != approvals.Approved {
		return nil
	}
	return allow
}

// approveToolSettings reads the tool's own ask rules, which a hook allow
// would answer. For Claude Code it returns the rules of its settings
// files (decision 022); ok is false when a settings file does not read,
// or a project file's env block moves the folders the hook reads its
// policy from. For Kiro, whose rules the hook cannot match, ok is false
// when any Kiro rule asks or a rules file does not read.
func approveToolSettings(target, cwd, userHome string) (approvals.ClaudeSettings, bool) {
	switch target {
	case "kiro":
		ask, err := approvals.KiroHasAskRule(approvals.KiroAskRuleFiles(userHome, cwd))
		return approvals.ClaudeSettings{}, err == nil && !ask
	case "claude":
	default:
		return approvals.ClaudeSettings{}, true
	}
	user, _ := claudeUserSettingsPath()
	files := approvals.ClaudeSettingsFiles(cwd, os.Getenv("CLAUDE_PROJECT_DIR"), user, claudeManagedDir)
	settings, err := approvals.ReadClaudeSettings(files)
	if err != nil || len(settings.MovedEnv) > 0 {
		return approvals.ClaudeSettings{}, false
	}
	return settings, true
}

// approveProjectRoot is the root of the Git checkout that holds cwd, or
// cwd outside one.
func approveProjectRoot(cwd string) string {
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return filepath.Clean(cwd)
		}
	}
}

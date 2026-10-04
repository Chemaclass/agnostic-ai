package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookpaths"
)

// guardReport is a hook finding for the agent: main prints it to stderr
// and exits 2, which every portable-event target reads as feedback.
type guardReport struct{ text string }

func (r *guardReport) Error() string { return r.text }
func (r *guardReport) ExitCode() int { return 2 }

func newHookGuardCmd() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "guard <after-edit|stop>",
		Short: "Check specs from inside an after-edit or stop hook",
		Long: "after-edit reads the hook payload on stdin and lints the specs the edit touched. " +
			"stop checks whether generated files match the specs. Each exits 2 with a short report " +
			"for the agent on a problem, and 0 with no output otherwise, including when it cannot " +
			"tell, so a broken setup never blocks the agent. It never runs sync.",
		Example: `  # Hook commands that fail open when the binary is missing
  command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard after-edit
  command -v agnostic-ai >/dev/null 2>&1 || exit 0; agnostic-ai hook guard stop`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"after-edit", "stop"},
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _ := io.ReadAll(cmd.InOrStdin())
			adapters.SetWarner(io.Discard)
			defer adapters.SetWarner(os.Stderr)
			switch args[0] {
			case "after-edit":
				return guardAfterEdit(raw, target)
			case "stop":
				return guardStop(raw)
			}
			return fmt.Errorf("unknown guard %q: want after-edit or stop", args[0])
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target whose payload stdin holds, for after-edit (default $"+adapters.HookTargetEnv+")")
	return cmd
}

// guardAfterEdit reports the error findings lint raises on the specs an
// edit touched.
func guardAfterEdit(raw []byte, target string) error {
	if target == "" {
		target = os.Getenv(adapters.HookTargetEnv)
	}
	if target == "" {
		target = hookpaths.GuessTarget(raw)
	}
	payload, err := hookpaths.Read(target, raw)
	if err != nil {
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return nil
	}
	var edited []string
	for _, c := range payload.Relative(root) {
		edited = append(edited, c.Path)
	}
	cfg, err := config.Load(".")
	if err != nil || len(specPaths(cfg, edited)) == 0 {
		return nil
	}
	findings, err := lintFindingsForFiles(edited)
	if err != nil {
		return &guardReport{"agnostic-ai could not load the specs after this edit: " + err.Error()}
	}
	var lines []string
	for _, f := range findings {
		if f.Severity == lintError {
			lines = append(lines, f.String())
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return &guardReport{"agnostic-ai lint found errors in the specs you edited:\n" + strings.Join(lines, "\n")}
}

// guardStop reports drift between the specs and the files sync writes.
// A stop the agent is already continuing from a stop hook passes, so the
// notice cannot loop.
func guardStop(raw []byte) error {
	var payload struct {
		StopHookActive bool `json:"stop_hook_active"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.StopHookActive {
		return nil
	}
	cfg, err := config.Load(".")
	if err != nil {
		return nil
	}
	reports, err := collectDrift(cfg.Targets)
	if err != nil || !slices.ContainsFunc(reports, driftReport.hasDrift) {
		return nil
	}
	return &guardReport{"Specs changed since the last sync: run `agnostic-ai sync`."}
}

// lintFindingsForFiles runs every project lint check and keeps the
// findings on files. Cross-spec checks still see the whole spec set.
func lintFindingsForFiles(files []string) ([]lintFinding, error) {
	scope, err := loadCheckScope(false)
	if err != nil {
		return nil, err
	}
	findings, err := lintScopeFindings(scope)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, f := range files {
		keep[cleanRelPath(f)] = true
	}
	var out []lintFinding
	for _, f := range findings {
		if keep[cleanRelPath(f.Path)] {
			out = append(out, f)
		}
	}
	return out, nil
}

// specPaths returns the paths among files that sit in a spec source.
func specPaths(cfg *config.Config, files []string) []string {
	roots := []string{".agnostic-ai"}
	for _, dir := range sourceDirsByKind(cfg.Sources) {
		if dir != "" {
			roots = append(roots, cleanRelPath(config.ResolveSourcePath(".", dir)))
		}
	}
	var out []string
	for _, f := range files {
		p := cleanRelPath(f)
		for _, r := range roots {
			if p == r || strings.HasPrefix(p, r+"/") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// cleanRelPath is path relative to the working directory, slash-separated.
func cleanRelPath(path string) string {
	if filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, path); err == nil {
				path = rel
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}

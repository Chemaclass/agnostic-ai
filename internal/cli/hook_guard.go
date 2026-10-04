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
			// A hook can start in a directory below the project root.
			root := guardProjectRoot()
			if root == "" {
				return nil
			}
			wd, err := os.Getwd()
			if err != nil || os.Chdir(root) != nil {
				return nil
			}
			defer func() { _ = os.Chdir(wd) }()
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
		if c.Action != hookpaths.ActionDelete {
			edited = append(edited, c.Path)
		}
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
	if err != nil || !slices.ContainsFunc(reports, driftReport.specsChanged) {
		return nil
	}
	return &guardReport{"Specs changed since the last sync: run `agnostic-ai sync`."}
}

// specsChanged reports whether sync would write a file it has not yet,
// or rewrite one nobody edited by hand. Other drift, such as a hand edit
// to a generated file, may predate the session and is not the agent's to
// settle with a sync.
func (r driftReport) specsChanged() bool {
	return len(r.Missing) > 0 || len(r.Stale) > 0
}

// guardProjectRoot returns the nearest directory at or above the working
// directory that holds a project config, or "".
func guardProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, _, err := config.ResolveConfigPath(dir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
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
	var out []lintFinding
	for _, f := range findings {
		p := cleanRelPath(f.Path)
		for _, file := range files {
			// A directory names every spec below it, such as a skill folder.
			if want := cleanRelPath(file); p == want || strings.HasPrefix(p, want+"/") {
				out = append(out, f)
				break
			}
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

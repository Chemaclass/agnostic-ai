package cli

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// gitignoreCommitDocs explains gitignore.commit, which keeps the files
// cloud agents read in Git.
const gitignoreCommitDocs = "https://agnostic-ai.org/docs/sync-settings/#gitignore"

// cloudInstructionAgents names, for each target with a hosted agent, the
// agent that starts from a fresh clone and reads only committed files.
var cloudInstructionAgents = map[string]string{
	"claude":    "Claude Code on the web",
	"codex":     "Codex Cloud",
	"cursor":    "Cursor cloud agents",
	"copilot":   "Copilot cloud agent",
	"jules":     "Jules",
	"amp":       "Amp",
	"openhands": "OpenHands Cloud",
}

// hasCloudInstructionTarget reports whether targets hold a target whose
// hosted agent reads instructions from a clone.
func hasCloudInstructionTarget(targets []string) bool {
	return slices.ContainsFunc(targets, func(t string) bool {
		_, ok := cloudInstructionAgents[t]
		return ok
	})
}

// cloudInstructionGap is a root instruction file Git ignores, and the
// hosted agents that start without it.
type cloudInstructionGap struct {
	Path   string
	Agents []string
}

// cloneSetupReport is what doctor found about files a clone lacks.
type cloneSetupReport struct {
	// Ignored counts generated files Git ignores and does not track.
	Ignored int
	// Freshness names what regenerates them, or "" when nothing does.
	Freshness string
	// Stale means ignored files have no freshness mechanism. It stays
	// false when the local checks were skipped.
	Stale bool
	Cloud []cloudInstructionGap
}

func (r cloneSetupReport) findings() int {
	n := len(r.Cloud)
	if r.Stale {
		n++
	}
	return n
}

// collectCloneSetup checks the generated paths of the project at root.
// local adds the check of this checkout's Git hooks; --checks project
// skips it, as it skips other local runtime state.
func collectCloneSetup(root string, cfg *config.Config, bundle spec.Bundle, generated []string, local bool) cloneSetupReport {
	var r cloneSetupReport
	if _, err := gitquery.LayoutAt(root); err != nil {
		return r
	}
	ignored := gitIgnoredUntracked(root, generated)
	r.Ignored = len(ignored)
	if r.Ignored == 0 {
		return r
	}
	switch {
	case cfg != nil && slices.Contains(cfg.Builtins, config.AutosyncBuiltin):
		// The autosync built-in is project config, so it counts even when
		// --checks project skips this checkout's Git hooks.
		r.Freshness = "builtin " + config.AutosyncBuiltin
	case local:
		r.Freshness = freshnessMechanism(root, bundle)
		r.Stale = r.Freshness == ""
	}
	r.Cloud = cloudInstructionGaps(cfg, bundle, generated, ignored, root)
	return r
}

// gitIgnoredUntracked returns the paths Git ignores. Git does not report
// a tracked path, so a committed file never counts.
func gitIgnoredUntracked(root string, paths []string) map[string]bool {
	want := map[string]bool{}
	for _, p := range paths {
		p = path.Clean(filepath.ToSlash(p))
		if p == "." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
			continue
		}
		want[p] = true
	}
	if len(want) == 0 {
		return nil
	}
	// --literal-pathspecs, which runGit sets, makes check-ignore fail.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--stdin", "-z")
	cmd.Env = gitquery.IsolatedEnv(os.Environ())
	cmd.Stdin = strings.NewReader(strings.Join(slices.Sorted(maps.Keys(want)), "\x00") + "\x00")
	// Exit 1 means none is ignored; any failure leaves the set empty.
	got, _ := cmd.Output()
	out := map[string]bool{}
	for _, p := range strings.Split(string(got), "\x00") {
		if p = path.Clean(filepath.ToSlash(p)); want[p] {
			out[p] = true
		}
	}
	return out
}

// cloudInstructionGaps groups the ignored root instruction files of the
// cloud targets by file. A target whose environment install runs sync
// regenerates its files in the cloud session, so it is left out.
func cloudInstructionGaps(cfg *config.Config, bundle spec.Bundle, generated []string, ignored map[string]bool, root string) []cloudInstructionGap {
	byPath := map[string][]string{}
	var order []string
	add := func(p, agent string) {
		if _, ok := byPath[p]; !ok {
			order = append(order, p)
		}
		if !slices.Contains(byPath[p], agent) {
			byPath[p] = append(byPath[p], agent)
		}
	}
	for _, target := range cfg.Targets {
		agent, ok := cloudInstructionAgents[target]
		if !ok || environmentInstallRunsSync(bundle, target, root) {
			continue
		}
		for _, p := range cloudInstructionPaths(cfg, target, generated) {
			if ignored[p] {
				add(p, agent)
			}
		}
	}
	slices.Sort(order)
	out := make([]cloudInstructionGap, 0, len(order))
	for _, p := range order {
		label := p
		if strings.HasPrefix(p, ".cursor/rules/") {
			label = ".cursor/rules/"
		}
		if n := len(out); n > 0 && out[n-1].Path == label {
			for _, a := range byPath[p] {
				if !slices.Contains(out[n-1].Agents, a) {
					out[n-1].Agents = append(out[n-1].Agents, a)
				}
			}
			continue
		}
		out = append(out, cloudInstructionGap{Path: label, Agents: byPath[p]})
	}
	return out
}

// cloudInstructionPaths returns the generated files that hold target's
// project instructions at the root: its entry point, and for Cursor,
// which has none, AGENTS.md and its rules.
func cloudInstructionPaths(cfg *config.Config, target string, generated []string) []string {
	var out []string
	entry := path.Clean(filepath.ToSlash(adapters.EntryPointPath(cfg, target)))
	for _, g := range generated {
		g = path.Clean(filepath.ToSlash(g))
		switch {
		case entry != "." && g == entry:
		case target == "cursor" && (g == "AGENTS.md" || strings.HasPrefix(g, ".cursor/rules/")):
		default:
			continue
		}
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

// lefthookConfigs are the file names lefthook reads its config from.
var lefthookConfigs = []string{
	"lefthook.yml", "lefthook.yaml", ".lefthook.yml", ".lefthook.yaml",
	"lefthook-local.yml", "lefthook-local.yaml", ".lefthook-local.yml", ".lefthook-local.yaml",
	".config/lefthook.yml", ".config/lefthook.yaml",
}

// freshnessGitEvents are the Git hooks that run after a pull or a branch
// switch.
var freshnessGitEvents = []string{"post-checkout", "post-merge"}

// freshnessMechanism names what regenerates ignored files after a pull,
// a branch switch, or at session start, or returns "" when nothing does:
// a Git hook from install-hook or one that runs sync, a hook manager
// config that does, a session-start hook spec, or an environment install.
func freshnessMechanism(root string, bundle spec.Bundle) string {
	if hooks, ok := runGit(root, "rev-parse", "--git-path", "hooks"); ok {
		dir := absFrom(root, strings.TrimSpace(hooks))
		for _, event := range freshnessGitEvents {
			data, err := os.ReadFile(filepath.Join(dir, event))
			if err != nil {
				continue
			}
			text := string(data)
			if postCheckoutHook.installedIn(text) || postMergeHook.installedIn(text) {
				return "install-hook --post-checkout"
			}
			if textRunsSync(text, root, 1) {
				return "Git " + event + " hook"
			}
		}
	}
	for _, name := range lefthookConfigs {
		if lefthookRunsSync(root, name) {
			return name
		}
	}
	for _, event := range freshnessGitEvents {
		name := ".husky/" + event
		if data, err := os.ReadFile(filepath.Join(root, name)); err == nil && textRunsSync(string(data), root, 1) {
			return name
		}
	}
	if preCommitRunsSync(root) {
		return ".pre-commit-config.yaml"
	}
	for _, hook := range bundle.Hooks {
		if hookStartsSession(hook.Meta) && metaCommandsRunSync(hook.Meta, "command", root) {
			return "session-start hook " + hook.Name
		}
	}
	for _, env := range bundle.Environments {
		if metaCommandsRunSync(env.Meta, "install", root) {
			return "environment " + env.Name
		}
	}
	return ""
}

func lefthookRunsSync(root, name string) bool {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return false
	}
	var doc map[string]any
	if yaml.Unmarshal(data, &doc) != nil {
		return false
	}
	for _, event := range freshnessGitEvents {
		for _, s := range yamlStrings(doc[event]) {
			if textRunsSync(s, root, 1) {
				return true
			}
		}
	}
	return false
}

func preCommitRunsSync(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, ".pre-commit-config.yaml"))
	if err != nil {
		return false
	}
	var doc struct {
		Repos []struct {
			Hooks []struct {
				Entry  string   `yaml:"entry"`
				Stages []string `yaml:"stages"`
			} `yaml:"hooks"`
		} `yaml:"repos"`
	}
	if yaml.Unmarshal(data, &doc) != nil {
		return false
	}
	for _, repo := range doc.Repos {
		for _, hook := range repo.Hooks {
			if slices.ContainsFunc(hook.Stages, func(s string) bool { return slices.Contains(freshnessGitEvents, s) }) && textRunsSync(hook.Entry, root, 1) {
				return true
			}
		}
	}
	return false
}

// yamlStrings returns every string value under v.
func yamlStrings(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case map[string]any:
		var out []string
		for _, child := range v {
			out = append(out, yamlStrings(child)...)
		}
		return out
	case []any:
		var out []string
		for _, child := range v {
			out = append(out, yamlStrings(child)...)
		}
		return out
	}
	return nil
}

// hookStartsSession reports a hook spec on the session start event, in
// its portable or native spelling.
func hookStartsSession(meta map[string]any) bool {
	if on, _ := meta["on"].(string); on == "session-start" {
		return true
	}
	event, _ := meta["event"].(string)
	event = strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(event))
	return event == "sessionstart"
}

// metaCommandsRunSync reports whether key, at the top level or under an
// x-<target> block, holds a command that runs sync.
func metaCommandsRunSync(meta map[string]any, key, root string) bool {
	if s, ok := meta[key].(string); ok && textRunsSync(s, root, 1) {
		return true
	}
	for k, v := range meta {
		block, ok := v.(map[string]any)
		if !ok || !strings.HasPrefix(k, "x-") {
			continue
		}
		if s, ok := block[key].(string); ok && textRunsSync(s, root, 1) {
			return true
		}
	}
	return false
}

// environmentInstallRunsSync reports an environment spec for target whose
// install runs sync, so the cloud session generates the files itself.
func environmentInstallRunsSync(bundle spec.Bundle, target, root string) bool {
	for _, env := range bundle.Environments {
		if !env.EmitsTo(target) {
			continue
		}
		if s, ok := adapters.ResolveMeta(env.Meta, target)["install"].(string); ok && textRunsSync(s, root, 1) {
			return true
		}
	}
	return false
}

var syncCommandLine = regexp.MustCompile(`agnostic-ai(?:\.exe)?["']?\s+(?:sync|project)\b`)
var syncWord = regexp.MustCompile(`\bsync\b`)

// checksOnly reports a command line that only compares or previews.
func checksOnly(line string) bool {
	return strings.Contains(line, "--check") || strings.Contains(line, "--dry-run") || strings.Contains(line, "--plan")
}

// textRunsSync reports whether a command or script runs agnostic-ai sync
// or project, directly or through a script in the project it names, up
// to depth scripts deep.
func textRunsSync(text, root string, depth int) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if syncCommandLine.MatchString(line) && !checksOnly(line) {
			return true
		}
		if depth <= 0 {
			continue
		}
		for _, script := range commandScriptPaths(line) {
			if scriptRunsSync(root, script, depth-1) {
				return true
			}
		}
	}
	return false
}

// scriptRunsSync reads a project script and reports whether it runs
// sync. A script that names agnostic-ai and runs a sync command through
// a variable, such as "$bin" sync, counts too.
func scriptRunsSync(root, rel string, depth int) bool {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<16 {
		return false
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return false
	}
	text := string(data)
	if textRunsSync(text, root, depth) {
		return true
	}
	if !strings.Contains(text, "agnostic-ai") {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "echo ") && syncWord.MatchString(line) && !checksOnly(line) {
			return true
		}
	}
	return false
}

var leadingVariable = regexp.MustCompile(`^\$(\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)/`)

// commandScriptPaths returns the words of a command line that name a
// script inside the project, such as .agnostic-ai/scripts/sync.sh or
// "$CLAUDE_PROJECT_DIR/scripts/sync.sh".
func commandScriptPaths(line string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == ';' || r == '&' || r == '|' }) {
		field = strings.Trim(field, `"'`)
		field = leadingVariable.ReplaceAllString(field, "")
		if field == "" || strings.HasPrefix(field, "-") || strings.HasPrefix(field, "$") || path.IsAbs(field) {
			continue
		}
		if !strings.Contains(field, "/") && !strings.HasSuffix(field, ".sh") {
			continue
		}
		clean := path.Clean(field)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		out = append(out, clean)
	}
	return out
}

// reportCloneSetup prints the clone and cloud agent block when a check
// found something, and stays silent otherwise.
func reportCloneSetup(cmd *cobra.Command, r cloneSetupReport) {
	if r.findings() == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Clones and cloud agents:")
	if r.Stale {
		noun := "files are"
		if r.Ignored == 1 {
			noun = "file is"
		}
		cmd.Printf("  ! %d generated %s ignored by Git, and nothing runs sync after a pull or branch switch, so tools read stale or missing files\n", r.Ignored, noun)
		cmd.Println("    → agnostic-ai install-hook --post-checkout")
	}
	for _, gap := range r.Cloud {
		verb := "start"
		if len(gap.Agents) == 1 {
			verb = "starts"
		}
		cmd.Printf("  ! %s is ignored, so %s %s without project instructions\n", gap.Path, joinAnd(gap.Agents), verb)
	}
	if len(r.Cloud) > 0 {
		cmd.Printf("    → commit them: set gitignore.commit: [instructions], run agnostic-ai sync, and commit the files (%s)\n", gitignoreCommitDocs)
	}
}

// joinAnd joins names as "a, b and c".
func joinAnd(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

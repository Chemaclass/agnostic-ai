package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// runningVersion is what the requires check compares. main.version
// cannot serve: a source build reports the last release. Go stamps a
// tagged checkout as vX.Y.Z, and a commit past a tag as a pseudo-version
// or go run as (devel), which the check does not place. A release
// candidate built from an untagged commit sets candidateVersion instead:
// go build -ldflags "-X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.74.0"
var runningVersion = candidateOr(candidateVersion, buildVersion(debug.ReadBuildInfo()))

// candidateVersion is the release a local build stands for, set only by
// -ldflags -X so that a plain source build stays unplaced.
var candidateVersion string

func candidateOr(candidate, build string) string {
	if candidate = strings.TrimSpace(candidate); candidate != "" {
		return "v" + strings.TrimPrefix(candidate, "v")
	}
	return build
}

// runningExecutable locates the binary, so an unmet requires can name
// the install command of the package manager that put it there.
var runningExecutable = os.Executable

// taggedBuildRE matches a tag with build metadata and no pre-release
// part. The release hook's go mod tidy can leave the tagged tree dirty,
// which stamps vX.Y.Z+dirty; that binary is still release X.Y.Z. A
// pseudo-version carries a pre-release part, so it does not match.
var taggedBuildRE = regexp.MustCompile(`^(v\d+\.\d+\.\d+)\+[0-9A-Za-z.-]+$`)

// requiresWarnOut and requiresWarned keep the warning for a build the
// check cannot place to once per config per run; the root command
// resets them.
var (
	requiresWarnOut io.Writer = os.Stderr
	requiresWarned            = map[string]bool{}
)

func buildVersion(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return ""
	}
	if m := taggedBuildRE.FindStringSubmatch(info.Main.Version); m != nil {
		return m[1]
	}
	return info.Main.Version
}

// requireGlobalVersion is requireVersion for the home configs, where a
// requires in local/agnostic-ai.yaml replaces the shared one and a null
// clears it. With skipBroken set, a config that does not parse warns
// there and is skipped instead of stopping the run.
func requireGlobalVersion(source string, skipBroken io.Writer) error {
	var requires, path string
	for _, p := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(p)
		if err != nil && skipBroken != nil {
			if _, werr := fmt.Fprintf(skipBroken, "warning: %v; skipping it\n", err); werr != nil {
				return fmt.Errorf("write global config warning: %w", werr)
			}
			continue
		}
		if err != nil {
			return err
		}
		node, ok := doc["requires"]
		if !ok {
			continue
		}
		requires, path = "", p
		if node.Tag == "!!null" {
			continue
		}
		if err := node.Decode(&requires); err != nil {
			return errs.Coded(errs.CodeConfigDecode, "%s: requires: %w", p, err)
		}
	}
	return requireVersion(path, requires)
}

// requireVersion stops the command when the running binary is outside
// requires, which source sets. A build that is not a release has
// no place in the order, so it warns and runs: contributors build from
// source.
func requireVersion(source, requires string) error {
	if requires == "" {
		return nil
	}
	req, err := config.ParseRequirement(requires)
	if err != nil {
		return errs.Coded(errs.CodeConfigDecode, "%s: requires: %w", source, err)
	}
	allowed, release := req.Allows(runningVersion)
	running := strings.TrimPrefix(runningVersion, "v")
	if !release {
		key := source + "\x00" + requires
		if verbosity < levelDefault || requiresWarned[key] {
			return nil
		}
		requiresWarned[key] = true
		if running == "" {
			running = "(unknown)"
		}
		if _, err := fmt.Fprintf(requiresWarnOut, "warning: %s: requires %s, but %s is not a release build; not checked\n", source, req, running); err != nil {
			return fmt.Errorf("write requires warning: %w", err)
		}
		return nil
	}
	if !allowed {
		return errs.Coded(errs.CodeRequiresUnmet, "%s requires agnostic-ai %s, but %s is installed; %s", source, req, running, requiresFix(req, source))
	}
	return nil
}

// requiresFix names the command that puts a binary outside req inside it.
// A minimum alone is met by the latest release; anything else needs a
// named one, since upgrade would jump past an upper bound. A binary a
// package manager installed for the project that source configures gets
// that manager's command, since upgrade cannot replace it.
func requiresFix(req config.Requirement, source string) string {
	version, latest := req.InstallTarget()
	if pm, ok := projectPackageManager(source); ok {
		switch {
		case latest:
			return "run `" + pm.add + " " + binaryName + "@latest`"
		case version != "":
			return "run `" + pm.install + "`, or `" + pm.add + " " + binaryName + "@" + version + "` if package.json pins another release"
		default:
			return "run `" + pm.add + " " + binaryName + "@X.Y.Z` with a release inside it"
		}
	}
	switch {
	case latest:
		return "run `agnostic-ai upgrade`"
	case version != "":
		return "run `agnostic-ai upgrade --version v" + version + "`"
	default:
		return "run `agnostic-ai upgrade --version vX.Y.Z` with a release inside it"
	}
}

// packageManager holds the commands that install a project's packages
// and add one as a dev dependency.
type packageManager struct{ install, add string }

// projectLockfiles map a lockfile to the manager that writes it, in the
// order they are checked; a project with none uses npm.
var projectLockfiles = []struct {
	file string
	pm   packageManager
}{
	{"pnpm-lock.yaml", packageManager{"pnpm install", "pnpm add -D"}},
	{"yarn.lock", packageManager{"yarn install", "yarn add -D"}},
	{"bun.lock", packageManager{"bun install", "bun add -D"}},
	{"bun.lockb", packageManager{"bun install", "bun add -D"}},
}

// projectPackageManager reports the package manager that installed the
// running binary into a node_modules of the project holding one of the
// config files source names (joined with " + " when layered). A global
// install sits in a node_modules outside that project, so it gets the
// upgrade advice.
func projectPackageManager(source string) (packageManager, bool) {
	exe, err := runningExecutable()
	if err != nil || source == "" {
		return packageManager{}, false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	sep := string(filepath.Separator)
	i := strings.Index(exe, sep+"node_modules"+sep)
	if i < 0 {
		return packageManager{}, false
	}
	project := exe[:i]
	for _, s := range strings.Split(source, " + ") {
		if within(project, s) {
			return lockfileManager(project), true
		}
	}
	return packageManager{}, false
}

// within reports whether the config file path sits in dir or below it,
// comparing the paths as given and with symlinks resolved.
func within(dir, path string) bool {
	abs, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return false
	}
	inside := func(dir, abs string) bool {
		rel, err := filepath.Rel(dir, abs)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if inside(dir, abs) {
		return true
	}
	rdir, err1 := filepath.EvalSymlinks(dir)
	rabs, err2 := filepath.EvalSymlinks(abs)
	return err1 == nil && err2 == nil && inside(rdir, rabs)
}

// lockfileManager picks the manager from the nearest lockfile at or above
// project, stopping at the repository root, so a workspace package whose
// lockfile sits at the workspace root gets the workspace's manager. At a
// pnpm workspace root, adding a dependency needs -w.
func lockfileManager(project string) packageManager {
	for dir := project; ; dir = filepath.Dir(dir) {
		for _, l := range projectLockfiles {
			if _, err := os.Stat(filepath.Join(dir, l.file)); err != nil {
				continue
			}
			pm := l.pm
			if _, err := os.Stat(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil && l.file == "pnpm-lock.yaml" && dir == project {
				pm.add += " -w"
			}
			return pm
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || filepath.Dir(dir) == dir {
			return packageManager{"npm install", "npm install -D"}
		}
	}
}

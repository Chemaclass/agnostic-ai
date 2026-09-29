package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// runningVersion is what the requires check compares. main.version
// cannot serve: a source build reports the last release. Go stamps a
// tagged checkout as vX.Y.Z, and a commit past a tag as a pseudo-version
// or go run as (devel), which the check does not place.
var runningVersion = buildVersion(debug.ReadBuildInfo())

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
		return errs.Coded(errs.CodeRequiresUnmet, "%s requires agnostic-ai %s, but %s is installed; %s", source, req, running, requiresFix(req))
	}
	return nil
}

// requiresFix names the command that puts a binary outside req inside it.
// A minimum alone is met by the latest release; anything else needs a
// named one, since upgrade would jump past an upper bound.
func requiresFix(req config.Requirement) string {
	version, latest := req.InstallTarget()
	switch {
	case latest:
		return "run `agnostic-ai upgrade`"
	case version != "":
		return "run `agnostic-ai upgrade --version v" + version + "`"
	default:
		return "run `agnostic-ai upgrade --version vX.Y.Z` with a release inside it"
	}
}

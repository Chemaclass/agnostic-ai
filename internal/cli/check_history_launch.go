package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func (h *historyRenderer) pathMatches(commit, p string) bool {
	if rendered, seen := h.renders[commit]; seen {
		return renderedContentMatches(rendered, p)
	}
	if p == ".claude/launch.json" && h.launchDefinitelyAbsent(commit) {
		return false
	}
	return renderedContentMatches(h.rendered(commit), p)
}

func (h *historyRenderer) launchDefinitelyAbsent(commit string) bool {
	if absent, seen := h.absentLaunch[commit]; seen {
		return absent
	}
	if h.absentLaunch == nil {
		h.absentLaunch = map[string]bool{}
	}
	absent := h.inspectLaunchAbsence(commit)
	h.absentLaunch[commit] = absent
	return absent
}

func (h *historyRenderer) inspectLaunchAbsence(commit string) bool {
	if !historyCanonicalSources(h.sources) || !historyLaunchPath(h.prefix) {
		return false
	}
	// Checkout filters can create sources beyond the committed environment tree.
	_, err := gitOutput(h.toplevel, nil, "config", "--name-only", "--get-regexp", "^filter\\.")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return false
	}
	tree, err := gitOutput(h.toplevel, nil, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return false
	}
	canonicalConfig := false
	for _, record := range strings.Split(tree, "\x00") {
		if record == "" {
			continue
		}
		meta, p, ok := strings.Cut(record, "\t")
		if !ok {
			return false
		}
		rel, inside := strings.CutPrefix(p, h.prefix)
		if !inside {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 || fields[1] != "blob" || fields[0] != "100644" && fields[0] != "100755" || !historyLaunchPath(rel) {
			return false
		}
		lower := strings.ToLower(rel)
		for _, root := range []string{".agnostic-ai/environments", ".agnostic-ai/local/environments", ".agnostic-ai/packs"} {
			if lower == root || strings.HasPrefix(lower, root+"/") {
				return false
			}
		}
		if lower == config.LocalOverrideFileName || lower == packsLockfile || lower == ".agnostic-ai/overlays/claude/launch.json" || strings.HasPrefix(lower, ".agnostic-ai/overlays/claude/launch.json/") {
			return false
		}
		if lower == config.ConfigFileName && rel != config.ConfigFileName {
			return false
		}
		if rel == config.ConfigFileName {
			canonicalConfig = true
		}
	}
	if !canonicalConfig {
		return false
	}
	cfg, err := h.launchHistoryConfig(commit)
	if err != nil || len(cfg.Outputs) != 0 || len(cfg.Builtins) != 0 || !historyCanonicalSources(configuredSources(cfg)) {
		return false
	}
	for _, target := range cfg.Targets {
		if _, known := adapters.Get(target); !known {
			return false
		}
	}
	return true
}

func (h *historyRenderer) launchHistoryConfig(commit string) (*config.Config, error) {
	scratch, err := os.MkdirTemp("", "agnostic-ai-history-config-")
	if err != nil {
		return nil, fmt.Errorf("create historical config directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	root := filepath.Join(scratch, "tree")
	project := filepath.Join(root, filepath.FromSlash(h.prefix))
	if err := os.MkdirAll(project, 0755); err != nil {
		return nil, fmt.Errorf("%s: %w", project, err)
	}
	env := append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(scratch, "index"))
	if _, err := gitOutput(h.toplevel, env, "read-tree", commit); err != nil {
		return nil, err
	}
	if err := checkoutPaths(h.toplevel, env, root, []string{h.prefix + config.ConfigFileName}); err != nil {
		return nil, err
	}
	return config.Load(project)
}

func historyCanonicalSources(sources []string) bool {
	if len(sources) == 0 {
		return true
	}
	kinds := []string{"agents", "skills", "rules", "hooks", "mcps", "commands", "settings", "reviews", "environments", "ignore"}
	if len(sources) != len(kinds) {
		return false
	}
	for i, source := range sources {
		if source != "" && source != config.SourceBaseDir+"/"+kinds[i] {
			return false
		}
	}
	return true
}

func historyLaunchPath(s string) bool {
	if strings.ContainsAny(s, "\\:~") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}

	for _, r := range s {
		if r >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

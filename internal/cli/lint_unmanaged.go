package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// lintUnmanagedDoubleStar reports each sync.unmanaged entry that uses `**`
// (LINT057, warning), naming path as the file that holds the list. The
// matcher reads it as `*`.
func lintUnmanagedDoubleStar(cfg *config.Config, path string) []lintFinding {
	if cfg == nil {
		return nil
	}
	var out []lintFinding
	for _, entry := range config.DoubleStarUnmanaged(cfg.Sync.Unmanaged) {
		out = append(out, lintFinding{
			Code: "LINT057", Severity: lintWarn, Path: path,
			Message: fmt.Sprintf("sync.unmanaged: %q uses **, which acts like * and stays inside one folder name; %s", entry, doubleStarAdvice(entry)),
		})
	}
	return out
}

// doubleStarAdvice says what the entry really matches and what to write.
func doubleStarAdvice(entry string) string {
	if dir, ok := strings.CutSuffix(entry, "/**"); ok && !strings.ContainsAny(dir, "*?[\\") {
		return "it matches only files directly inside " + dir + "/; write " + dir + "/ to own everything under it"
	}
	if strings.HasPrefix(entry, "**/") {
		return "it matches one folder deep, so it misses the root and deeper folders; add one entry per depth"
	}
	return "write each folder level out, one entry per depth"
}

// noteUnmanagedDoubleStar prints the LINT057 findings as sync notes.
func noteUnmanagedDoubleStar(cfg *config.Config, root string) {
	for _, f := range lintUnmanagedDoubleStar(cfg, unmanagedSource(root)) {
		adapters.NoteProject(f.Path + ": " + f.Message)
	}
}

// unmanagedSource names the config file whose sync.unmanaged list is in
// effect: the local file replaces the base list whole when it sets the
// key, even to null.
func unmanagedSource(root string) string {
	if data, err := os.ReadFile(filepath.Join(root, config.LocalOverrideFileName)); err == nil {
		var local map[string]any
		if yaml.Unmarshal(data, &local) == nil {
			if sync, ok := local["sync"].(map[string]any); ok {
				if _, set := sync["unmanaged"]; set {
					return config.LocalOverrideFileName
				}
			}
		}
	}
	if base, _, err := config.ResolveConfigPath(root); err == nil {
		return filepath.Base(base)
	}
	return config.ConfigFileName
}

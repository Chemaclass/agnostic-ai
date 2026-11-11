package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// sharedRulesFileConflicts names each target whose merged rules-file is
// its own instructions file while sync also writes that file for another
// target. The merged file has no AGNOSTIC_AI.md body and only the
// owner's rules, and the file sync writes for the other readers drops
// them, so no single version is right for both (#3302).
func sharedRulesFileConflicts(cfg *config.Config, targets []string) []string {
	all := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(cfg.Targets), targets...))))
	var out []string
	for _, owner := range all {
		if !adapters.LegacyRulesFileOwnsEntryPoint(cfg, owner) {
			continue
		}
		path := adapters.EntryPointPath(cfg, owner)
		if cfg.IsUnmanaged(path) {
			continue
		}
		var readers []string
		for _, r := range all {
			if r != owner && !adapters.LegacyRulesFileOwnsEntryPoint(cfg, r) && filepath.Clean(adapters.EntryPointPath(cfg, r)) == filepath.Clean(path) {
				readers = append(readers, r)
			}
		}
		if len(readers) == 0 {
			continue
		}
		names := strings.Join(readers, ", ")
		fileKey := "outputs.<target>.file"
		if len(readers) == 1 {
			fileKey = "outputs." + readers[0] + ".file"
		}
		out = append(out, fmt.Sprintf("%s: outputs.%s.rules-file puts %s's merged rules there, and sync also writes it as the instructions file of %s. The merged file holds neither the AGNOSTIC_AI.md body nor the rules of %s. Point outputs.%s.rules-file elsewhere, remove it, or set %s",
			path, owner, owner, names, names, owner, fileKey))
	}
	return out
}

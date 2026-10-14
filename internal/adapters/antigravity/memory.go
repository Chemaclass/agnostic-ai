package antigravity

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func nativeMemoryRules(rules []spec.Entry, cfg *config.Config, entryDir func(spec.Entry) string) ([]spec.Entry, error) {
	out := slices.Clone(rules)
	for i, entry := range out {
		if entry.BodySourceLayer() != "builtin" || entry.Name != "antigravity-memory-index" {
			continue
		}
		if cfg.RepoPersonalMemory() {
			entry.Body, _, _ = strings.Cut(entry.Body, "\nPersonal memory, `.agnostic-ai/local/memory/MEMORY.md`:\n")
			entry.Body = strings.TrimRight(entry.Body, "\n") + "\n"
		}
		for _, index := range []string{emit.ProjectMemoryIndexPath, emit.PersonalMemoryIndexPath} {
			old := "](../../" + index + ")"
			if !strings.Contains(entry.Body, old) {
				continue
			}
			ref, err := nativeMemoryIndexRef(entryDir(entry), index)
			if err != nil {
				return nil, err
			}
			entry.Body = strings.ReplaceAll(entry.Body, old, "]("+ref+")")
		}
		out[i] = entry
	}
	return out, nil
}

func nativeMemoryIndexRef(dir, index string) (string, error) {
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	path, err := filepath.Abs(filepath.FromSlash(index))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", index, err)
	}
	ref, err := filepath.Rel(base, path)
	if err != nil {
		ref = path
	}
	return filepath.ToSlash(ref), nil
}

package antigravity

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
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

func nativeMemoryRuleBytes(entry spec.Entry, dir string) (int, error) {
	size := len(rule(entry))
	if entry.BodySourceLayer() != "builtin" || entry.Name != "antigravity-memory-index" {
		return size, nil
	}
	for _, index := range []struct{ label, path string }{
		{"Project memory", emit.ProjectMemoryIndexPath},
		{"Personal memory", emit.PersonalMemoryIndexPath},
	} {
		ref, err := nativeMemoryIndexRef(dir, index.path)
		if err != nil {
			return 0, err
		}
		include := "@[" + index.label + "](" + ref + ")"
		count := strings.Count(entry.Body, include)
		if count == 0 {
			continue
		}
		path := filepath.FromSlash(ref)
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		body, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		size += count * (len(nativeMemoryIndexBody(body)) - len(include))
	}
	return size, nil
}

func nativeMemoryIndexBody(data []byte) []byte {
	line, rest, ok := bytes.Cut(data, []byte("\n"))
	if !ok || !bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), []byte("---")) {
		return data
	}
	for len(rest) > 0 {
		line, tail, ok := bytes.Cut(rest, []byte("\n"))
		if bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), []byte("---")) {
			return tail
		}
		if !ok {
			break
		}
		rest = tail
	}
	return data
}

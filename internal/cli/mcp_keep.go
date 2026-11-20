package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// unledgeredMCPFile is a whole MCP file a run writes that the ledger
// does not list: sync never wrote it, so a hand edit there has no backup.
type unledgeredMCPFile struct {
	path, target, key string
	written           map[string]bool
}

// unledgeredMCPFiles returns the whole MCP files the run's targets write
// that are on disk, not ledgered, and not under sync.unmanaged.
func unledgeredMCPFiles(renders *targetRenders, cfg *config.Config, targets, ledgered []string) []unledgeredMCPFile {
	cfg = cfg.WithAdditionalTargets(targets...)
	var out []unledgeredMCPFile
	seen := map[string]bool{}
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		for _, f := range renders.capture(adapter, cfg).files {
			slash := filepath.ToSlash(f.Path)
			if f.Merged || seen[slash] || cfg.IsUnmanaged(slash) || slices.Contains(ledgered, slash) {
				continue
			}
			key, written, ok := wholeMCPDocument(f.Content)
			if !ok || !fileExists(f.Path) {
				continue
			}
			seen[slash] = true
			out = append(out, unledgeredMCPFile{path: f.Path, target: t, key: key, written: written})
		}
	}
	return out
}

// keepHandWrittenMCPFiles stops a sync before it replaces a whole MCP
// file, such as .cursor/mcp.json, that holds servers or keys agnostic-ai
// did not write and no spec has. It follows keepHandWrittenInstructions:
// --backup replaces the file when its `<path>.bak` is free, and
// --keep-edits leaves out a file Git tracks with edits.
func keepHandWrittenMCPFiles(renders *targetRenders, cfg *config.Config, targets, ledgered []string, backup, keepEdits bool) error {
	var lines []string
	for _, f := range unledgeredMCPFiles(renders, cfg, targets, ledgered) {
		if keepEdits {
			if sum := committedSum(f.path); sum != "" && fileSum(f.path) != sum {
				continue
			}
		}
		lost, err := handWrittenMCPEntries(f.path, f.key, f.written)
		if err != nil {
			return fmt.Errorf("%w; nothing was written", err)
		}
		if len(lost) == 0 {
			continue
		}
		path := filepath.ToSlash(f.path)
		if backup {
			if _, err := os.Lstat(f.path + ".bak"); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s holds MCP config agnostic-ai did not write, and %s.bak already exists, so --backup would replace it.\n  move %s.bak away, then run agnostic-ai sync --backup", path, path, path))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s holds MCP config agnostic-ai did not write and no spec has (%s).\n  keep it:    %s\n  replace it: agnostic-ai sync --backup, which keeps the file as %s.bak",
			path, strings.Join(lost, ", "), mcpKeepStep(cfg, path), path))
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("%s\nnothing was written", strings.Join(lines, "\n"))
}

// unledgeredMCPSum stands in for the ledger sum of an unledgered MCP
// file. No content sums to it, so any bytes on disk count as an edit. The
// ledger never stores it.
const unledgeredMCPSum = "unledgered"

// backingUpMCPFiles makes the edit guard treat each unledgered MCP file
// as a hand edit: kept as `<path>.bak` before the first write over it,
// or left in place under --keep-edits. Import may have read the servers
// back in another form, such as a literal key as a `${NAME}` reference.
func (g editGuard) backingUpMCPFiles(files []unledgeredMCPFile) editGuard {
	if len(files) == 0 {
		return g
	}
	sums := make(map[string]string, len(g.sums)+len(files))
	maps.Copy(sums, g.sums)
	for _, f := range files {
		sums[f.path] = unledgeredMCPSum
	}
	g.sums = sums
	return g
}

// wholeMCPDocument reports whether content is an MCP file sync writes
// whole: a JSON object with one key, `mcpServers` or `servers`, holding
// the servers. It returns that key and the server names.
func wholeMCPDocument(content string) (string, map[string]bool, bool) {
	var doc map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &doc) != nil || len(doc) != 1 {
		return "", nil, false
	}
	for _, key := range []string{"mcpServers", "servers"} {
		raw, ok := doc[key]
		if !ok {
			continue
		}
		var servers map[string]json.RawMessage
		if json.Unmarshal(raw, &servers) != nil {
			return "", nil, false
		}
		names := make(map[string]bool, len(servers))
		for name := range servers {
			names[name] = true
		}
		return key, names, true
	}
	return "", nil, false
}

// handWrittenMCPEntries returns what the file at path holds that a write
// of the written servers under key would drop: other servers and other
// top-level keys. A file that does not parse is lost whole. A missing
// file holds nothing.
func handWrittenMCPEntries(path, key string, written map[string]bool) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	data, _ = adapters.StripJSONC(data)
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return []string{"the file does not parse as JSON"}, nil
	}
	var lost []string
	for k, raw := range doc {
		if k != key {
			if !emptyJSON(raw) {
				lost = append(lost, "key "+k)
			}
			continue
		}
		var servers map[string]json.RawMessage
		if json.Unmarshal(raw, &servers) != nil {
			lost = append(lost, "key "+k)
			continue
		}
		for name := range servers {
			if !written[name] {
				lost = append(lost, "server "+name)
			}
		}
	}
	sort.Strings(lost)
	return lost, nil
}

// emptyJSON reports whether raw is null or an empty object or array.
func emptyJSON(raw json.RawMessage) bool {
	switch strings.Join(strings.Fields(string(raw)), "") {
	case "null", "{}", "[]":
		return true
	}
	return false
}

// mcpFileImporters maps each whole MCP file an importer reads to that
// importer.
var mcpFileImporters = map[string]string{
	filepath.ToSlash(cursorMCPFile): "cursor",
	claudeMCPFile:                   "claude",
	kiroMCPFile:                     "kiro",
	".junie/mcp/mcp.json":           "junie",
	factoryMCPFile:                  "factory",
	traeMCPFile:                     "trae",
	windsurfMCPFile:                 "windsurf",
	warpMCPFile:                     "warp",
}

// mcpKeepStep is the step that brings the servers of the MCP file at
// path into specs: the importer that reads it, or adding them by hand.
func mcpKeepStep(cfg *config.Config, path string) string {
	if source := mcpFileImporters[path]; source != "" {
		return "agnostic-ai import " + source
	}
	if path == filepath.ToSlash(antigravityMCPFileFromCfg(cfg)) {
		return "agnostic-ai import antigravity"
	}
	return "add its servers to .agnostic-ai/mcps/, then run agnostic-ai sync"
}

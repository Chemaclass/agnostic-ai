package claude

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const rejectionKey = "disabledMcpjsonServers"

type mcpRejections struct {
	path     string
	owned    []string
	disabled []string
	active   bool
}

func withoutMCPDisabled(entries []spec.Entry) []spec.Entry {
	out := slices.Clone(entries)
	for i := range out {
		out[i].Meta = maps.Clone(out[i].Meta)
		delete(out[i].Meta, "disabled")
	}
	return out
}

func readMCPRejections(entries []spec.Entry, dir string, cfg *config.Config) (mcpRejections, error) {
	p := mcpRejections{path: filepath.Join(dir, ".agnostic-ai-mcp-disabled.json")}
	for _, entry := range entries {
		if disabled, _ := entry.Meta["disabled"].(bool); disabled {
			p.disabled = append(p.disabled, entry.Name)
		}
	}
	slices.Sort(p.disabled)
	if len(p.disabled) > 0 && filepath.Clean(emit.OutputMCPFile(cfg, target, defaultMCPFile)) != defaultMCPFile {
		return p, fmt.Errorf("claude: disabled MCP servers require project .mcp.json; remove outputs.claude.mcp-file override")
	}
	raw, err := os.ReadFile(p.path)
	if err != nil && !emit.IsAbsent(err) {
		return p, fmt.Errorf("%s: %w", p.path, err)
	}
	if err == nil {
		if err := json.Unmarshal(raw, &p.owned); err != nil {
			return p, fmt.Errorf("parse %s: %w", p.path, err)
		}
	}
	p.active = len(p.disabled) > 0 || err == nil
	return p, nil
}

func rejectionNames(doc *emit.OrderedJSON) ([]string, error) {
	raw, ok := doc.Get(rejectionKey)
	if !ok {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("claude settings: parse %s: %w", rejectionKey, err)
	}
	return names, nil
}

func (p mcpRejections) removeOwned(doc *emit.OrderedJSON) error {
	if !p.active {
		return nil
	}
	names, err := rejectionNames(doc)
	if err != nil {
		return err
	}
	names = slices.DeleteFunc(names, func(name string) bool { return slices.Contains(p.owned, name) })
	if len(names) == 0 {
		doc.Delete(rejectionKey)
		return nil
	}
	return doc.Set(rejectionKey, names)
}

func (p mcpRejections) apply(sess *emit.Session, doc *emit.OrderedJSON, dryRun bool) error {
	if !p.active {
		return nil
	}
	names, err := rejectionNames(doc)
	if err != nil {
		return err
	}
	owned := []string{}
	for _, name := range p.disabled {
		if !slices.Contains(names, name) {
			names = append(names, name)
			owned = append(owned, name)
		}
	}
	if len(names) > 0 {
		if err := doc.Set(rejectionKey, names); err != nil {
			return fmt.Errorf("claude settings: rejection list: %w", err)
		}
	}
	raw, err := json.MarshalIndent(owned, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", p.path, err)
	}
	return sess.WriteFile(p.path, string(raw)+"\n", dryRun)
}

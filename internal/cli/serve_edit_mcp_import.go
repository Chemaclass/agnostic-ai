package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func previewMCPSpec(target, name, path, format string, server map[string]any) ([]byte, []string, error) {
	if err := spec.ValidateMCPNames([]string{name}); err != nil {
		return nil, nil, err
	}
	copy, ok := jsonRoundTrip(server).(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s: MCP server %s is not an object", path, name)
	}
	servers := map[string]any{name: copy}
	switch target {
	case "codex":
		if format != "toml" {
			return nil, nil, fmt.Errorf("%s: unsupported Codex MCP format", path)
		}
		_, native, err := readCodexConfigTOMLFile(path)
		if err != nil {
			return nil, nil, err
		}
		entry, ok := native[name]
		if !ok {
			return nil, nil, fmt.Errorf("%s: MCP server %s no longer exists", path, name)
		}
		servers = codexMCPDocs(map[string]codexMCPEntry{name: entry})
	case "grok":
		servers = grokMCPDocs(servers)
	case "opencode", "kilo":
		servers[name] = normalizeOpencodeMCPEntry(copy)
	case "windsurf":
		normalizeWindsurfMCPEntry(copy)
	case "vibe":
		copy["name"] = name
		var err error
		servers, err = vibeMCPDocs([]map[string]any{copy})
		if err != nil {
			return nil, nil, err
		}
	case "zed":
		servers = zedMCPDocs(servers)
	case "factory":
		normalizeFactoryMCPEntry(copy)
	}
	if target != "codex" {
		normalizeImportedMCP(target, servers)
	}
	for _, ref := range referenceMCPLiterals(servers) {
		if ref.leftOut {
			return nil, nil, mcpImportCredentialError(name, ref.field)
		}
	}
	entry, ok := servers[name].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s: MCP server %s has no importable command or URL", path, name)
	}
	variables := map[string]bool{}
	unsafe := ""
	walkMCPStrings("", "", jsonRoundTrip(entry), func(field, key, value string) {
		if unsafe == "" && (preview.CredentialDetected(value) || preview.CredentialKey(key) && !preview.RefOnly(value) && preview.SecretValue(preview.DetectorText(value), false, preview.WeakCredential)) {
			unsafe = strings.TrimPrefix(field, ".")
		}
		for _, token := range spec.EnvRefTokens(value) {
			if token.Known() && !token.EditorVariable() {
				variables[token.Name] = true
			}
		}
	})
	if unsafe != "" {
		return nil, nil, mcpImportCredentialError(name, unsafe)
	}
	delete(entry, "name")
	data, err := encodeMCPSpec(name, entry, nil)
	if err != nil {
		return nil, nil, err
	}
	return data, slices.Sorted(maps.Keys(variables)), nil
}

func mcpImportCredentialError(name, field string) error {
	return fmt.Errorf("MCP server %s: left out; %s holds a credential import cannot rewrite", name, field)
}

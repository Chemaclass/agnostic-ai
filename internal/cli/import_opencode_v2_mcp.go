package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func readOpencodeMCP(root string) (map[string]any, bool, error) {
	path := filepath.Join(root, opencodeMCPFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	raw, _ = adapters.StripJSONC(raw)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	mcp, _ := doc[opencodeMCPKey].(map[string]any)
	servers, envelope := mcp["servers"].(map[string]any)
	if !envelope {
		return mcp, false, nil
	}
	if opencodeV1MCPEntry(servers) {
		for key, value := range servers {
			switch key {
			case "headers", "environment", "oauth", "timeout", "requestOptions":
				continue
			}
			if strings.HasPrefix(key, "x-") {
				continue
			}
			if entry, object := value.(map[string]any); object && opencodeV1MCPEntry(entry) {
				return nil, false, fmt.Errorf("parse %s: mcp.servers mixes a V1 entry with a V2 server %q", path, key)
			}
		}
		return mcp, false, nil
	}
	for key, value := range mcp {
		entry, object := value.(map[string]any)
		if key != "servers" && object && opencodeV1MCPEntry(entry) {
			return nil, false, fmt.Errorf("parse %s: mcp mixes V2 servers with a V1 server %q", path, key)
		}
	}
	for name, value := range servers {
		if _, object := value.(map[string]any); !object {
			return nil, false, fmt.Errorf("parse %s: mcp.servers.%s must be an object", path, name)
		}
	}
	return mcp, true, nil
}

func opencodeV1MCPEntry(entry map[string]any) bool {
	_, commandString := entry["command"].(string)
	_, commandArray := entry["command"].([]any)
	_, url := entry["url"].(string)
	typeName, _ := entry["type"].(string)
	return commandString || commandArray || url || typeName == "local" || typeName == "remote"
}

func planOpencodeV2MCP(root string, sources config.Sources) (map[string]any, error) {
	mcp, native, err := readOpencodeMCP(root)
	if err != nil || !native {
		return nil, err
	}
	path := filepath.Join(root, opencodeMCPFile)
	ownerSources := config.Sources{
		MCPs:     importSourcePath(root, sources.MCPs),
		Settings: importSourcePath(root, sources.Settings),
	}
	layers, err := resolveLayers(root, &config.Config{Sources: ownerSources})
	if err != nil {
		return nil, fmt.Errorf("import %s: resolve existing spec owners: %w", path, err)
	}
	bundle, err := spec.LoadLayered(layers)
	if err != nil {
		return nil, fmt.Errorf("import %s: inspect existing specs: %w", path, err)
	}
	if entries := bundle.For("opencode").MCPs; len(entries) > 0 {
		return nil, fmt.Errorf("import %s: native V2 MCP settings conflict with portable MCP spec %s", path, entries[0].Path)
	}
	settingsPath := filepath.Join(importSourcePath(root, sources.Settings), "opencode-mcp.yaml")
	for _, entry := range bundle.For("opencode").Settings {
		x, _ := entry.Meta["x-opencode"].(map[string]any)
		if _, owns := x["mcp"]; owns && filepath.Clean(entry.Path) != filepath.Clean(settingsPath) {
			return nil, fmt.Errorf("import %s: native V2 MCP settings conflict with settings spec %s", path, entry.Path)
		}
	}
	servers := mcp["servers"].(map[string]any)
	normalized := map[string]any{}
	for name, raw := range servers {
		entry := normalizeOpencodeMCPEntry(raw.(map[string]any))
		unknown := maps.Clone(entry)
		for _, key := range []string{"command", "args", "url", "env", "headers", "disabled"} {
			delete(unknown, key)
		}
		entry["x-opencode"] = opencodeNativeCredentialView(unknown)
		adapters.EscapeMCPLiterals("opencode", entry)
		adapters.ReadMCPEnvRefs("opencode", entry)
		normalized[name] = entry
	}
	refs := referenceMCPLiterals(normalized)
	for _, ref := range refs {
		if ref.leftOut {
			return nil, fmt.Errorf("import %s: mcp.servers.%s.%s holds a credential import cannot rewrite", path, ref.server, ref.field)
		}
	}
	defaults := maps.Clone(mcp)
	delete(defaults, "servers")
	unsafe := ""
	walkMCPStrings("mcp", "mcp", opencodeNativeCredentialView(defaults), func(field, key, value string) {
		if unsafe == "" && (preview.CredentialDetected(value) || preview.CredentialKey(key) && !preview.RefOnly(value) && preview.SecretValue(preview.DetectorText(value), false, preview.WeakCredential)) {
			unsafe = field
		}
	})
	if unsafe != "" {
		return nil, fmt.Errorf("import %s: %s holds a credential import cannot rewrite", path, unsafe)
	}
	for name, raw := range normalized {
		entry := raw.(map[string]any)
		nativeEntry := servers[name].(map[string]any)
		for nativeKey, portableKey := range map[string]string{"environment": "env", "headers": "headers"} {
			if values, ok := entry[portableKey].(map[string]any); ok {
				for key, value := range values {
					if text, ok := value.(string); ok {
						values[key] = opencodeNativeReference(text)
					}
				}
				nativeEntry[nativeKey] = values
			}
		}
		if url, ok := entry["url"].(string); ok {
			nativeEntry["url"] = opencodeNativeReference(url)
		}
		if command, ok := nativeEntry["command"].([]any); ok {
			args, _ := entry["args"].([]string)
			for i, arg := range args {
				command[i+1] = opencodeNativeReference(arg)
			}
		}
	}
	reportMCPLiteralRefs(refs)
	return mcp, nil
}

func opencodeNativeReference(value string) string {
	return spec.DecodeEnvRefEscapes(spec.EnvRefBraceEnv.WriteLaunch(value))
}

func opencodeNativeCredentialView(value any) any {
	switch value := value.(type) {
	case string:
		return spec.EnvRefBraceEnv.ReadLaunch(spec.EscapeEnvRefs(value), spec.EnvRefReading{})
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, child := range value {
			out[key] = opencodeNativeCredentialView(child)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, child := range value {
			out[i] = opencodeNativeCredentialView(child)
		}
		return out
	default:
		return value
	}
}

func writeOpencodeV2MCP(mcp map[string]any, dstDir string) error {
	doc := map[string]any{"target": "opencode", "x-opencode": map[string]any{"mcp": mcp}}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal native OpenCode MCP settings: %w", err)
	}
	path := filepath.Join(dstDir, "opencode-mcp.yaml")
	if err := importWriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

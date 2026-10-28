package cli

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/grok"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// nativeMCPFile is where a target keeps project MCP servers: a JSON map
// at key, a TOML table (toml), an array of TOML tables (toml-array), or
// OpenCode's mcp block (opencode). noImport marks a file that
// `agnostic-ai import` does not read.
type nativeMCPFile struct {
	path, format, key string
	noImport          bool
}

// projectNativeMCP lists each target's project MCP file, as its importer
// reads it. A path a target's adapter declares wins over path, so an
// outputs override is honoured.
var projectNativeMCP = map[string]nativeMCPFile{
	"amp":         {path: ".amp/settings.json", format: "json", key: "amp.mcpServers"},
	"antigravity": {path: antigravityMCPFile, format: "json", key: antigravityMCPKey},
	"claude":      {path: claudeMCPFile, format: "json", key: claudeMCPKey},
	"codebuddy":   {format: "json", key: "mcpServers"},
	"codex":       {path: codexConfigTOML, format: "toml", key: "mcp_servers"},
	"copilot":     {path: copilotMCPFile, format: "json", key: "servers"},
	"crush":       {path: crushMCPFile, format: "json", key: crushMCPKey},
	"cursor":      {path: ".cursor/mcp.json", format: "json", key: "mcpServers", noImport: true},
	"factory":     {path: factoryMCPFile, format: "json", key: factoryMCPKey},
	"gemini":      {path: ".gemini/settings.json", format: "json", key: "mcpServers"},
	"grok":        {format: "toml", key: "mcp_servers"},
	"junie":       {path: ".junie/mcp/mcp.json", format: "json", key: "mcpServers"},
	"kimi":        {path: ".kimi-code/mcp.json", format: "json", key: "mcpServers"},
	"kiro":        {path: kiroMCPFile, format: "json", key: kiroMCPKey},
	"opencode":    {path: opencodeMCPFile, format: "opencode"},
	"pi":          {path: ".pi/mcp.json", format: "json", key: "mcpServers"},
	"qoder":       {path: qoderSettingsFile, format: "json", key: "mcpServers"},
	"qwen":        {path: ".qwen/settings.json", format: "json", key: "mcpServers"},
	"trae":        {path: traeMCPFile, format: "json", key: traeMCPKey},
	"vibe":        {path: ".vibe/config.toml", format: "toml-array", key: "mcp_servers"},
	"warp":        {path: warpMCPFile, format: "json", key: warpMCPKey},
	"windsurf":    {path: windsurfMCPFile, format: "json", key: windsurfMCPKey},
	"zed":         {path: zedSettings, format: "json", key: zedMCPKey},
}

// nativeMCPTransports are the transport names a server entry may give
// that the overview shows as they are.
var nativeMCPTransports = []string{"stdio", "http", "sse", "streamable-http"}

// nativeMCPServers lists the MCP servers the targets' own config files
// define: each target's user file and, unless global, its project file
// under root and Claude Code's local servers for root. It reads names,
// paths, and transport only, so no value from a server, such as an env
// entry, a header, or a URL, leaves this function. A server a global
// sync wrote, as state/global.json under source records, is left out. A
// file that is missing or does not parse gives no servers.
func nativeMCPServers(home, root, source string, cfg *config.Config, global bool) []overview.NativeMCP {
	if cfg == nil {
		return nil
	}
	var out []overview.NativeMCP
	if !global {
		for _, target := range cfg.Targets {
			f, ok := projectNativeMCP[target]
			if !ok {
				continue
			}
			path := projectNativeMCPPath(target, f, cfg)
			if path == "" {
				continue
			}
			importCmd := "agnostic-ai import " + target
			if f.noImport {
				importCmd = ""
			}
			servers := readNativeMCPServers(filepath.Join(root, path), f.format, f.key)
			out = appendNativeMCPs(out, servers, target, filepath.ToSlash(path), overview.NativeScopeProject, importCmd, nil)
		}
		if slices.Contains(cfg.Targets, "claude") {
			path := globalTargets["claude"].mcpPath(home)
			out = appendNativeMCPs(out, claudeLocalMCPServers(path, root), "claude", nativeHomePath(home, path), overview.NativeScopeProject, "", nil)
		}
	}
	var owned map[string]map[string]any
	if source != "" {
		if state, err := loadGlobalState(filepath.Join(source, "state", "global.json")); err == nil {
			owned = state.MCP
		}
	}
	for _, target := range cfg.Targets {
		g, ok := globalTargets[target]
		if !ok || g.mcp.path == "" || g.mcp.retired != "" || g.rootError(target) != nil {
			continue
		}
		path := g.mcpPath(home)
		servers := readNativeMCPServers(path, g.mcp.format, g.mcp.key)
		out = appendNativeMCPs(out, servers, target, nativeHomePath(home, path), overview.NativeScopeUser, "agnostic-ai import --global "+target, owned[target])
	}
	return out
}

// projectNativeMCPPath is the project MCP file the target's adapter
// declares, or f's default.
func projectNativeMCPPath(target string, f nativeMCPFile, cfg *config.Config) string {
	for _, a := range adapters.NativeArtifactsFor(target, cfg) {
		if a.Label == "MCP servers" && a.Location != "" {
			return a.Location
		}
	}
	if target == "grok" {
		return grok.OutputPaths(cfg)["MCP_FILE"]
	}
	return f.path
}

func appendNativeMCPs(out []overview.NativeMCP, servers map[string]map[string]any, target, path, scope, importCmd string, owned map[string]any) []overview.NativeMCP {
	names := make([]string, 0, len(servers))
	for name := range servers {
		if _, mine := owned[name]; !mine {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		out = append(out, overview.NativeMCP{
			Name:      name,
			Target:    target,
			Path:      path,
			Scope:     scope,
			Transport: nativeMCPTransport(servers[name]),
			Import:    importCmd,
		})
	}
	return out
}

// nativeMCPTransport names how a server runs from the keys its entry
// has: stdio for a command, a transport type the entry names when it is
// a known one, remote for a URL, and empty otherwise. It never returns
// text it did not check.
func nativeMCPTransport(server map[string]any) string {
	if _, ok := server["command"]; ok {
		return "stdio"
	}
	for _, key := range []string{"type", "transport"} {
		if v, ok := server[key].(string); ok {
			if v = strings.ToLower(v); slices.Contains(nativeMCPTransports, v) {
				return v
			}
		}
	}
	for _, key := range []string{"url", "httpUrl", "serverUrl"} {
		if _, ok := server[key]; ok {
			return "remote"
		}
	}
	return ""
}

// nativeHomePath shows a path under home as ~/rest.
func nativeHomePath(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && filepath.IsLocal(rel) {
		return "~/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// readNativeMCPServers returns the server entries in path by name.
func readNativeMCPServers(path, format, key string) map[string]map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc map[string]any
	switch format {
	case "json", "opencode":
		data, _ = adapters.StripJSONC(data)
		if json.Unmarshal(data, &doc) != nil {
			return nil
		}
	case "toml", "toml-array":
		if _, err := toml.Decode(string(data), &doc); err != nil {
			return nil
		}
	default:
		return nil
	}
	switch format {
	case "opencode":
		return opencodeNativeMCPServers(doc)
	case "toml-array":
		return namedNativeMCPServers(doc[key])
	}
	return nativeMCPObjects(doc[key])
}

// nativeMCPObjects keeps the entries of a server map that are objects.
func nativeMCPObjects(value any) map[string]map[string]any {
	servers, _ := value.(map[string]any)
	out := map[string]map[string]any{}
	for name, raw := range servers {
		if server, ok := raw.(map[string]any); ok {
			out[name] = server
		}
	}
	return out
}

// namedNativeMCPServers reads an array of tables, each with a name.
func namedNativeMCPServers(value any) map[string]map[string]any {
	out := map[string]map[string]any{}
	add := func(server map[string]any) {
		if name, ok := server["name"].(string); ok && name != "" {
			out[name] = server
		}
	}
	switch records := value.(type) {
	case []map[string]any:
		for _, r := range records {
			add(r)
		}
	case []any:
		for _, r := range records {
			if server, ok := r.(map[string]any); ok {
				add(server)
			}
		}
	}
	return out
}

// opencodeNativeMCPServers reads OpenCode's mcp block, where servers sit
// either directly under mcp or under mcp.servers.
func opencodeNativeMCPServers(doc map[string]any) map[string]map[string]any {
	mcp, _ := doc[opencodeMCPKey].(map[string]any)
	if servers, ok := mcp["servers"].(map[string]any); ok && !opencodeV1MCPEntry(servers) {
		return nativeMCPObjects(servers)
	}
	return nativeMCPObjects(mcp)
}

// claudeLocalMCPServers reads the servers Claude Code keeps for root at
// local scope, under projects.<root>.mcpServers in ~/.claude.json.
func claudeLocalMCPServers(path, root string) map[string]map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		Projects map[string]struct {
			MCPServers map[string]any `json:"mcpServers"`
		} `json:"projects"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.Projects) == 0 {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	resolvedRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolvedRoot = abs
	}
	out := map[string]map[string]any{}
	for key, project := range doc.Projects {
		dir := filepath.Clean(filepath.FromSlash(key))
		if dir != abs && dir != resolvedRoot {
			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil || resolved != resolvedRoot {
				continue
			}
		}
		maps.Copy(out, nativeMCPObjects(project.MCPServers))
	}
	return out
}

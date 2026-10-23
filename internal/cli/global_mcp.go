package cli

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalMCPFile is the user file a target reads MCP servers from, and
// the map or table that holds them.
type globalMCPFile struct {
	path   string
	legacy []string
	format string
	// key is the JSON map (mcpServers) or TOML parent table
	// (mcp_servers) holding one entry per server name.
	key string
	// rootFile names the file inside the target's rootEnv directory
	// when that variable is set, for a file that sits beside the
	// default root rather than in it: Claude keeps ~/.claude.json, and
	// $CLAUDE_CONFIG_DIR/.claude.json when the variable is set.
	rootFile string
	// private marks a file created at 0600, such as ~/.claude.json,
	// which holds account state the tool itself keeps private.
	private bool
}

// fileMode is the mode a sync gives the file when it creates it.
func (f globalMCPFile) fileMode() fs.FileMode {
	if f.private {
		return 0o600
	}
	return 0o644
}

// mcpPath resolves the target's user MCP file.
func (g globalTarget) mcpPath(home string) string {
	if g.mcp.rootFile != "" && g.rootEnv != "" {
		if root := os.Getenv(g.rootEnv); root != "" {
			return filepath.Join(root, g.mcp.rootFile)
		}
	}
	preferred := g.path(home, g.mcp.path)
	for _, candidate := range append([]string{g.mcp.path}, g.mcp.legacy...) {
		path := g.path(home, candidate)
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return path
		}
	}
	return preferred
}

// warnUnsupportedGlobalMCP names the MCP specs a target in the run
// cannot take at user level.
func warnUnsupportedGlobalMCP(warn io.Writer, targets []string, mcps []spec.Entry) ([]dropRecord, error) {
	warnings := []dropRecord{}
	for _, target := range targets {
		if globalTargets[target].mcp.path != "" {
			continue
		}
		var names []string
		for _, m := range mcps {
			if m.EmitsTo(target) {
				names = append(names, m.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		message := "global MCP servers are unsupported; skipping " + strings.Join(names, ", ")
		if _, err := fmt.Fprintf(warn, "warning: %s: %s\n", target, message); err != nil {
			return nil, fmt.Errorf("write global MCP warning: %w", err)
		}
		warnings = append(warnings, dropRecord{Target: target, Kind: string(spec.KindMCP), Count: len(names), Message: message})
	}
	return warnings, nil
}

// mergeGlobalMCP plans the MCP servers for one target's user file, one
// whole server record per name, with the ownership rules of settings
// keys: an equal hand-written server is adopted, a different one is a
// conflict, and a recorded server the specs dropped is removed.
func mergeGlobalMCP(path string, f globalMCPFile, base []byte, target string, mcps []spec.Entry, previous map[string]any) (globalSettingsMerge, error) {
	if target == "codebuddy" {
		if err := codebuddy.ValidateMCP((spec.Bundle{MCPs: mcps}).For(target).MCPs); err != nil {
			return globalSettingsMerge{}, err
		}
	}
	sources := map[string]string{}
	for _, m := range mcps {
		sources[m.Name] = m.Path
	}
	if f.format == "toml" {
		tables, _ := adapters.UserMCPServerTables(target, mcps)
		return mergeGlobalMCPTables(path, f.key, base, tables, sources, previous)
	}
	servers, _, err := adapters.CheckedUserMCPServers(target, mcps)
	if err != nil {
		return globalSettingsMerge{}, fmt.Errorf("render %s user MCP servers: %w", target, err)
	}
	current := map[string]any{}
	var comparison []globalSettingsComparison
	if target == "pi" {
		data := base
		if data == nil {
			read, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return globalSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
			}
			data = read
		}
		if data == nil {
			data = []byte{}
		}
		values, err := settingsValues(path, "json", data)
		if err != nil {
			return globalSettingsMerge{}, err
		}
		current, err = piMCPMap(path, data, f.key)
		if err != nil {
			return globalSettingsMerge{}, err
		}
		values[f.key] = current
		comparison = []globalSettingsComparison{{current: values, equal: func(a, b any) bool { return sameMCPServer("pi", a, b) }}}
		base = data
	} else {
		if data := base; data != nil || fileExists(path) {
			if data == nil {
				var err error
				if data, err = os.ReadFile(path); err != nil {
					return globalSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
				}
			}
			values, err := settingsValues(path, "json", data)
			if err != nil {
				return globalSettingsMerge{}, err
			}
			current, _ = values[f.key].(map[string]any)
		}
	}
	var want []globalSetting
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		if strings.Contains(name, ".") {
			adapters.NoteSettingsFieldNoOp(target, "mcp "+name, 1, "a server name with a dot cannot be a key of the user MCP map here; rename the spec")
			continue
		}
		value := servers[name]
		// A hand-written server that means the same is kept as written.
		if have, ok := current[name]; ok && sameMCPServer(target, have, value) {
			value = have
		}
		want = append(want, globalSetting{target: target, key: f.key + "." + name, value: value, field: "mcp", source: sources[name]})
	}
	return mergeGlobalSettingsKeeping(path, "json", base, want, previous, nil, comparison...)
}

// mergeGlobalMCPTables is mergeGlobalMCP for TOML tables under parent.
func mergeGlobalMCPTables(path, parent string, base []byte, tables, sources map[string]string, previous map[string]any) (globalSettingsMerge, error) {
	data := base
	if data == nil {
		read, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			if len(tables) == 0 {
				return globalSettingsMerge{}, nil
			}
		case err != nil:
			return globalSettingsMerge{}, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
	}
	current, err := tomlTableValues(path, parent, data)
	if err != nil {
		return globalSettingsMerge{}, err
	}
	m := globalSettingsMerge{owned: map[string]any{}}
	set := map[string]string{}
	var order, remove []string
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		desired, err := tomlTableValues(path, parent, []byte(tables[name]))
		if err != nil {
			return globalSettingsMerge{}, err
		}
		value := jsonRoundTrip(desired[name])
		have, present := current[name]
		recorded, owned := previous[name]
		key := parent + "." + name
		switch {
		case !present:
			m.changes = append(m.changes, "add ["+key+"]")
			set[name], order = tables[name], append(order, name)
		case sameSetting(have, value):
			if !owned || !sameSetting(recorded, value) {
				m.adopted = append(m.adopted, "["+key+"]")
			}
		case owned && sameSetting(have, recorded):
			m.changes = append(m.changes, "replace ["+key+"]")
			set[name], order = tables[name], append(order, name)
		default:
			m.changes = append(m.changes, "overwrite ["+key+"]")
			m.conflicts = append(m.conflicts, fmt.Sprintf("[%s] was written outside agnostic-ai and differs from %s; remove it or make the spec match", key, sources[name]))
			set[name], order = tables[name], append(order, name)
		}
		m.owned[name] = value
	}
	for _, name := range slices.Sorted(maps.Keys(previous)) {
		if _, ok := tables[name]; ok {
			continue
		}
		if have, ok := current[name]; ok && sameSetting(have, previous[name]) {
			m.changes = append(m.changes, "remove ["+parent+"."+name+"]")
			remove = append(remove, name)
		}
	}
	if len(order) == 0 && len(remove) == 0 {
		return m, nil
	}
	if m.data, err = editTOMLTables(path, data, parent, order, set, remove); err != nil {
		return m, err
	}
	// A server written as dotted keys or an inline table has no header
	// block to replace, so the edit would define it twice.
	if _, err := toml.Decode(string(m.data), new(map[string]any)); err != nil {
		return m, fmt.Errorf("%s: an MCP server is defined outside its own [%s.<name>] table; move it into one, then sync: %w", path, parent, err)
	}
	if m.data == nil {
		m.data = []byte{}
	}
	return m, nil
}

// tomlTableValues decodes the tables under parent, by name.
func tomlTableValues(path, parent string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	tables, _ := doc[parent].(map[string]any)
	out := map[string]any{}
	for name, value := range tables {
		out[name] = jsonRoundTrip(value)
	}
	return out, nil
}

// sameMCPServer reports whether two user MCP entries of target mean the
// same server: equal once each is spelled the canonical way, so a
// hand-written entry that leaves out an implied `type`, or Copilot's
// default `tools: ["*"]`, counts as what sync would write.
func sameMCPServer(target string, a, b any) bool {
	return sameSetting(canonicalMCPServer(target, a), canonicalMCPServer(target, b))
}

func canonicalMCPServer(target string, v any) any {
	var exposure any
	if target == "pi" {
		if server, ok := v.(map[string]any); ok {
			exposure = canonicalPiToolExposure(server["toolExposure"])
		}
	}
	server, ok := jsonRoundTrip(v).(map[string]any)
	if !ok {
		return v
	}
	out := maps.Clone(server)
	if target == "pi" {
		delete(out, "toolExposure")
		if out["exposure"] == "codemode" || out["exposure"] == "codemode-deferred" {
			delete(out, "exposure")
		}
		if exposure != nil {
			out["toolExposure"] = exposure
		}
		if out["enabled"] == true {
			delete(out, "enabled")
		}
		if out["type"] == "streamable-http" {
			out["type"] = "http"
		}
	}
	if target == "antigravity" {
		if disabled, ok := out["disabled"].(bool); ok && !disabled {
			delete(out, "disabled")
		}
	}
	// An empty list or map, such as args: [], says nothing.
	for key, value := range out {
		switch v := value.(type) {
		case []any:
			if len(v) == 0 {
				delete(out, key)
			}
		case map[string]any:
			if len(v) == 0 {
				delete(out, key)
			}
		}
	}
	if target == "openhands" {
		return canonicalOpenHandsServer(out)
	}
	if target == "codebuddy" {
		if kind, err := codebuddy.MCPType(out); err == nil {
			out["type"] = kind
		}
		return out
	}
	_, hasCommand := out["command"]
	_, hasURL := out["url"]
	switch kind, _ := out["type"].(string); {
	case target == "gemini":
		// httpUrl is streamable HTTP; url is SSE unless type says http.
		if url, ok := out["httpUrl"]; ok {
			delete(out, "httpUrl")
			out["url"], out["type"] = url, "http"
		} else if hasURL && kind == "" {
			out["type"] = "sse"
		}
	case hasCommand && (kind == "stdio" || kind == "local"):
		delete(out, "type")
	case hasURL && kind == "http":
		delete(out, "type")
	}
	if target == "copilot" {
		if tools, ok := out["tools"].([]any); ok && len(tools) == 1 && tools[0] == "*" {
			delete(out, "tools")
		}
	}
	return out
}

// canonicalOpenHandsServer spells an ~/.openhands/mcp.json entry the way
// fastmcp reads it. OpenHands saves the whole file with every model field,
// nulls included, plus `enabled`, whenever `openhands mcp add` or its
// siblings run, so those defaults must not count as a different server.
func canonicalOpenHandsServer(server map[string]any) map[string]any {
	for key, value := range server {
		if value == nil {
			delete(server, key)
		}
	}
	if server["enabled"] == true {
		delete(server, "enabled")
	}
	transport, _ := server["transport"].(string)
	url, hasURL := server["url"].(string)
	switch {
	case !hasURL && transport == "stdio":
		delete(server, "transport")
	case hasURL && transport == "streamable-http":
		server["transport"] = "http"
	case hasURL && transport == "":
		server["transport"] = openHandsURLTransport(url)
	}
	return server
}

// openHandsURLTransport is the transport fastmcp infers for a remote
// server that names none: SSE for a path ending in /sse, else HTTP.
func openHandsURLTransport(url string) string {
	path, _, _ := strings.Cut(url, "?")
	if strings.HasSuffix(strings.TrimRight(path, "/"), "/sse") {
		return "sse"
	}
	return "http"
}

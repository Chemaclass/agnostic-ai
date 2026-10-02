package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// globalImportSettingsSpec is where import --global puts the default
// model and effort it reads.
const globalImportSettingsSpec = "settings/imported.yaml"

// runImportGlobal reads the user settings and MCP files sync --global
// writes and turns them into specs in the home, so a following sync
// adopts them and changes nothing. A spec file that already exists is
// never replaced: an equal one is left as it is, and a different one is
// reported and skipped.
func runImportGlobal(cmd *cobra.Command, args []string, dryRun bool) error {
	home, err := globalUserHome()
	if err != nil {
		return err
	}
	source := globalSourceHome(home)
	targets, err := globalImportTargets(args)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := globalTargets[target].rootError(target); err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp("", "agnostic-ai-import-global-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	// Anything a home spec already provides, shared or local, is left
	// out: importing it again would copy a local secret into the
	// shared home, or add a second spec for one key.
	bundle, err := spec.LoadLayered(globalLayers(source))
	if err != nil {
		return err
	}
	warn := cmd.ErrOrStderr()
	if err := stageGlobalSettings(home, stage, targets, bundle.Settings); err != nil {
		return err
	}
	if err := stageGlobalMCP(home, stage, targets, bundle.MCPs, warn); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var created, kept, differ int
	err = filepath.WalkDir(stage, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		dst := filepath.Join(source, rel)
		existing, readErr := os.ReadFile(dst)
		switch {
		case readErr == nil && bytes.Equal(existing, data):
			kept++
			return nil
		case readErr == nil:
			differ++
			_, err := fmt.Fprintf(warn, "skipped %s: it exists with other content; compare it with what the tool holds, then edit it by hand\n", dst)
			return err
		case !os.IsNotExist(readErr):
			return fmt.Errorf("read %s: %w", dst, readErr)
		}
		created++
		if dryRun {
			_, err := fmt.Fprintf(out, "dry-run: write %s\n", dst)
			return err
		}
		if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", dst, err)
		}
		if err := importWriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		_, err = fmt.Fprintf(out, "wrote %s\n", dst)
		return err
	})
	if err != nil {
		return err
	}
	if created+kept+differ == 0 {
		_, err := fmt.Fprintf(out, "No user settings or MCP servers found for %s.\n", strings.Join(targets, ", "))
		return err
	}
	if dryRun {
		return nil
	}
	_, err = fmt.Fprintf(out, "Imported into %s: %d written, %d already there, %d skipped. Run `agnostic-ai sync --global --check` to confirm nothing would change.\n", source, created, kept, differ)
	return err
}

// globalImportTargets is the named targets, or every target sync
// --global writes user settings or MCP servers for.
func globalImportTargets(args []string) ([]string, error) {
	importable := func(name string) bool {
		g, ok := globalTargets[name]
		return ok && (g.settings.path != "" || g.mcp.path != "")
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "all") {
		var out []string
		for _, name := range globalTargetNames() {
			if importable(name) {
				out = append(out, name)
			}
		}
		return out, nil
	}
	var out []string
	for _, name := range args {
		if !importable(name) {
			var names []string
			for _, n := range globalTargetNames() {
				if importable(n) {
					names = append(names, n)
				}
			}
			return nil, fmt.Errorf("import --global: %q has no user settings or MCP file to read; supported: %s", name, strings.Join(names, ", "))
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// stageGlobalSettings writes one settings spec with each target's
// default model and effort as a per-target map, so sync writes each
// value back to the key it came from.
func stageGlobalSettings(home, stage string, targets []string, have []spec.Entry) error {
	model, effort := map[string]any{}, map[string]any{}
	for _, target := range targets {
		g := globalTargets[target]
		f := g.settings
		if f.path == "" {
			continue
		}
		path := g.path(home, f.path)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		values, err := settingsValues(path, f.format, data)
		if err != nil {
			return err
		}
		if f.model != "" && adapters.SettingsModel(have, target) == "" {
			if v, ok := settingsLookup(values, f.model, f.format); ok {
				if s, _ := v.(string); s != "" {
					model[target] = s
				}
			}
		}
		if f.effort != "" && adapters.SettingsEffort(have, target) == nil {
			if v, ok := settingsLookup(values, f.effort, f.format); ok && f.accepts(v) {
				effort[target] = v
			}
		}
	}
	doc := map[string]any{}
	if len(model) > 0 {
		doc["model"] = model
	}
	if len(effort) > 0 {
		doc["effort"] = effort
	}
	if len(doc) == 0 {
		return nil
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", globalImportSettingsSpec, err)
	}
	dst := filepath.Join(stage, filepath.FromSlash(globalImportSettingsSpec))
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", dst, err)
	}
	return importWriteFile(dst, raw, 0o644)
}

// stageGlobalMCP writes one MCP spec per user server, with each
// target's own importer. A server a home spec already names is left
// out, and so is one whose spec would not render back to the same
// entry, since sync would then report it as a conflict. A name two
// targets define differently keeps the first target's server and
// warns, since one spec reaches both.
func stageGlobalMCP(home, stage string, targets []string, have []spec.Entry, warn io.Writer) error {
	dst := filepath.Join(stage, "mcps")
	// first holds, per lowercased server name, the target that supplied
	// it and its entry; names differing only in case share one file on
	// a case-insensitive disk.
	type supplied struct {
		target, name string
		server       any
	}
	first := map[string]supplied{}
	covered := map[string]bool{}
	for _, m := range have {
		covered[m.Name] = true
	}
	skip := func(target, name, reason string) error {
		_, err := fmt.Fprintf(warn, "warning: %s: skipped MCP server %s: %s\n", target, name, reason)
		return err
	}
	for _, target := range targets {
		g := globalTargets[target]
		if g.mcp.path == "" {
			continue
		}
		path := g.mcpPath(home)
		dir, err := os.MkdirTemp(stage, ".mcp-"+target+"-")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		var native map[string]any
		if g.mcp.format == "toml" {
			_, servers, err := readCodexConfigTOMLFile(path)
			if err != nil {
				return err
			}
			for name := range servers {
				if covered[name] {
					delete(servers, name)
				}
			}
			if _, err := writeMCPSpecs(codexMCPDocs(servers), dir); err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("read %s: %w", path, err)
			}
			if native, err = tomlTableValues(path, g.mcp.key, data); err != nil {
				return err
			}
		} else {
			servers, err := readJSONMapAt(path, g.mcp.key)
			if err != nil {
				return err
			}
			native = map[string]any{}
			for name, server := range servers {
				if covered[name] {
					delete(servers, name)
					continue
				}
				native[name] = jsonRoundTrip(server)
			}
			normalizeImportedMCP(target, servers)
			if _, err := writeMCPSpecs(servers, dir); err != nil {
				return err
			}
		}
		staged, err := spec.LoadLayered([]spec.Layer{{Name: "import", Root: dir, Sources: config.Sources{MCPs: "."}}})
		if err != nil {
			return err
		}
		rendered := map[string]any{}
		if g.mcp.format == "toml" {
			tables, _ := adapters.UserMCPServerTables(target, staged.MCPs)
			for name, table := range tables {
				values, err := tomlTableValues(path, g.mcp.key, []byte(table))
				if err != nil {
					return err
				}
				rendered[name] = values[name]
			}
		} else {
			rendered, _ = adapters.UserMCPServers(target, staged.MCPs)
		}
		var kept []spec.Entry
		for _, m := range staged.MCPs {
			same := sameSetting(rendered[m.Name], native[m.Name])
			if g.mcp.format == "json" {
				same = sameMCPServer(target, rendered[m.Name], native[m.Name])
			}
			if same {
				kept = append(kept, m)
				continue
			}
			if err := os.Remove(m.Path); err != nil {
				return fmt.Errorf("remove %s: %w", m.Path, err)
			}
			if err := skip(target, m.Name, "its spec would not write back the same server; add it to the home by hand"); err != nil {
				return fmt.Errorf("write import warning: %w", err)
			}
		}
		for _, m := range kept {
			key := strings.ToLower(m.Name)
			if prior, ok := first[key]; ok {
				if prior.name != m.Name || !sameSetting(canonicalMCPServer(prior.target, prior.server), canonicalMCPServer(target, native[m.Name])) {
					if _, err := fmt.Fprintf(warn, "warning: %s: %s defines %s differently from %s's %s; kept %s's\n", target, path, m.Name, prior.target, prior.name, prior.target); err != nil {
						return fmt.Errorf("write import warning: %w", err)
					}
				}
				continue
			}
			first[key] = supplied{target: target, name: m.Name, server: native[m.Name]}
			data, err := os.ReadFile(m.Path)
			if err != nil {
				return fmt.Errorf("read %s: %w", m.Path, err)
			}
			out := filepath.Join(dst, filepath.Base(m.Path))
			if err := importMkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create directory for %s: %w", out, err)
			}
			if err := importWriteFile(out, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	return nil
}

// normalizeImportedMCP spells each server's transport the portable
// way, so the spec renders back to the same entry: Gemini's `httpUrl`
// is streamable HTTP and a bare `url` is SSE, Copilot's `local` is the
// stdio default, and OpenHands' `transport` becomes `type` once its
// saved defaults are dropped. target's own environment references read
// back as `${NAME}`.
func normalizeImportedMCP(target string, servers map[string]any) {
	for _, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		adapters.ReadMCPEnvRefs(target, server)
		if target == "copilot" {
			if server["type"] == "local" || server["type"] == "stdio" {
				delete(server, "type")
			}
			// "*" is the default Copilot gets back when a spec sets none.
			if tools, ok := server["tools"].([]any); ok && len(tools) == 1 && tools[0] == "*" {
				delete(server, "tools")
			}
			continue
		}
		if target == "openhands" {
			canonicalOpenHandsServer(server)
			if transport, _ := server["transport"].(string); transport != "" {
				server["type"] = transport
			}
			delete(server, "transport")
			continue
		}
		if target != "gemini" {
			continue
		}
		if url, ok := server["httpUrl"]; ok {
			delete(server, "httpUrl")
			server["url"] = url
			server["type"] = "http"
			continue
		}
		if _, ok := server["url"]; ok && server["type"] == nil {
			server["type"] = "sse"
		}
	}
}

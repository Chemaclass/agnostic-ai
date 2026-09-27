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
	stage, err := os.MkdirTemp("", "agnostic-ai-import-global-*")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	warn := cmd.ErrOrStderr()
	if err := stageGlobalSettings(home, stage, targets); err != nil {
		return err
	}
	if err := stageGlobalMCP(home, stage, targets, warn); err != nil {
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
func stageGlobalSettings(home, stage string, targets []string) error {
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
		if f.model != "" {
			if v, ok := settingsLookup(values, f.model, f.format); ok {
				if s, _ := v.(string); s != "" {
					model[target] = s
				}
			}
		}
		if f.effort != "" {
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
// target's own importer. A name two targets define differently keeps
// the first target's server and warns, since one spec reaches both.
func stageGlobalMCP(home, stage string, targets []string, warn io.Writer) error {
	dst := filepath.Join(stage, "mcps")
	from := map[string]string{}
	for _, target := range targets {
		g := globalTargets[target]
		if g.mcp.path == "" {
			continue
		}
		path := g.path(home, g.mcp.path)
		dir, err := os.MkdirTemp(stage, ".mcp-"+target+"-")
		if err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		if g.mcp.format == "toml" {
			_, servers, err := readCodexConfigTOMLFile(path)
			if err != nil {
				return err
			}
			if _, err := writeCodexMCPs(servers, dir); err != nil {
				return err
			}
		} else {
			servers, err := readJSONMapAt(path, g.mcp.key)
			if err != nil {
				return err
			}
			if target == "gemini" {
				normalizeGeminiMCPTransport(servers)
			}
			if _, err := writeMCPYAMLs(servers, dir); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return fmt.Errorf("read %s: %w", entry.Name(), err)
			}
			out := filepath.Join(dst, entry.Name())
			if first, ok := from[entry.Name()]; ok {
				if existing, err := os.ReadFile(out); err == nil && !bytes.Equal(existing, data) {
					if _, err := fmt.Fprintf(warn, "warning: %s: %s defines %s differently from %s; kept %s's\n", target, path, strings.TrimSuffix(entry.Name(), ".yaml"), first, first); err != nil {
						return fmt.Errorf("write import warning: %w", err)
					}
				}
				continue
			}
			from[entry.Name()] = target
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

// normalizeGeminiMCPTransport spells Gemini's transport the portable
// way: `httpUrl` is streamable HTTP and a bare `url` is SSE, so the
// spec renders back to the same key.
func normalizeGeminiMCPTransport(servers map[string]any) {
	for _, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
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

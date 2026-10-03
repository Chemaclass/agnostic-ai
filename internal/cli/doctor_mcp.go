package cli

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// reportMCPCommandResolution scans every loaded MCP spec and reports
// whether the configured `command:` resolves on PATH. Stdio servers
// fail at IDE-launch time when the binary is missing; surfacing the
// gap during `doctor` saves the user a confusing round-trip.
//
// HTTP/SSE servers (no command, only url) are skipped. The check is
// advisory: missing binaries do not flip doctor's exit code, since
// they reflect environment state, not spec state.
func reportMCPCommandResolution(cmd *cobra.Command) {
	_, b, err := loadProject(".")
	if err != nil {
		return
	}
	if len(b.MCPs) == 0 {
		return
	}
	type result struct {
		name string
		cmd  string
		path string
		err  error
	}
	var results []result
	for _, e := range b.MCPs {
		command, _ := e.Meta["command"].(string)
		if command == "" {
			// HTTP/SSE server (url-only); nothing to resolve.
			continue
		}
		resolved, err := exec.LookPath(command)
		results = append(results, result{name: e.Name, cmd: command, path: resolved, err: err})
	}
	if len(results) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("MCP command resolution:")
	for _, r := range results {
		if r.err == nil {
			cmd.Printf("  ✓ %s (%s) → %s\n", r.name, r.cmd, r.path)
			continue
		}
		cmd.Printf("  ✗ %s (%s) not found on PATH. %s\n", r.name, r.cmd, installHint(r.cmd))
	}
}

// installHint returns a short, well-known install suggestion for the
// MCP command names users hit most often. Anything else gets a
// generic message — no need to maintain an exhaustive table.
func installHint(command string) string {
	switch command {
	case "npx":
		return "Install Node.js (https://nodejs.org or `brew install node`)."
	case "uvx", "uv":
		return "Install uv (`brew install uv` or `curl -LsSf https://astral.sh/uv/install.sh | sh`)."
	case "python", "python3":
		return "Install Python 3 (https://python.org or `brew install python`)."
	case "docker":
		return "Install Docker (https://www.docker.com/products/docker-desktop)."
	}
	return fmt.Sprintf("Install or expose %q on PATH.", command)
}

// reportMCPUnsetEnvRefs lists each `${NAME}` an MCP server reads that
// is unset in this shell, for the configured targets the server emits
// to. Disabled servers count too: several targets cannot write
// `disabled` and start the server anyway. Most tools pass the unexpanded text or an empty value
// to the server, and Factory fails the connection. A reference with a
// default is skipped. The tool may run with a different environment than
// this shell, so the check is advisory and prints names, never values.
func reportMCPUnsetEnvRefs(cmd *cobra.Command) {
	cfg, b, err := loadProject(".")
	if err != nil {
		return
	}
	type result struct {
		name  string
		unset []string
	}
	var results []result
	for _, e := range b.MCPs {
		var names []string
		for _, target := range cfg.Targets {
			if e.EmitsTo(target) {
				names = append(names, mcpEnvRefNames(e.Meta, target)...)
			}
		}
		slices.Sort(names)
		names = slices.Compact(names)
		if len(names) == 0 {
			continue
		}
		var unset []string
		for _, name := range names {
			if _, ok := os.LookupEnv(name); !ok {
				unset = append(unset, name)
			}
		}
		results = append(results, result{name: e.Name, unset: unset})
	}
	if len(results) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("MCP environment references:")
	for _, r := range results {
		if len(r.unset) == 0 {
			cmd.Printf("  ✓ %s\n", r.name)
			continue
		}
		cmd.Printf("  ✗ %s reads %s, unset in this shell. Export it before starting the tool.\n", r.name, strings.Join(r.unset, ", "))
	}
}

// mcpEnvRefNames returns the variables a server reads through `${NAME}`
// without a default, in its top-level fields and in its `x-<target>`
// block. Both count: whether an override replaces a top-level field
// differs by adapter, and a missed variable costs more than an extra
// one. Editor variables such as `${workspaceFolder}` are filled in by
// the tool only in `url` and `args`, and `args` counts only for a stdio
// server.
func mcpEnvRefNames(meta map[string]any, target string) []string {
	var names []string
	add := func(value any, launch bool) {
		s, ok := value.(string)
		if !ok {
			return
		}
		for _, t := range spec.EnvRefTokens(s) {
			if t.Known() && !t.HasDefault && !(launch && t.EditorVariable()) {
				names = append(names, t.Name)
			}
		}
	}
	blocks := []map[string]any{meta}
	if override, ok := meta["x-"+target].(map[string]any); ok {
		blocks = append(blocks, override)
	}
	stdio := false
	for _, block := range blocks {
		if command, _ := block["command"].(string); command != "" {
			stdio = true
		}
	}
	for _, block := range blocks {
		credentials := []any{block["env"], block["headers"]}
		if options, ok := block["requestOptions"].(map[string]any); ok {
			credentials = append(credentials, options["headers"])
		}
		for _, c := range credentials {
			values, _ := c.(map[string]any)
			for _, v := range values {
				add(v, false)
			}
		}
		add(block["url"], true)
		if stdio {
			args, _ := block["args"].([]any)
			for _, v := range args {
				add(v, true)
			}
		}
	}
	return names
}

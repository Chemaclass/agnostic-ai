package cli

import (
	"fmt"
	"os/exec"

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

// touchedKinds is unused but kept here to assert the intent of the
// MCP-only check; future extensions (e.g. validating the args
// pointer) should reuse the same single-pass shape.
var _ = touchedKinds

func touchedKinds() []spec.Kind { return []spec.Kind{spec.KindMCP} }

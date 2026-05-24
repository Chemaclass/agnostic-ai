package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// hookScriptVariant records one body of a same-named hook script captured
// for a specific source tool under `.agnostic-ai/scripts/<tool>/<basename>`.
type hookScriptVariant struct {
	tool string
	path string
	size int64
	sha  string
}

// reportDivergentHookScripts walks `.agnostic-ai/scripts/<tool>/` and
// flags any basename that exists under two or more tools with different
// SHA-256 bodies. Same-content duplicates are silent. Drift is not
// auto-fixable; reconciliation needs human judgement about which body
// wins.
func reportDivergentHookScripts(cmd *cobra.Command, root string) (bool, error) {
	cmd.Println()
	cmd.Println("Hook script divergence:")

	scriptsRoot := filepath.Join(root, agnosticScriptsDir)
	variants, err := collectHookScriptVariants(scriptsRoot)
	if err != nil {
		return false, err
	}
	if len(variants) == 0 {
		cmd.Println("  (no per-tool hook scripts captured)")
		return false, nil
	}

	diverged := false
	basenames := make([]string, 0, len(variants))
	for b := range variants {
		basenames = append(basenames, b)
	}
	sort.Strings(basenames)
	for _, basename := range basenames {
		vs := variants[basename]
		if len(vs) < 2 || allSameHash(vs) {
			continue
		}
		diverged = true
		tools := make([]string, len(vs))
		for i, v := range vs {
			tools[i] = v.tool
		}
		cmd.Printf("  ✗ divergent hook script: scripts/{%s}/%s\n",
			joinSorted(tools), basename)
		for _, v := range vs {
			cmd.Printf("      %-7s variant: %d bytes  sha256: %s\n",
				v.tool, v.size, v.sha[:12])
		}
		cmd.Println("      consolidate by moving the agreed body to")
		cmd.Printf("      %s/%s and deleting the per-tool copies,\n",
			agnosticScriptsDir, basename)
		cmd.Println("      or document the intentional divergence in a sibling note.")
	}
	if !diverged {
		cmd.Println("  ✓ no divergent hook scripts")
	}
	return diverged, nil
}

// collectHookScriptVariants returns basename → variants list for every
// per-tool hook script captured under root. Returns an empty map (no
// error) when root does not exist.
func collectHookScriptVariants(root string) (map[string][]hookScriptVariant, error) {
	out := map[string][]hookScriptVariant{}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		toolDir := filepath.Join(root, e.Name())
		files, err := os.ReadDir(toolDir)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", toolDir, err)
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			path := filepath.Join(toolDir, f.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			sum := sha256.Sum256(data)
			info, err := f.Info()
			if err != nil {
				return nil, fmt.Errorf("stat %s: %w", path, err)
			}
			out[f.Name()] = append(out[f.Name()], hookScriptVariant{
				tool: e.Name(),
				path: path,
				size: info.Size(),
				sha:  hex.EncodeToString(sum[:]),
			})
		}
	}
	// Sort each variant slice by tool for stable output.
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].tool < out[k][j].tool })
	}
	return out, nil
}

// allSameHash reports whether every variant has the same body sha.
func allSameHash(vs []hookScriptVariant) bool {
	for i := 1; i < len(vs); i++ {
		if vs[i].sha != vs[0].sha {
			return false
		}
	}
	return true
}

// joinSorted returns the tool names comma-joined in alphabetical order,
// used inside the `scripts/{<tools>}/<basename>` finding line.
func joinSorted(tools []string) string {
	cp := append([]string(nil), tools...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

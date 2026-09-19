package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// traeHooksFile is Trae's project hook tier: "Project Hook |
// `$PROJECT_FOLDER/.trae/hooks.json` | Applies only to the current
// project or workspace" (docs.trae.ai/ide/hook-configuration-reference).
// The global tier (`~/.trae/hooks.json`) is outside a project and not
// read here.
const traeHooksFile = ".trae/hooks.json"

// traeHookGroup mirrors one group inside `hooks.<EventName>`: the shared
// `{matcher, hooks: [...]}` shape claude, codex, openhands, and qoder
// already write, plus Trae's own group-level `loop_limit`.
type traeHookGroup struct {
	Matcher   string             `json:"matcher"`
	LoopLimit int                `json:"loop_limit"`
	Hooks     []groupedHookEntry `json:"hooks"`
}

// importTraeHooks reads `.trae/hooks.json` and writes one yaml per
// matcher group into dstDir. No-op when the file or its `hooks` key is
// absent.
//
// The integer `version` envelope is read past rather than carried onto
// the specs: the vendor says "The default value is 1, and currently only
// 1 is supported", so the emit side pegs it and a spec field would have
// nothing to vary.
//
// `loop_limit` comes back as a top-level spec key, the same spelling the
// emit side reads. A value of 0 is not written: Trae treats an absent
// key and a value "less than or equal to 0" identically, both falling
// back to its default of 5.
func importTraeHooks(root, dstDir string) (int, error) {
	src := filepath.Join(root, filepath.FromSlash(traeHooksFile))
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var doc struct {
		Hooks map[string][]traeHookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	count := 0
	for _, event := range sortedHookEvents(doc.Hooks) {
		for _, g := range doc.Hooks[event] {
			var extra map[string]any
			if g.LoopLimit > 0 {
				extra = map[string]any{"loop_limit": g.LoopLimit}
			}
			n, err := writeGroupedHookSpec(dstDir, "trae", event, g.Matcher, g.Hooks, extra)
			if err != nil {
				return count, err
			}
			count += n
		}
	}
	return count, nil
}

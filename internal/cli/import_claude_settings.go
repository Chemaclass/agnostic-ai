package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	// claudeOverlayFile is the captured non-hooks portion of
	// `.claude/settings.json`. The emitter uses it as the base document
	// when writing settings.json, layering the spec-derived `hooks` key
	// on top.
	claudeOverlayFile = "claude.settings.json"

	// claudeHookOrderFile is a sidecar capturing where `hooks` sat in
	// the source settings.json and the order of its event keys
	// (`PreToolUse`, `PostToolUse`, ...). The claude adapter reads it on
	// emit so the user's authored order survives a round-trip instead of
	// being normalized to the canonical lifecycle order.
	claudeHookOrderFile = "claude.settings.hook-events.json"

	// claudeSettingsSpec is the settings spec a promoted effortLevel lands in.
	claudeSettingsSpec = "claude.yaml"

	// claudePermissionsSpec is the settings spec the imported permission
	// lists land in.
	claudePermissionsSpec = "permissions-claude"
)

// claudeSettingsImport reports what importClaudeSettingsOverlay wrote.
type claudeSettingsImport struct {
	seeded           bool
	effortMoved      bool
	permissionsMoved bool
}

// claudeOverlayDir is an alias for the shared overlay directory.
// Kept as a separate identifier so call sites read claude-scoped.
const claudeOverlayDir = agnosticOverlayDir

// claudeOverlayPath returns the project-relative path to the captured
// Claude settings overlay.
func claudeOverlayPath(root string) string {
	return filepath.Join(root, claudeOverlayDir, claudeOverlayFile)
}

// importClaudeSettingsOverlay writes no overlay when settings.json is
// missing or holds only hooks, so a fresh project gets no empty file.
func importClaudeSettingsOverlay(root, settingsDir string) (claudeSettingsImport, error) {
	var out claudeSettingsImport
	src := filepath.Join(root, claudeDir, "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read %s: %w", src, err)
	}
	doc := adapters.NewOrderedJSON()
	if err := json.Unmarshal(data, doc); err != nil {
		return out, fmt.Errorf("parse %s: %w", src, err)
	}
	removedPolicy, err := excludeGeneratedClaudeRejections(root, doc)
	if err != nil {
		return out, err
	}
	if out.effortMoved, err = moveClaudeEffortLevel(doc, settingsDir); err != nil {
		return out, err
	}
	if out.permissionsMoved, err = moveClaudePermissions(root, doc, settingsDir); err != nil {
		return out, err
	}
	if err := excludeClaudeHookTargetEnv(doc); err != nil {
		return out, err
	}
	if rawHooks, ok := doc.Get("hooks"); ok {
		if err := captureClaudeHookOrder(root, doc.Keys(), rawHooks); err != nil {
			return out, err
		}
		doc.Delete("hooks")
	}
	if !removedPolicy && doc.Len() == 0 {
		return out, nil
	}
	indent := adapters.DetectJSONIndent(data)
	raw, err := adapters.MarshalJSONIndentWith(doc, indent)
	if err != nil {
		return out, fmt.Errorf("marshal overlay: %w", err)
	}
	dst := claudeOverlayPath(root)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return out, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := importWriteFile(dst, append(raw, '\n'), 0o644); err != nil {
		return out, fmt.Errorf("write %s: %w", dst, err)
	}
	out.seeded = true
	return out, nil
}

// claudePermissionLists are the `permissions` keys a settings spec
// carries. Sibling keys such as defaultMode have no portable form and
// stay in the overlay.
var claudePermissionLists = []string{"allow", "ask", "deny"}

// moveClaudePermissions moves the permission lists out of doc into a
// portable settings spec, so lint and the other targets' coverage notes
// see them. A rule another settings spec or the config already gives
// Claude is dropped instead: sync wrote it into settings.json, and a
// portable copy would widen a rule pinned to Claude to every target.
func moveClaudePermissions(root string, doc *adapters.OrderedJSON, settingsDir string) (bool, error) {
	raw, ok := doc.Get("permissions")
	if !ok {
		return false, nil
	}
	perms := adapters.NewOrderedJSON()
	if err := json.Unmarshal(raw, perms); err != nil {
		return false, nil
	}
	owned, err := claudeOwnedPermissionRules(root, settingsDir)
	if err != nil {
		return false, err
	}
	lists := map[string][]string{}
	for _, list := range claudePermissionLists {
		rawList, ok := perms.Get(list)
		if !ok {
			continue
		}
		var rules []string
		if err := json.Unmarshal(rawList, &rules); err != nil {
			return false, fmt.Errorf("parse Claude settings permissions.%s: %w", list, err)
		}
		for _, rule := range rules {
			if rule != "" && !owned[list][rule] {
				lists[list] = appendUnique(lists[list], rule)
			}
		}
		perms.Delete(list)
	}
	n, err := writePermissionsSpec(settingsDir, claudePermissionsSpec, lists)
	if err != nil {
		return false, err
	}
	if perms.Len() == 0 {
		doc.Delete("permissions")
	} else if err := doc.Set("permissions", perms); err != nil {
		return false, fmt.Errorf("marshal Claude settings permissions: %w", err)
	}
	return n > 0, nil
}

// claudeOwnedPermissionRules returns, per list, the rules sync already
// writes into settings.json from somewhere other than the spec this
// import owns: other settings specs that reach Claude, portable, protected paths, or under x-claude, and
// outputs.claude.settings.permissions.
func claudeOwnedPermissionRules(root, settingsDir string) (map[string]map[string]bool, error) {
	owned := map[string]map[string]bool{}
	add := func(list string, rules []string) {
		for _, rule := range rules {
			if owned[list] == nil {
				owned[list] = map[string]bool{}
			}
			owned[list][rule] = true
		}
	}
	others, err := otherSettingsSpecs(settingsDir, filepath.Join(settingsDir, claudePermissionsSpec+".yaml"))
	if err != nil {
		return nil, err
	}
	for _, entry := range others {
		if !entry.EmitsTo("claude") {
			continue
		}
		layers := []any{entry.Meta["permissions"]}
		if hatch, ok := entry.Meta["x-claude"].(map[string]any); ok {
			layers = append(layers, hatch["permissions"])
		}
		for _, layer := range layers {
			perms, _ := layer.(map[string]any)
			for _, list := range claudePermissionLists {
				add(list, stringSliceFromAny(perms[list]))
			}
		}
		groups, _ := spec.ProtectedPaths([]spec.Entry{entry})
		for _, group := range groups {
			for _, path := range group.Paths {
				add(group.Decision, []string{spec.ProtectEditRule(path)})
			}
		}
	}
	for _, name := range adapters.ClaudeOwnedPermissionsFiles {
		path := filepath.Join(root, claudeDir, name)
		recorded, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var lists map[string][]string
		if err := json.Unmarshal(recorded, &lists); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for list, rules := range lists {
			add(list, rules)
		}
	}
	if cfg, err := config.Load(root); err == nil &&
		cfg.Outputs["claude"].Settings != nil && cfg.Outputs["claude"].Settings.Permissions != nil {
		p := cfg.Outputs["claude"].Settings.Permissions
		add("allow", p.Allow)
		add("ask", p.Ask)
		add("deny", p.Deny)
	}
	return owned, nil
}

// moveClaudeEffortLevel moves effortLevel out of doc into a settings
// spec whenever a settings spec decides what sync writes for it.
func moveClaudeEffortLevel(doc *adapters.OrderedJSON, settingsDir string) (bool, error) {
	raw, ok := doc.Get("effortLevel")
	if !ok {
		return false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, nil
	}
	plan, level, err := planSettingsEffort("claude", value, settingsDir, claudeSettingsSpec)
	if err != nil || plan == effortStays {
		return false, err
	}
	if err := writeSettingsSpec(settingsDir, claudeSettingsSpec, settingsEffortSpec(plan, "claude", "effortLevel", level)); err != nil {
		return false, err
	}
	doc.Delete("effortLevel")
	return true, nil
}

// Generated rejections are reconstructed from MCP specs. Capturing them as
// manual overlay policy would resurrect them after a server is re-enabled.
func excludeGeneratedClaudeRejections(root string, doc *adapters.OrderedJSON) (bool, error) {
	path := filepath.Join(root, claudeDir, ".agnostic-ai-mcp-disabled.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	var owned []string
	if err := json.Unmarshal(data, &owned); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	const key = "disabledMcpjsonServers"
	raw, ok := doc.Get(key)
	if !ok || len(owned) == 0 {
		return false, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return false, fmt.Errorf("parse Claude settings %s: %w", key, err)
	}
	before := len(names)
	names = slices.DeleteFunc(names, func(name string) bool { return slices.Contains(owned, name) })
	if len(names) == 0 {
		doc.Delete(key)
	} else if err := doc.Set(key, names); err != nil {
		return false, fmt.Errorf("marshal Claude settings %s: %w", key, err)
	}
	return len(names) != before, nil
}

// excludeClaudeHookTargetEnv drops the target variable sync writes into
// `env` while command hooks exist. Captured into the overlay, it would
// outlive the last hook.
func excludeClaudeHookTargetEnv(doc *adapters.OrderedJSON) error {
	if err := adapters.SetHookTargetEnv(doc, "claude", false); err != nil {
		return fmt.Errorf("claude settings: %w", err)
	}
	return nil
}

// claudeOverlayRelPath returns the overlay path relative to the project
// root, suitable for printing in import summary lines.
func claudeOverlayRelPath() string {
	return filepath.Join(claudeOverlayDir, claudeOverlayFile)
}

// captureClaudeHookOrder writes the key `hooks` sits next to among
// keys and the event keys of rawHooks, in source order, to
// `.agnostic-ai/overlays/claude.settings.hook-events.json`. The claude
// adapter reads that file on emit, so `hooks` keeps its place and a
// user authored as `PostToolUse` first stays that way across a
// round-trip.
func captureClaudeHookOrder(root string, keys []string, rawHooks json.RawMessage) error {
	if len(rawHooks) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(rawHooks))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		// non-object hook value — nothing to record.
		return nil
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil
	}
	var events []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		name, _ := key.(string)
		var groups json.RawMessage
		if err := dec.Decode(&groups); err != nil {
			return nil
		}
		if name != "" && !importLocal.feedsOnlyLocalHooks(name, groups) {
			events = append(events, name)
		}
	}
	if len(events) == 0 {
		return nil
	}
	dst := filepath.Join(root, claudeOverlayDir, claudeHookOrderFile)
	if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	body, err := json.MarshalIndent(claudehooks.OrderOf(keys, events), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hook order: %w", err)
	}
	if err := importWriteFile(dst, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

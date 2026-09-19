package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// kiloToolToPortable reverses the vocabulary the kilo adapter emits
// into `kilo.jsonc`'s `permission` map. Kilo's own keys outside this
// table (`external_directory`, `lsp`, `doom_loop`, `agent_manager`,
// and any namespaced `{server}_{tool}` MCP key) have no portable
// spelling and are left where they are rather than guessed at: the
// MCP form especially, since `github_list_issues` gives no way to tell
// where the server name ends and the tool name starts.
var kiloToolToPortable = map[string]string{
	"read":      "Read",
	"edit":      "Edit",
	"write":     "Write",
	"bash":      "Bash",
	"glob":      "Glob",
	"grep":      "Grep",
	"task":      "Task",
	"skill":     "Skill",
	"webfetch":  "WebFetch",
	"websearch": "WebSearch",
	"todoread":  "TodoRead",
	"todowrite": "TodoWrite",
}

// importKiloPermissions reads the committed `permission` map from
// `kilo.jsonc` and writes it back as one Settings spec. Kilo documents
// that file as the shared project config: "Permissions are configured
// under the `permission` key in `kilo.jsonc`"
// (kilo.ai/docs/getting-started/settings/auto-approving-actions), so a
// team that wrote one before adopting agnostic-ai keeps it (#890).
func importKiloPermissions(root, dstDir string) (int, error) {
	src := filepath.Join(root, "kilo.jsonc")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	data, _ = adapters.StripJSONC(data)
	var doc struct {
		Permission map[string]json.RawMessage `json:"permission"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	lists := map[string][]string{}
	for _, tool := range sortedKiloTools(doc.Permission) {
		scope, known := kiloToolToPortable[tool]
		if !known {
			continue
		}
		for pattern, action := range kiloToolRules(doc.Permission[tool]) {
			if !isPortablePermissionList(action) {
				continue
			}
			lists[action] = appendUnique(lists[action], portableKiloRule(scope, pattern))
		}
	}
	for list := range lists {
		sort.Strings(lists[list])
	}
	return writePermissionsSpec(dstDir, "permissions-kilo", lists)
}

// kiloToolRules normalizes one tool's value into pattern-to-action
// pairs. Kilo accepts both shapes: "You can write each permission as
// one action for the whole tool or as a pattern map".
func kiloToolRules(raw json.RawMessage) map[string]string {
	var action string
	if err := json.Unmarshal(raw, &action); err == nil {
		return map[string]string{"*": action}
	}
	var patterns map[string]string
	if err := json.Unmarshal(raw, &patterns); err != nil {
		return nil
	}
	return patterns
}

// portableKiloRule spells one Kilo pattern in the portable vocabulary.
// A `*` pattern covers the whole tool, which is the bare tool name.
// A trailing ` *` on a shell pattern is Kilo's prefix form, which maps
// onto Claude's `:*` suffix; anything else is an exact or glob target
// and passes through inside the scope call.
func portableKiloRule(scope, pattern string) string {
	if pattern == "*" {
		return scope
	}
	if scope == "Bash" {
		if prefix, ok := strings.CutSuffix(pattern, " *"); ok && prefix != "" {
			return "Bash(" + prefix + ":*)"
		}
	}
	return scope + "(" + pattern + ")"
}

// isPortablePermissionList reports whether a Kilo action names one of
// the three portable lists. Kilo publishes exactly those three.
func isPortablePermissionList(action string) bool {
	switch action {
	case "allow", "deny", "ask":
		return true
	}
	return false
}

// sortedKiloTools returns the permission map's tool keys in a stable
// order so two imports of the same file produce the same spec.
func sortedKiloTools(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

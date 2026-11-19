package cli

import (
	"path/filepath"
	"strings"
)

// opencodeToolToPortable reverses the vocabulary the opencode adapter
// emits. Keys outside this table (`list`, `lsp`, `question`,
// `todowrite`, `external_directory`, `doom_loop`, and any namespaced
// `<server>_<tool>` MCP key) have no portable spelling and are left
// where they are rather than guessed at.
//
// The MCP form is asymmetric on purpose (#947). Emit writes
// `mcp__github__list_issues` out as `github_list_issues`, but nothing
// in that key says where the server name ends and the tool name
// starts, and both halves may carry underscores. Reading it back would
// invent an `mcp__` rule the author never wrote, so the key stays in
// `opencode.json` untouched. Kilo's importer declines the same join
// for the same reason.
//
// OpenCode's `edit` covers the portable Edit and Write both. It reads
// back as Edit, which re-emits to the same `edit` key, so the pair is
// a fixed point either way round.
var opencodeToolToPortable = map[string]string{
	"bash":      "Bash",
	"read":      "Read",
	"edit":      "Edit",
	"glob":      "Glob",
	"grep":      "Grep",
	"task":      "Task",
	"skill":     "Skill",
	"webfetch":  "WebFetch",
	"websearch": "WebSearch",
}

// importOpencodePermissions reads the committed `permission` map from
// `opencode.json` and writes it back as one Settings spec. OpenCode
// documents that file as the project-tier config: the same file this
// tool already merges `mcp` and `model` into, so a team that wrote a
// policy before adopting agnostic-ai keeps it (#922).
func importOpencodePermissions(root, dstDir string) (int, error) {
	return importPermissionMap(filepath.Join(root, opencodeMCPFile), opencodeToolToPortable, portableOpencodeRule, "permissions-opencode", dstDir)
}

// portableOpencodeRule composes the portable spelling for one tool and
// pattern.
//
// The catch-all is the whole tool, so it reads back as the bare name.
// A trailing ` *` is how the emitter spells this project's `:*` prefix
// convention, so it reverses to that. Anything else is a literal or a
// glob and passes through untouched.
//
// One lossy edge, deliberate: a hand-written OpenCode pattern that ends
// in a real ` *` is indistinguishable from the emitted prefix form and
// comes back as `Scope(prefix:*)`. Re-emitting that produces the same
// OpenCode pattern, so the file is a fixed point even where the spec
// spelling is not.
func portableOpencodeRule(scope, pattern string) string {
	if pattern == "*" {
		return scope
	}
	if prefix, isPrefixRule := strings.CutSuffix(pattern, " *"); isPrefixRule && prefix != "" {
		return scope + "(" + prefix + ":*)"
	}
	return scope + "(" + pattern + ")"
}

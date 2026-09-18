package emit

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// SharedAgentMarkdown renders the portable project-agent fields shared
// by Goose and OpenHands. Both tools discover flat Markdown files in
// .agents/agents, so every writer to that tree must use this renderer
// and produce byte-identical content.
//
// Explicit x-<target> metadata is preserved. When it differs between
// two targets sharing the default path, collision detection tells the
// user to choose a target-specific agents-dir instead of silently
// discarding requested behavior.
func SharedAgentMarkdown(agent spec.Entry, target string) string {
	resolved := ResolveMeta(agent.Meta, target)
	description, _ := resolved["description"].(string)
	if description == "" {
		description = agent.Name
	}
	meta := map[string]any{
		"name":        agent.Name,
		"description": description,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	MergeCustomTargetMeta(meta, &keys, agent.Meta, target, "name", "description", "model")

	front := FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(agent.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// WriteSharedAgentFiles writes one portable flat agent profile per spec.
func (s *Session) WriteSharedAgentFiles(agents []spec.Entry, target, agentsDir string, dryRun bool) error {
	for _, agent := range agents {
		path := filepath.Join(agentsDir, agent.Name+".md")
		content := WithHeader(SharedAgentMarkdown(agent, target), FormatMarkdown)
		if err := s.WriteFile(path, content, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// SharedAgentToolsDropped reports whether a generic tools list is
// omitted for target. An explicit x-<target>.tools key, including a nil
// delete marker, means the author already chose the target behavior and
// suppresses the generic-field coverage note.
func SharedAgentToolsDropped(agent spec.Entry, target string) bool {
	if len(StringSlice(agent.Meta["tools"])) == 0 {
		return false
	}
	custom, _ := agent.Meta[XPrefix+target].(map[string]any)
	_, explicit := custom["tools"]
	return !explicit
}

// SharedAgentFieldDropped reports whether a portable scalar frontmatter
// field is omitted for target because this renderer writes no key for
// it. An explicit x-<target> entry, including a nil delete marker,
// means the author already chose the target behavior and suppresses the
// coverage note.
func SharedAgentFieldDropped(agent spec.Entry, target, field string) bool {
	if value, _ := agent.Meta[field].(string); value == "" {
		return false
	}
	custom, _ := agent.Meta[XPrefix+target].(map[string]any)
	_, explicit := custom[field]
	return !explicit
}

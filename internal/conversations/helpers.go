package conversations

import (
	"slices"
	"strings"
)

// A helper is an agent a conversation started that is no row of its
// own and still works: a Claude Code or Codex native subagent, or a
// Codex run a Claude Code session started with `codex exec`. Helpers
// never become rows, and one that finished is not listed.
//
// Which session started a Codex run is read from the run's process, not
// guessed from its title, folder or time: the codex process holds its
// thread's writer lock, and inherits CLAUDE_CODE_SESSION_ID and
// CLAUDE_CODE_CHILD_SESSION=1 from the Claude Code Bash tool that
// started it. Every scan reads that link again; a run whose process is
// gone has finished, so nothing needs to remember it.
type helper struct {
	Tool  Tool   `json:"tool"`
	ID    string `json:"id"`
	Title string `json:"title"`
	// Kind is the agent type the tool gives, such as Explore, when it
	// gives one.
	Kind string `json:"kind,omitempty"`
	// Reply is a Claude Code subagent's latest message, as a row's
	// reply; empty before it wrote one, when it may hold a credential, or
	// when it is further back than claudeReplyWindow.
	Reply     string      `json:"reply,omitempty"`
	State     helperState `json:"state"`
	UpdatedAt string      `json:"updatedAt"`
	// ParentID is the helper that started this one, when another helper
	// did. It may name a helper no longer listed.
	ParentID string `json:"parentId,omitempty"`
	// Reach is how the person reaches the helper's own view.
	Reach helperReach `json:"reach,omitempty"`
	// ParentName names the helper it runs inside, for reachNested.
	ParentName string `json:"parentName,omitempty"`
	// name is the name the tool shows the helper by, when it has one,
	// such as a teammate's.
	name string
	// member is a pane teammate's id in its team, which its process names.
	member string
}

// helperReach is how the person reaches a helper's own view.
type helperReach string

const (
	// reachPane: it runs in a terminal pane of its own.
	reachPane helperReach = "pane"
	// reachRows: it is one of the agent rows below its chat's prompt.
	reachRows helperReach = "rows"
	// reachNested: it runs inside another helper, whose view lists it.
	reachNested helperReach = "nested"
	// reachCodexThreads: Codex's /subagents command switches to it.
	reachCodexThreads helperReach = "codex-threads"
)

// shownName is the name a note calls a helper by.
func (h helper) shownName() string {
	return firstNonEmpty(h.name, h.Title, h.Kind)
}

// helperState is whether a helper still runs. Only working helpers are
// listed, so it is always helperWorking; the API names it all the same.
type helperState string

const helperWorking helperState = "working"

// helperTitle is a helper's title, read like a row's: its own title, or
// the first line of its first prompt. Empty when it may hold a credential.
func (s *Scanner) helperTitle(title, firstPrompt string) string {
	return s.shown.get(texts{title: title, firstPrompt: firstPrompt}).title
}

// codexExecRun is a running `codex exec` thread: never a row, and listed
// only under the Claude Code session that started it.
type codexExecRun struct {
	helper helper
	// startedBy is the session the run's process environment names.
	startedBy Ref
}

// attachRuns lists each Codex run under the row of the session that
// started it. A row's own helpers are shared with kept results, so they
// are copied before the runs join them.
func attachRuns(results []sourceResult) {
	byParent := map[Ref][]helper{}
	for _, result := range results {
		for _, run := range result.runs {
			byParent[run.startedBy] = append(byParent[run.startedBy], run.helper)
		}
	}
	if len(byParent) == 0 {
		return
	}
	for r := range results {
		for i := range results[r].conversations {
			c := &results[r].conversations[i]
			runs := byParent[Ref{Tool: c.Tool, ID: c.ID}]
			if len(runs) > 0 {
				c.Helpers = sortedHelpers(append(slices.Clone(c.Helpers), runs...))
			}
		}
	}
}

// listsHelper reports whether a row in results lists the helper. Only
// such a helper may be opened: results hold only the rows of the
// requested scope, so a run or subagent of a chat outside it is not.
func listsHelper(results []sourceResult, tool Tool, id string) bool {
	for _, result := range results {
		for _, c := range result.conversations {
			for _, h := range c.Helpers {
				if h.Tool == tool && h.ID == id {
					return true
				}
			}
		}
	}
	return false
}

// ValidAgentID reports whether id can name a Claude Code subagent or
// teammate: 1 to 128 letters, digits, hyphens, underscores or at signs,
// so it never reaches outside the subagents folder. A pane teammate's
// id is its member id, such as reviewer@session-0d2fda98.
func ValidAgentID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	return strings.IndexFunc(id, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '@'
	}) < 0
}

// ancestorsOf lists the listed helpers h runs inside, nearest first. A
// parent that is not listed, or a cycle, ends the walk.
func ancestorsOf(helpers []helper, h helper) []helper {
	byID := make(map[string]helper, len(helpers))
	for _, other := range helpers {
		if other.Tool == h.Tool {
			byID[other.ID] = other
		}
	}
	seen := map[string]bool{h.ID: true}
	var ancestors []helper
	for parent, ok := byID[h.ParentID]; ok && !seen[parent.ID]; parent, ok = byID[parent.ParentID] {
		seen[parent.ID] = true
		ancestors = append(ancestors, parent)
	}
	return ancestors
}

// subagentIn finds the Claude Code subagent id among helpers.
func subagentIn(helpers []helper, id string) (helper, bool) {
	for _, h := range helpers {
		if h.Tool == ToolClaude && h.ID == id {
			return h, true
		}
	}
	return helper{}, false
}

// sortedHelpers orders helpers newest first, then by tool and id, so a
// list that did not change keeps its order.
func sortedHelpers(helpers []helper) []helper {
	slices.SortStableFunc(helpers, func(a, b helper) int {
		if by := strings.Compare(b.UpdatedAt, a.UpdatedAt); by != 0 {
			return by
		}
		if by := strings.Compare(string(a.Tool), string(b.Tool)); by != 0 {
			return by
		}
		return strings.Compare(a.ID, b.ID)
	})
	return helpers
}

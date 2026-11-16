package conversations

import (
	"slices"
	"strings"
)

// A helper is an agent a conversation started that is no row of its
// own: a Claude Code or Codex native subagent, or a Codex run a Claude
// Code session started with `codex exec`. Helpers never become rows.
//
// Which session started a Codex run is read from the run's process, not
// guessed from its title, folder or time: the codex process holds its
// thread's writer lock, and inherits CLAUDE_CODE_SESSION_ID and
// CLAUDE_CODE_CHILD_SESSION=1 from the Claude Code Bash tool that
// started it. The process is gone once the run ends, so the link is
// saved in the marks file the first time it is seen. A run never seen
// running has no link and shows nowhere.
type helper struct {
	Tool  Tool   `json:"tool"`
	ID    string `json:"id"`
	Title string `json:"title"`
	// Kind is the agent type the tool gives, such as Explore, when it
	// gives one.
	Kind      string      `json:"kind,omitempty"`
	State     helperState `json:"state"`
	UpdatedAt string      `json:"updatedAt"`
}

// helperState is whether a helper still runs.
type helperState string

const (
	helperWorking  helperState = "working"
	helperFinished helperState = "finished"
)

// helperTitle is a helper's title, read like a row's: its own title, or
// the first line of its first prompt. Empty when it may hold a credential.
func (s *Scanner) helperTitle(title, firstPrompt string) string {
	return s.shown.get(texts{title: title, firstPrompt: firstPrompt}).title
}

// codexExecRun is a `codex exec` thread: never a row, and listed only under
// the Claude Code session that started it.
type codexExecRun struct {
	helper helper
	// startedBy is the session this read saw running it; nil when no
	// process ran it at this read.
	startedBy *Ref
}

// attachRuns lists each Codex run under the row of the session that
// started it, as parents names them. A row's own helpers are shared
// with kept results, so they are copied before the runs join them.
func attachRuns(results []sourceResult, parents map[string]Ref) {
	byParent := map[Ref][]helper{}
	for _, result := range results {
		for _, run := range result.runs {
			if parent, linked := parents[run.helper.ID]; linked {
				byParent[parent] = append(byParent[parent], run.helper)
			}
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

// observedRunParents are the links between a Codex run and the session
// that started it that this read saw in running processes.
func observedRunParents(results []sourceResult) map[string]Ref {
	parents := map[string]Ref{}
	for _, result := range results {
		for _, run := range result.runs {
			if run.startedBy != nil {
				parents[run.helper.ID] = *run.startedBy
			}
		}
	}
	return parents
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

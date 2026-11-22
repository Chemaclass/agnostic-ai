package conversations

import "testing"

func TestRefsRead_OnlyACompleteListingWithoutTheFilesMakesARefGone(t *testing.T) {
	live := Ref{Tool: ToolClaude, ID: "live"}
	unopened := Ref{Tool: ToolClaude, ID: "unopened"}
	gone := Ref{Tool: ToolClaude, ID: "gone"}
	codex := Ref{Tool: ToolCodex, ID: "thread"}
	results := []sourceResult{
		{
			source:        Source{Tool: ToolClaude, Found: true},
			conversations: []Conversation{{Tool: ToolClaude, ID: "live"}},
			inventories:   []inventory{{tool: ToolClaude, files: map[string]string{"live": "a.jsonl", "unopened": "b.jsonl"}}},
		},
		// A store read that failed lists nothing completely.
		missing(ToolCodex, "Could not read the Codex thread index"),
	}
	read := refsRead(results, []Ref{live, unopened, gone, codex})
	if _, found := read.Rows[live]; !found || len(read.Rows) != 1 {
		t.Errorf("rows = %v", read.Rows)
	}
	if !read.Complete[ToolClaude] || read.Complete[ToolCodex] {
		t.Errorf("complete = %v, want Claude Code only", read.Complete)
	}
	if !read.Listed[unopened] || !read.Listed[live] || read.Listed[gone] || read.Listed[codex] {
		t.Errorf("listed = %v", read.Listed)
	}
}

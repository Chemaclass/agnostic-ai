package conversations

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	idParent = "abababab-abab-4bab-8bab-abababababab"
	idChild  = "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
)

func TestSplitLink_ReadsAStrictLineForEachTool(t *testing.T) {
	for line, want := range map[string]Ref{
		"Continues claude:" + idParent:      {Tool: ToolClaude, ID: idParent},
		"Continues codex:" + idParent:       {Tool: ToolCodex, ID: idParent},
		"Continues cursor:agent_Chat-12345": {Tool: ToolCursor, ID: "agent_Chat-12345"},
	} {
		ref, rest, ok := splitLink(line + "\n\nWrite the release notes\n## Context\nOn main.")
		if !ok || ref != want {
			t.Errorf("%q: ref = %+v ok %v, want %+v", line, ref, ok, want)
		}
		if rest != "Write the release notes\n## Context\nOn main." {
			t.Errorf("%q: rest = %q", line, rest)
		}
	}
}

func TestSplitLink_KeepsALookAlikeAsText(t *testing.T) {
	for name, prompt := range map[string]string{
		"lower case":          "continues claude:" + idParent + "\nWrite the notes",
		"colon after word":    "Continues: claude:" + idParent + "\nWrite the notes",
		"words after id":      "Continues claude:" + idParent + " and more\nWrite the notes",
		"unknown tool":        "Continues gemini:" + idParent + "\nWrite the notes",
		"bad claude id":       "Continues claude:not-an-id\nWrite the notes",
		"cursor id too short": "Continues cursor:abc\nWrite the notes",
		"no id":               "Continues claude:\nWrite the notes",
		"line not first":      "Write the notes\nContinues claude:" + idParent,
		"plain sentence":      "Continues the work from yesterday",
		"leading spaces":      "  Continues claude:" + idParent + "\nWrite the notes",
		"two spaces":          "Continues  claude:" + idParent + "\nWrite the notes",
		"tab":                 "Continues\tclaude:" + idParent + "\nWrite the notes",
	} {
		if ref, rest, ok := splitLink(prompt); ok || ref != (Ref{}) || rest != prompt {
			t.Errorf("%s: ref = %+v ok %v rest %q, want the text kept", name, ref, ok, rest)
		}
	}
}

func TestShowTexts_CutsTheLinkLineFromTitleOpeningAndPrompt(t *testing.T) {
	brief := "Continues claude:" + idParent + "\n\nWrite the release notes\n## Context\nBranch main."
	shown := showTexts(texts{firstPrompt: brief, lastPrompt: brief})
	if shown.parent == nil || *shown.parent != (Ref{Tool: ToolClaude, ID: idParent}) {
		t.Fatalf("parent = %+v", shown.parent)
	}
	if shown.title != "Write the release notes" {
		t.Errorf("title = %q", shown.title)
	}
	if !slices.Equal(shown.opening, []string{"Write the release notes", "## Context", "Branch main."}) {
		t.Errorf("opening = %q", shown.opening)
	}
	if shown.prompt != "Write the release notes ## Context Branch main." {
		t.Errorf("prompt = %q", shown.prompt)
	}

	codex := showTexts(texts{title: brief, firstPrompt: brief, lastPrompt: "Now the API page"})
	if codex.title != "Write the release notes ## Context Branch main." || codex.prompt != "Now the API page" {
		t.Errorf("a title equal to the first prompt keeps the line: title %q prompt %q", codex.title, codex.prompt)
	}
	named := showTexts(texts{title: "Release notes", firstPrompt: brief})
	if named.title != "Release notes" {
		t.Errorf("a tool title of its own stays: %q", named.title)
	}
}

func TestShowTexts_WithholdsABriefThatMayHoldACredentialAndKeepsTheParent(t *testing.T) {
	brief := "Continues codex:" + idParent + "\n\nRotate the key\nAWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	shown := showTexts(texts{firstPrompt: brief})
	if len(shown.opening) != 0 || !slices.Contains(shown.withheld, FieldOpening) {
		t.Errorf("opening = %q withheld %q", shown.opening, shown.withheld)
	}
	if shown.parent == nil || *shown.parent != (Ref{Tool: ToolCodex, ID: idParent}) {
		t.Errorf("parent = %+v", shown.parent)
	}
}

func TestFill_KeepsALinkToTheRowItselfAsText(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	line := "Continues claude:" + idChild
	c := Conversation{Tool: ToolClaude, ID: idChild}
	scanner.fill(&c, texts{firstPrompt: line + "\nWrite the notes"})
	if c.Parent != nil || c.Title != line || c.Opening[0] != line {
		t.Errorf("a link to itself: parent %+v title %q opening %q", c.Parent, c.Title, c.Opening)
	}
}

func TestScan_ReadsTheParentOfAChildOfTheSameToolAndOfAnother(t *testing.T) {
	codexChild, agent := "77777777-7777-4777-8777-777777777777", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	brief := func(tool Tool, id string) string {
		return "Continues " + string(tool) + ":" + id + "\n\nWrite the release notes"
	}
	meta := hex.EncodeToString([]byte(`{"agentId":"` + agent + `","name":"New Agent","createdAt":1762500000000}`))
	codexBrief := brief(ToolClaude, idParent)
	scanner, stores := testScanner(t, fakeStores(t, map[string]string{
		"state_5.sqlite|FROM threads":      `[{"id":"` + codexChild + `","cwd":"/tmp/app","created_at_ms":1762500000000,"updated_at_ms":1762520000000,"originator":"codex-tui","source":"cli","title":` + asJSON(t, codexBrief) + `,"first_user_message":` + asJSON(t, codexBrief) + `}]`,
		"state.vscdb|FROM composerHeaders": `[]`,
		"store.db|FROM meta":               `[{"value":"` + meta + `"}]`,
	}), fakeProcesses{})
	write(t, filepath.Join(stores.CodexDir, "state_5.sqlite"), "")
	write(t, stores.CursorDB, "")
	write(t, filepath.Join(stores.CursorDir, "chats", md5Hex("/tmp/app"), agent, "store.db"), "")
	write(t, filepath.Join(stores.CursorDir, "projects", "p1", "agent-transcripts", agent, agent+".jsonl"), lines(
		obj{"role": "user", "message": obj{"content": []obj{{"type": "text", "text": "<user_query>" + brief(ToolCodex, codexChild) + "</user_query>"}}}},
	))
	writeSession(t, stores, idParent, typedPrompt("Plan the release"))
	writeSession(t, stores, idChild, typedPrompt(brief(ToolClaude, idParent)))
	scanner.Now = func() time.Time { return time.UnixMilli(1762800000000) }

	view := scanner.Scan(context.Background(), ScanOptions{})

	for id, want := range map[string]Ref{
		idChild:    {Tool: ToolClaude, ID: idParent},
		codexChild: {Tool: ToolClaude, ID: idParent},
		agent:      {Tool: ToolCodex, ID: codexChild},
	} {
		c := byID(t, view, id)
		if c.Parent == nil || *c.Parent != want {
			t.Errorf("%s parent = %+v, want %+v", id[:4], c.Parent, want)
		}
		if c.Title != "Write the release notes" || slices.ContainsFunc(c.Opening, func(line string) bool { return line != "Write the release notes" }) {
			t.Errorf("%s title %q opening %q", id[:4], c.Title, c.Opening)
		}
	}
	if parent := byID(t, view, idParent); parent.Parent != nil {
		t.Errorf("a first prompt with no link line gives no parent: %+v", parent.Parent)
	}
}

func TestScan_ListsAChildWhoseParentIsNotOnThePage(t *testing.T) {
	const old = "efefefef-efef-4fef-8fef-efefefefefef"
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{alive: map[int]bool{4242: true}})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeSessionSaying(t, stores, old, "2026-10-01T08:00:00.000Z", "Plan the release")
	writeSessionSaying(t, stores, idParent, "2026-11-07T08:00:00.000Z", "Fix the parser")
	writeSessionSaying(t, stores, idChild, "2026-11-07T11:00:00.000Z", "Continues claude:"+idParent+"\n\nWrite the release notes")
	writeSessionSaying(t, stores, idSide, "2026-11-07T11:00:00.000Z", "Continues claude:"+old+"\n\nTag the release")
	write(t, filepath.Join(stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+idChild+`","procStart":"Sat Nov  7 08:00:00 2026","status":"waiting","entrypoint":"cli"}`)
	now := scanner.Now
	scanner.Now = func() time.Time { return now().Add(-4 * time.Hour) }
	scanner.Scan(context.Background(), ScanOptions{})
	scanner.Now = now
	if err := scanner.Mark(ToolClaude, idParent, MarkHidden); err != nil {
		t.Fatal(err)
	}
	if err := scanner.Mark(ToolClaude, idSide, MarkSeen); err != nil {
		t.Fatal(err)
	}

	view := scanner.Scan(context.Background(), ScanOptions{})

	child := byID(t, view, idChild)
	if child.Parent == nil || child.Parent.ID != idParent || child.State != StateBlocked {
		t.Errorf("a child of a hidden parent = parent %+v state %s", child.Parent, child.State)
	}
	if view.NeedsYou != 1 {
		t.Errorf("needsYou = %d, want the waiting child counted", view.NeedsYou)
	}
	if side := byID(t, view, idSide); side.Parent == nil || side.Parent.ID != old {
		t.Errorf("a child of a parent older than the page = %+v", side.Parent)
	}
	for _, c := range view.Conversations {
		if c.ID == old || c.ID == idParent {
			t.Errorf("parent %s is on the page", c.ID[:4])
		}
	}
}

func TestScan_ArchivingHidingOrRenamingAParentLeavesItsChild(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	writeSessionSaying(t, stores, idParent, "2026-11-07T08:00:00.000Z", "Fix the parser")
	writeSessionSaying(t, stores, idChild, "2026-11-07T11:00:00.000Z", "Continues claude:"+idParent+"\n\nWrite the release notes")
	before := asJSON(t, byID(t, scanner.Scan(context.Background(), ScanOptions{}), idChild))

	if err := scanner.Rename(ToolClaude, idParent, "Parser work"); err != nil {
		t.Fatal(err)
	}
	for _, mark := range []Mark{MarkCompleted, MarkHidden} {
		if err := scanner.Mark(ToolClaude, idParent, mark); err != nil {
			t.Fatal(err)
		}
	}

	if after := asJSON(t, byID(t, scanner.Scan(context.Background(), ScanOptions{}), idChild)); after != before {
		t.Errorf("the child changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestShowTexts_ReadsTheLinkFromTheFirstTypedLine(t *testing.T) {
	want := Ref{Tool: ToolClaude, ID: idParent}
	for name, prompt := range map[string]string{
		"an injected block first": "<system-reminder>Today is Monday.</system-reminder>\nContinues claude:" + idParent + "\n\nWrite the notes",
		"a wrapped prompt":        "<user_query>\nContinues claude:" + idParent + "\n\nWrite the notes\n</user_query>",
		"a wrapper on the line":   "<user_query>Continues claude:" + idParent + "</user_query>\nWrite the notes",
		"a trailing CR":           "Continues claude:" + idParent + "\r\n\r\nWrite the notes",
	} {
		shown := showTexts(texts{firstPrompt: prompt, lastPrompt: prompt})
		if shown.parent == nil || *shown.parent != want || shown.title != "Write the notes" || shown.prompt != "Write the notes" {
			t.Errorf("%s: parent %+v title %q prompt %q", name, shown.parent, shown.title, shown.prompt)
		}
	}
	for name, prompt := range map[string]string{
		"leading spaces": "  Continues claude:" + idParent + "\nWrite the notes",
		"two spaces":     "Continues  claude:" + idParent + "\nWrite the notes",
		"a tab":          "Continues\tclaude:" + idParent + "\nWrite the notes",
		"trailing text":  "Continues claude:" + idParent + " now\nWrite the notes",
		"wrapped spaces": "<user_query>  Continues claude:" + idParent + "</user_query>\nWrite the notes",
	} {
		shown := showTexts(texts{firstPrompt: prompt, lastPrompt: prompt})
		if shown.parent != nil || !strings.HasPrefix(shown.title, "Continues") || !strings.HasPrefix(shown.prompt, "Continues") {
			t.Errorf("%s: parent %+v title %q prompt %q", name, shown.parent, shown.title, shown.prompt)
		}
	}
}

func TestShowTexts_GivesNoParentForAnIDThatMayBeACredential(t *testing.T) {
	prompt := "Continues cursor:ghp_abcdefghijklmnopqrstuvwxyz0123456789\n\nhello there"
	shown := showTexts(texts{firstPrompt: prompt, lastPrompt: prompt})
	if shown.parent != nil || len(shown.opening) != 0 || shown.prompt != "" {
		t.Errorf("parent %+v opening %q prompt %q withheld %q", shown.parent, shown.opening, shown.prompt, shown.withheld)
	}
}

func TestShowTexts_GivesNoParentForALinkInsideANestedInjectedBlock(t *testing.T) {
	prompt := "<system-reminder>Context <ide_selection>x</ide_selection>\nContinues claude:" + idParent + "\n</system-reminder>\nWrite the notes"
	shown := showTexts(texts{firstPrompt: prompt, lastPrompt: prompt})
	if shown.parent != nil || shown.title == "</system-reminder>" {
		t.Errorf("parent %+v title %q opening %q", shown.parent, shown.title, shown.opening)
	}
}

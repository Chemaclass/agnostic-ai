package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestImportFromClaude_MovesSkillAllowedToolsUnderXClaude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/skills/style/SKILL.md"),
		"---\nname: style\ndescription: Style guide.\nallowed-tools: \"Read, Bash(*)\"\nmodel: sonnet\n---\n\nBody.\n")
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "skills/style/SKILL.md"))
	want := "---\nname: style\ndescription: Style guide.\nx-claude:\n  allowed-tools: \"Read, Bash(*)\"\nmodel: sonnet\n---\n\nBody.\n"
	if got != want {
		t.Errorf("SKILL.md:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestImportFromClaude_MovesCommandAllowedToolsIntoExistingXClaude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/commands/fix.md"),
		"---\ndescription: Fix a bug\nallowed-tools:\n  - Read\n  - Edit\nx-claude:\n  model: opus\n---\n\nFix it.\n")
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "commands/fix.md"))
	want := "---\ndescription: Fix a bug\nx-claude:\n  model: opus\n  allowed-tools:\n    - Read\n    - Edit\n---\n\nFix it.\n"
	if got != want {
		t.Errorf("command:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMoveClaudeOnlyKeys_LeavesAFlowXClaudeAlone(t *testing.T) {
	in := "---\nname: a\nallowed-tools: Read\nx-claude: {model: opus}\n---\n\nBody.\n"
	if got := moveClaudeOnlyKeys(in); got != in {
		t.Errorf("a flow x-claude mapping must stay as written:\n%s", got)
	}
}

func TestMoveClaudeOnlyKeys_KeepsAnXClaudeValueThatIsAlreadySet(t *testing.T) {
	in := "---\nname: a\nallowed-tools: Read\nx-claude:\n  allowed-tools: Grep\n---\n\nBody.\n"
	if got := moveClaudeOnlyKeys(in); got != in {
		t.Errorf("an x-claude value already set must not be replaced:\n%s", got)
	}
}

func TestImportFromClaude_NamesHookAfterItsCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/settings.json"), `{"hooks":{
  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}],
  "PostToolUse":[{"matcher":"Edit|Write","hooks":[{"type":"command","command":"\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/format.sh --fix"}]}]
}}`)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	pre := readFile(t, filepath.Join(dir, "hooks/pretooluse-bash-exit-0.yaml"))
	for _, want := range []string{"name: pretooluse-bash-exit-0\n", "description: Runs `exit 0` on PreToolUse for Bash.\n"} {
		if !strings.Contains(pre, want) {
			t.Errorf("missing %q in:\n%s", want, pre)
		}
	}
	post := readFile(t, filepath.Join(dir, "hooks/posttooluse-edit-write-format.yaml"))
	if !strings.Contains(post, "name: posttooluse-edit-write-format\n") {
		t.Errorf("hook name should come from the script:\n%s", post)
	}
}

func TestImportFromClaude_HooksWithTheSameReadableNameStayApart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/settings.json"), `{"hooks":{"Stop":[
  {"hooks":[{"type":"command","command":"make lint"}]},
  {"hooks":[{"type":"command","command":"make lint --fast"}]}
]}}`)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "hooks", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 hook specs, got %v", files)
	}
	if filepath.Base(files[1]) != "stop-make-lint.yaml" {
		t.Errorf("the first hook keeps the readable name, got %v", files)
	}
}

// A hook spec an older release wrote under its hash name keeps that
// name, so a re-import updates it instead of adding a second copy.
func TestImportFromClaude_KeepsAHashNamedHookSpec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}]}}`)
	legacy := hookSpecName("PreToolUse", "Bash", []string{"exit 0"})
	writeFile(t, filepath.Join(dir, "hooks", legacy+".yaml"), "name: "+legacy+"\nevent: PreToolUse\nmatcher: Bash\ncommand: exit 0\n")
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "hooks", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != legacy+".yaml" {
		t.Fatalf("want only %s.yaml, got %v", legacy, files)
	}
	if got := readFile(t, files[0]); !strings.Contains(got, "description: ") {
		t.Errorf("re-import should add the description:\n%s", got)
	}
}

func TestImportFromClaude_ImportedSpecsLeaveLintQuiet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/skills/style/SKILL.md"),
		"---\nname: style\ndescription: Style guide.\nallowed-tools: Read, Grep\n---\n\nBody.\n")
	writeFile(t, filepath.Join(dir, ".claude/settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"exit 0"}]}]}}`)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	skill, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(readFile(t, filepath.Join(dir, "skills/style/SKILL.md"))))
	if err != nil {
		t.Fatal(err)
	}
	hook, err := spec.ParseYAMLBytes(spec.KindHook, []byte(readFile(t, filepath.Join(dir, "hooks/pretooluse-bash-exit-0.yaml"))))
	if err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{skill, hook}
	findings := append(lintEmptySpecs(entries), lintNearMissKeys(entries, []string{"claude", "codex"})...)
	for _, f := range findings {
		t.Errorf("%s %s: %s", f.Code, f.Path, f.Message)
	}
}

func TestImportFromClaude_TellsWhatToDoWithTheNestedEntryFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), "# Project\n")
	out := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	want := "  ! delete .claude/CLAUDE.md after the next sync: sync writes its text to CLAUDE.md, and Claude Code loads both files\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestImportFromClaude_NoEntryFileNoteWhenClaudeIsNotSynced(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	writeFile(t, filepath.Join(dir, ".claude/CLAUDE.md"), "# Project\n")
	out := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "delete .claude/CLAUDE.md") {
		t.Errorf("no Claude entry file is written, so no note:\n%s", out)
	}
}

func TestImportFromClaude_ListsClaudeFilesItLeftBehind(t *testing.T) {
	dir := t.TempDir()
	for path, body := range map[string]string{
		".claude/CLAUDE.md":                 "# Project\n",
		".claude/settings.json":             "{}\n",
		".claude/settings.local.json":       "{}\n",
		".claude/skills/style/SKILL.md":     "---\nname: style\ndescription: d\n---\n\nBody.\n",
		".claude/skills/style/ref.md":       "ref\n",
		".claude/skills/draft/notes.md":     "no SKILL.md here\n",
		".claude/agents/reviewer.md":        "---\nname: reviewer\ndescription: d\n---\n\nReview.\n",
		".claude/hooks/format.sh":           "#!/bin/sh\n",
		".claude/templates/post.md":         "template\n",
		".claude/output-styles/terse.md":    "terse\n",
		".claude/agent-memory/reviewer.md":  "memory\n",
		".claude/worktrees/feature/file.go": "package x\n",
		".claude/.DS_Store":                 "x",
	} {
		writeFile(t, filepath.Join(dir, path), body)
	}
	out := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	want := "  ! not imported, left in place (sync neither copies nor removes them):\n" +
		"      .claude/output-styles/terse.md\n" +
		"      .claude/skills/draft/notes.md\n" +
		"      .claude/templates/post.md\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("missing:\n%s\nin:\n%s", want, out)
	}
}

func TestImportFromClaude_NoLeftBehindListWhenEverythingIsImported(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/skills/style/SKILL.md"), "---\nname: style\ndescription: d\n---\n\nBody.\n")
	out := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "not imported") {
		t.Errorf("unexpected left-behind list:\n%s", out)
	}
}

func TestClaudeFilesNotImported_CapsTheList(t *testing.T) {
	var paths []string
	for i := range 13 {
		paths = append(paths, filepath.Join(".claude", "templates", string(rune('a'+i))+".md"))
	}
	out := captureSummary(t)
	reportClaudeFilesNotImported(paths)
	if !strings.Contains(out.String(), "      .claude/templates/j.md\n      and 3 more\n") {
		t.Errorf("list should stop at 10 and count the rest:\n%s", out)
	}
	if strings.Contains(out.String(), "k.md") {
		t.Errorf("list should stop at 10:\n%s", out)
	}
}

func TestImportFromClaude_DoesNotListSyncedOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude/rules/extra/notes.txt"), "not markdown\n")
	writeFile(t, filepath.Join(dir, ".claude/generated.md"), "<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\nx\n")
	out := captureSummary(t)
	if err := importFromClaude(dir, rootSources(), defaultClaudeLayout()); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "generated.md") {
		t.Errorf("a file sync wrote is not user content to list:\n%s", got)
	}
	if !strings.Contains(got, "      .claude/rules/extra/notes.txt\n") {
		t.Errorf("a non-markdown file in rules/ is not imported:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude/rules/extra/notes.txt")); err != nil {
		t.Errorf("the file must stay in place: %v", err)
	}
}

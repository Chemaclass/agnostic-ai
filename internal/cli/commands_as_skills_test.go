package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const reviewCommand = "---\nname: review\ndescription: Review the diff.\nargument-hint: \"[path]\"\n---\n\nReview $ARGUMENTS.\n"

func commandsAsSkillsProject(t *testing.T, targets string) *bytes.Buffer {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "review.md"), reviewCommand)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	return &notes
}

func TestSync_WritesACommandAsAUserRunSkillOnATargetWithoutCommands(t *testing.T) {
	notes := commandsAsSkillsProject(t, "claude, codex, copilot, gemini")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, native := range []string{filepath.Join(".claude", "commands", "review.md"), filepath.Join(".gemini", "commands", "review.toml")} {
		if _, err := os.Stat(native); err != nil {
			t.Errorf("want the native command %s: %v", native, err)
		}
	}
	if _, err := os.Stat(filepath.Join(".claude", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("claude has commands, so no skill, stat err = %v", err)
	}
	copilot := readFile(t, filepath.Join(".github", "skills", "review", "SKILL.md"))
	for _, want := range []string{"name: review", "description: Review the diff.", "disable-model-invocation: true", "Run it only when the user calls it by name", "Review $ARGUMENTS."} {
		if !strings.Contains(copilot, want) {
			t.Errorf("want %q in the copilot skill:\n%s", want, copilot)
		}
	}
	codex := readFile(t, filepath.Join(".agents", "skills", "review", "SKILL.md"))
	if !strings.Contains(codex, "Review $ARGUMENTS.") {
		t.Errorf("want the command body in the codex skill:\n%s", codex)
	}
	if policy := readFile(t, filepath.Join(".agents", "skills", "review", "agents", "openai.yaml")); !strings.Contains(policy, "allow_implicit_invocation: false") {
		t.Errorf("codex must run the skill only when called:\n%s", policy)
	}
	for _, want := range []string{
		"1 command reaches codex, copilot but not as a command (written as a skill the user runs by name)",
		"gemini also reads .agents/skills, so it lists review both as its own command and as the skill written there for codex",
	} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("want note %q, got:\n%s", want, notes.String())
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}

	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, gemini]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync without copilot: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".github", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("turning copilot off must remove its skill, stat err = %v", err)
	}
}

func TestSync_KeepsTheCommandFolderOnCodexWithACommandsDir(t *testing.T) {
	commandsAsSkillsProject(t, "codex")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    commands-dir: .codex/prompts\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".codex", "prompts", "review.md")); err != nil {
		t.Errorf("want the custom prompt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("no skill beside the custom prompt, stat err = %v", err)
	}
}

func TestSync_ASkillSpecOfTheSameNameWinsOverACommand(t *testing.T) {
	notes := commandsAsSkillsProject(t, "copilot")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review skill.\n---\n\nThe skill body.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	got := readFile(t, filepath.Join(".github", "skills", "review", "SKILL.md"))
	if !strings.Contains(got, "The skill body.") || strings.Contains(got, "Review $ARGUMENTS.") {
		t.Errorf("the skill spec must win:\n%s", got)
	}
	if want := "1 command reaches copilot but not as a skill (the skill spec of the same name takes its folder: review)"; !strings.Contains(notes.String(), want) {
		t.Errorf("want note %q, got:\n%s", want, notes.String())
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Aider writes skills only into its always-loaded rules file.
func TestSync_LeavesCommandsOffAider(t *testing.T) {
	commandsAsSkillsProject(t, "aider")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\noutputs:\n  aider:\n    rules-file: AIDER.md\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, "AIDER.md"); strings.Contains(got, "Review $ARGUMENTS.") {
		t.Errorf("a command must not load in every aider session:\n%s", got)
	}
}

func TestImport_LeavesACommandWrittenAsASkillToTheCommandSpec(t *testing.T) {
	commandsAsSkillsProject(t, "codex")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	importCapturing(t, "codex")

	if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("import must not copy the command skill back, stat err = %v", err)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

func TestLint_CountsCommandsWrittenAsSkillsAsConsumed(t *testing.T) {
	commandsAsSkillsProject(t, "copilot")

	out, _ := runCLI(t, "lint")
	if strings.Contains(out, "LINT004") {
		t.Errorf("copilot writes the command as a skill, got:\n%s", out)
	}
}

// Codex keeps the flag in agents/openai.yaml, so a Copilot SKILL.md in
// the same folder could not keep it and still match Codex's copy.
func TestSync_CopilotSharingCodexSkillsFolderKeepsFilesInStep(t *testing.T) {
	notes := commandsAsSkillsProject(t, "codex, copilot")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, copilot]\noutputs:\n  copilot:\n    skills-dir: .agents/skills\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nargument-hint: \"[env]\"\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(".agents", "skills", "review", "agents", "openai.yaml")); !strings.Contains(got, "allow_implicit_invocation: false") {
		t.Errorf("codex still gets the user-run skill:\n%s", got)
	}
	if want := "copilot also reads .agents/skills, so its model can start review on its own from the skill written there for codex"; !strings.Contains(notes.String(), want) {
		t.Errorf("want note %q, got:\n%s", want, notes.String())
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// Copilot's own .github/skills copy wins over the Codex copy in
// .agents/skills, so Copilot needs no note about it.
func TestSync_NoReaderNoteWhenCopilotHasItsOwnCopy(t *testing.T) {
	notes := commandsAsSkillsProject(t, "codex, copilot")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(notes.String(), "copilot also reads") {
		t.Errorf("copilot has its own user-run copy, got:\n%s", notes.String())
	}
}

func TestSync_LeavesCommandsOffCodexWithoutSkills(t *testing.T) {
	notes := commandsAsSkillsProject(t, "codex")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\noutputs:\n  codex:\n    shared-subagents: false\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(notes.String(), "written as a skill the user runs by name") {
		t.Errorf("codex writes no skills here, got:\n%s", notes.String())
	}
}

func TestSync_WritesANestedCommandAsATopLevelSkill(t *testing.T) {
	commandsAsSkillsProject(t, "codex")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "git", "commit.md"), "---\nname: commit\ndescription: Commit.\n---\n\nCommit.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "commit", "SKILL.md")); err != nil {
		t.Errorf("want the command at the top level: %v", err)
	}
	if _, err := os.Stat(filepath.Join("git", ".agents")); !os.IsNotExist(err) {
		t.Errorf("no skills tree under git/, stat err = %v", err)
	}
}

func TestSync_ASkillWhoseNameDiffersOnlyInCaseWinsOverACommand(t *testing.T) {
	notes := commandsAsSkillsProject(t, "copilot")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "Helper.md"), "---\nname: Helper\ndescription: Helps.\n---\n\nCommand body.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "helper", "SKILL.md"), "---\nname: helper\ndescription: Helps.\n---\n\nSkill body.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(".github", "skills", "helper", "SKILL.md")); !strings.Contains(got, "Skill body.") {
		t.Errorf("the skill spec must win:\n%s", got)
	}
	// A case-insensitive disk resolves Helper to helper, so list names.
	entries, err := os.ReadDir(filepath.Join(".github", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "Helper" {
			t.Errorf("the command must not get its own folder next to the skill")
		}
	}
	if want := "the skill spec of the same name takes its folder: Helper"; !strings.Contains(notes.String(), want) {
		t.Errorf("want note %q, got:\n%s", want, notes.String())
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

func TestSync_NotesClaudeSyntaxOnTheCommandLine(t *testing.T) {
	notes := commandsAsSkillsProject(t, "copilot")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	// review.md has five frontmatter lines and a blank line before its body.
	if want := filepath.Join(".agnostic-ai", "commands", "review.md") + ":7"; !strings.Contains(notes.String(), want) {
		t.Errorf("want the note to name %s, got:\n%s", want, notes.String())
	}
}

func TestSync_NotesArgumentsInACommandSkillUnderOnUnsupportedError(t *testing.T) {
	for _, target := range []string{"codex", "copilot"} {
		t.Run(target, func(t *testing.T) {
			notes := commandsAsSkillsProject(t, target)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\non-unsupported: error\n")
			writeFile(t, filepath.Join(".agnostic-ai", "commands", "fix.md"), "Fix issue $1.\n")

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("argument syntax in a command written as a skill must not fail sync: %v\n%s", err, out)
			}
			for _, want := range []string{"`$ARGUMENTS`", "`$N`"} {
				if !strings.Contains(notes.String(), want) {
					t.Errorf("want a note for %s, got:\n%s", want, notes.String())
				}
			}
		})
	}
}

func TestSync_NamesOnlyThePathForSyntaxInACommandWithoutFrontmatter(t *testing.T) {
	notes := commandsAsSkillsProject(t, "copilot")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "fix.md"), "Fix issue $1.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	path := filepath.Join(".agnostic-ai", "commands", "fix.md")
	if !strings.Contains(notes.String(), path) || strings.Contains(notes.String(), path+":") {
		t.Errorf("want the note to name %s without a line past the body, got:\n%s", path, notes.String())
	}
}

func TestSync_KeepsACommandSkillUserRunWhenTheCommandAllowsTheModel(t *testing.T) {
	commandsAsSkillsProject(t, "copilot")
	writeFile(t, filepath.Join(".agnostic-ai", "commands", "review.md"), "---\nname: review\ndisable-model-invocation: false\n---\n\nReview.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(".github", "skills", "review", "SKILL.md")); !strings.Contains(got, "disable-model-invocation: true") {
		t.Errorf("a command written as a skill must stay user-run:\n%s", got)
	}
}

func TestLint_ReportsArgumentsInACommandCodexWritesAsASkill(t *testing.T) {
	commandsAsSkillsProject(t, "codex")

	out, _ := runCLI(t, "lint")
	if !strings.Contains(out, "LINT019") || !strings.Contains(out, "codex reads $ARGUMENTS as plain text") {
		t.Errorf("want LINT019 for codex, got:\n%s", out)
	}
}

func TestSync_LeavesAHandWrittenSkillOfTheCommandNameAlone(t *testing.T) {
	for _, c := range []struct{ target, path string }{
		{"codex", filepath.Join(".agents", "skills", "review", "SKILL.md")},
		{"copilot", filepath.Join(".agents", "skills", "review", "SKILL.md")},
		{"copilot", filepath.Join(".github", "skills", "review", "SKILL.md")},
	} {
		t.Run(c.target+"/"+filepath.ToSlash(c.path), func(t *testing.T) {
			notes := commandsAsSkillsProject(t, c.target)
			const own = "---\nname: review\ndescription: Mine.\n---\n\nMy own review.\n"
			writeFile(t, c.path, own)

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if got := readFile(t, c.path); got != own {
				t.Errorf("sync replaced the hand-written skill:\n%s", got)
			}
			if want := "a skill sync did not write already has its folder: review"; !strings.Contains(notes.String(), want) {
				t.Errorf("want note %q, got:\n%s", want, notes.String())
			}
		})
	}
}

func TestSync_NotesFieldsCopilotDropsInASharedSkillsFolder(t *testing.T) {
	notes := commandsAsSkillsProject(t, "codex, copilot")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, copilot]\noutputs:\n  copilot:\n    skills-dir: .agents/skills\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !regexp.MustCompile("`disable-model-invocation` on 1 skill has no effect on [a-z, ]*copilot").MatchString(notes.String()) {
		t.Errorf("want the dropped field noted on copilot, got:\n%s", notes.String())
	}
}

func TestLint_SkipsCopilotForACommandItDoesNotGet(t *testing.T) {
	commandsAsSkillsProject(t, "codex, copilot")
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, copilot]\noutputs:\n  copilot:\n    skills-dir: .agents/skills\n")

	out, _ := runCLI(t, "lint")
	if strings.Contains(out, "copilot reads $ARGUMENTS") || strings.Contains(out, "codex, copilot read $ARGUMENTS") {
		t.Errorf("copilot gets no command here, got:\n%s", out)
	}
}

// Zed, Windsurf, and Crush each document a SKILL.md field that keeps a
// skill out of the model's reach, so a command reaches them as a skill.
func TestSync_WritesACommandAsAUserRunSkillWithTheToolsOwnField(t *testing.T) {
	cases := map[string]struct {
		want, absent []string
	}{
		"zed":      {want: []string{"disable-model-invocation: true"}, absent: []string{"triggers", "user-invocable"}},
		"windsurf": {want: []string{"triggers:\n  - user\n"}, absent: []string{"disable-model-invocation", "user-invocable"}},
		"crush":    {want: []string{"disable-model-invocation: true", "user-invocable: true"}, absent: []string{"triggers"}},
	}
	for target, c := range cases {
		t.Run(target, func(t *testing.T) {
			notes := commandsAsSkillsProject(t, target)

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			got := readFile(t, filepath.Join(".agents", "skills", "review", "SKILL.md"))
			for _, want := range append([]string{"name: review", "Run it only when the user calls it by name", "Review $ARGUMENTS."}, c.want...) {
				if !strings.Contains(got, want) {
					t.Errorf("want %q in the %s skill:\n%s", want, target, got)
				}
			}
			for _, absent := range c.absent {
				if strings.Contains(got, absent) {
					t.Errorf("%s reads no %q:\n%s", target, absent, got)
				}
			}
			if want := "1 command reaches " + target + " but not as a command (written as a skill the user runs by name)"; !strings.Contains(notes.String(), want) {
				t.Errorf("want note %q, got:\n%s", want, notes.String())
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}

// Zed and Windsurf write the same .agents/skills folder with different
// fields, so neither can keep a command user-run there.
func TestSync_LeavesCommandsOffToolsSharingASkillsFolder(t *testing.T) {
	notes := commandsAsSkillsProject(t, "zed, windsurf")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agents", "skills", "review")); !os.IsNotExist(err) {
		t.Errorf("no command skill in a shared folder, stat err = %v", err)
	}
	ship := readFile(t, filepath.Join(".agents", "skills", "ship", "SKILL.md"))
	if strings.Contains(ship, "triggers") || strings.Contains(ship, "disable-model-invocation") {
		t.Errorf("a shared SKILL.md carries no tool's own field:\n%s", ship)
	}
	for _, target := range []string{"windsurf", "zed"} {
		if !regexp.MustCompile("`disable-model-invocation` on 1 skill has no effect on [a-z, ]*" + target).MatchString(notes.String()) {
			t.Errorf("want the dropped field noted on %s, got:\n%s", target, notes.String())
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check: %v\n%s", err, out)
	}
}

// An x-<target> value is the author's choice for that tool and wins.
func TestSync_AnExplicitTargetValueWinsOverTheUserOnlyField(t *testing.T) {
	commandsAsSkillsProject(t, "windsurf")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\nx-windsurf:\n  triggers: [user, model]\n---\n\nShip it.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(".agents", "skills", "ship", "SKILL.md")); !strings.Contains(got, "triggers:\n  - user\n  - model\n") {
		t.Errorf("want the x-windsurf triggers:\n%s", got)
	}
}

// A user-only skill imports back to the portable field, so sync writes
// the same file again and other tools keep it user-only too.
func TestImport_ReadsTheToolsUserOnlyFieldBackAsDisableModelInvocation(t *testing.T) {
	for _, target := range []string{"windsurf", "crush", "zed"} {
		t.Run(target, func(t *testing.T) {
			commandsAsSkillsProject(t, target)
			const spec = "---\nname: ship\ndescription: Ships.\nlicense: MIT\ndisable-model-invocation: true\n---\n\nShip it.\n"
			writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), spec)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if err := os.RemoveAll(filepath.Join(".agnostic-ai", "skills", "ship")); err != nil {
				t.Fatal(err)
			}

			importCapturing(t, target)

			got := readFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"))
			meta, _ := splitMdcFrontmatter([]byte(got))
			if meta["disable-model-invocation"] != true || meta["license"] != "MIT" {
				t.Errorf("want disable-model-invocation and license back:\n%s", got)
			}
			for _, native := range []string{"triggers", "user-invocable", "x-" + target} {
				if _, ok := meta[native]; ok {
					t.Errorf("want no %s in the spec:\n%s", native, got)
				}
			}
			if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "review")); !os.IsNotExist(err) {
				t.Errorf("import must leave the command skill to the command spec, stat err = %v", err)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check after import: %v\n%s", err, out)
			}
		})
	}
}

// A command is run by the user, so an x-<target> field cannot hand its
// skill to the model.
func TestSync_ACommandsTargetFieldCannotLetTheModelStartIt(t *testing.T) {
	cases := map[string]struct{ override, want string }{
		"windsurf": {"x-windsurf:\n  triggers: [user, model]\n", "triggers:\n  - user\n"},
		"zed":      {"x-zed:\n  disable-model-invocation: false\n", "disable-model-invocation: true"},
		"crush":    {"x-crush:\n  disable-model-invocation: false\n  user-invocable: false\n", "user-invocable: true"},
	}
	for target, c := range cases {
		t.Run(target, func(t *testing.T) {
			commandsAsSkillsProject(t, target)
			writeFile(t, filepath.Join(".agnostic-ai", "commands", "review.md"), "---\ndescription: Review a change.\n"+c.override+"---\n\nReview $ARGUMENTS.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			got := readFile(t, filepath.Join(".agents", "skills", "review", "SKILL.md"))
			if !strings.Contains(got, c.want) || strings.Contains(got, "model\n") || strings.Contains(got, ": false") {
				t.Errorf("want %q and no model-startable value:\n%s", c.want, got)
			}
		})
	}
}

// A skill made user-only for one tool through x-<target> still gets that
// tool's own field.
func TestSync_AUserOnlyValueUnderTheTargetGetsTheToolsField(t *testing.T) {
	cases := map[string]string{"windsurf": "triggers:\n  - user\n", "crush": "user-invocable: true"}
	for target, want := range cases {
		t.Run(target, func(t *testing.T) {
			commandsAsSkillsProject(t, target)
			writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nx-"+target+":\n  disable-model-invocation: true\n---\n\nShip it.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if got := readFile(t, filepath.Join(".agents", "skills", "ship", "SKILL.md")); !strings.Contains(got, want) {
				t.Errorf("want %q:\n%s", want, got)
			}
		})
	}
}

// A native Crush skill with disable-model-invocation alone is hidden from
// the user too, and import keeps it hidden after the next sync.
func TestImport_KeepsACrushSkillHiddenFromTheUser(t *testing.T) {
	commandsAsSkillsProject(t, "crush")
	const native = "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\n---\n\nShip it.\n"
	writeFile(t, filepath.Join(".agents", "skills", "ship", "SKILL.md"), native)

	importCapturing(t, "crush")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	got := readFile(t, filepath.Join(".agents", "skills", "ship", "SKILL.md"))
	if strings.Contains(got, "user-invocable: true") || !strings.Contains(got, "disable-model-invocation: true") {
		t.Errorf("want the skill still hidden from the command palette:\n%s", got)
	}
}

// Tightening a native Windsurf skill to triggers: [user] survives import
// over a spec that allowed the model, and the next sync.
func TestImport_AWindsurfSkillTightenedToTheUserStaysThatWay(t *testing.T) {
	commandsAsSkillsProject(t, "windsurf")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nx-windsurf:\n  triggers: [user, model]\n---\n\nShip it.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "  - user\n  - model\n", "  - user\n", 1))

	if out, err := runCLI(t, "import", "windsurf", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, native); strings.Contains(got, "model") || !strings.Contains(got, "triggers:\n  - user\n") {
		t.Errorf("want the skill kept user-only:\n%s", got)
	}
}

// Another x-windsurf key brings the override block back on import, but
// not its stale triggers.
func TestImport_AWindsurfSkillTightenedToTheUserStaysThatWayBesideOtherOverrides(t *testing.T) {
	commandsAsSkillsProject(t, "windsurf")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nx-windsurf:\n  description: Ships on Devin.\n  triggers: [user, model]\n---\n\nShip it.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "  - user\n  - model\n", "  - user\n", 1))

	if out, err := runCLI(t, "import", "windsurf", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	got := readFile(t, native)
	if strings.Contains(got, "model") || !strings.Contains(got, "triggers:\n  - user\n") || !strings.Contains(got, "Ships on Devin.") {
		t.Errorf("want the skill kept user-only with its description override:\n%s", got)
	}
}

// Showing a hidden Crush skill in the palette from the tool survives
// import over a spec that hid it, and the next sync.
func TestImport_ACrushSkillShownToTheUserStaysThatWay(t *testing.T) {
	commandsAsSkillsProject(t, "crush")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\nx-crush:\n  user-invocable: false\n---\n\nShip it.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "user-invocable: false", "user-invocable: true", 1))

	if out, err := runCLI(t, "import", "crush", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, native); !strings.Contains(got, "user-invocable: true") {
		t.Errorf("want the skill kept in the command palette:\n%s", got)
	}
}

// Dropping an anchored trigger list keeps the spec readable where an
// alias reused it.
func TestImport_DroppingAnAnchoredWindsurfTriggerKeepsItsAliases(t *testing.T) {
	commandsAsSkillsProject(t, "windsurf")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nx-windsurf:\n  triggers: &who [user, model]\nmetadata:\n  who: *who\n---\n\nShip it.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "  - user\n  - model\n", "  - user\n", 1))

	if out, err := runCLI(t, "import", "windsurf", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md")); strings.Contains(got, "*who") {
		t.Errorf("want no dangling alias in the spec:\n%s", got)
	}
}

// Letting the model start a skill again from the tool survives import
// when the native file still states the tool's own policy, for that tool
// only: other tools keep the restriction.
func TestImport_LoosenedNativePolicyReplacesThePortableRestriction(t *testing.T) {
	cases := map[string]struct{ from, to string }{
		"crush":    {"disable-model-invocation: true\n", ""},
		"windsurf": {"  - user\n", "  - user\n  - model\n"},
	}
	for target, c := range cases {
		t.Run(target, func(t *testing.T) {
			commandsAsSkillsProject(t, target)
			writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\ndisable-model-invocation: true\n---\n\nShip it.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
			writeFile(t, native, strings.Replace(readFile(t, native), c.from, c.to, 1))
			loosened := readFile(t, native)

			if out, err := runCLI(t, "import", target, "--overwrite"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if got := readFile(t, native); strings.Contains(got, "disable-model-invocation: true") || got != loosened && target == "windsurf" {
				t.Errorf("want the loosened policy kept, had:\n%s\ngot:\n%s", loosened, got)
			}
			spec := readFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"))
			if meta, _ := splitMdcFrontmatter([]byte(spec)); meta["disable-model-invocation"] != true {
				t.Errorf("one tool's file must not lift the restriction for every tool:\n%s", spec)
			}
		})
	}
}

// An x-crush block anchored and reused under another target stays
// readable when import drops its invocation fields.
func TestImport_DroppingFieldsFromAnAnchoredCrushBlockKeepsItsAliases(t *testing.T) {
	commandsAsSkillsProject(t, "crush")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "ship", "SKILL.md"), "---\nname: ship\ndescription: Ships.\nx-crush: &policy\n  disable-model-invocation: true\n  user-invocable: true\nx-zed: *policy\n---\n\nShip it.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	native := filepath.Join(".agents", "skills", "ship", "SKILL.md")
	writeFile(t, native, strings.Replace(readFile(t, native), "Ship it.", "Ship it now.", 1))
	if out, err := runCLI(t, "import", "crush", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync after import: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after import: %v\n%s", err, out)
	}
}

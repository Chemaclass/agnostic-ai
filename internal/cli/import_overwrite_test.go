package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	handSkill   = "---\nname: review\ndescription: Mine.\n---\nMine\n"
	nativeSkill = "---\nname: review\ndescription: Native.\n---\nNative\n"
)

// importOverwriteProject is a claude project whose hand-written review
// skill shares its name with a native one, next to a native skill the
// import would create.
func importOverwriteProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
	mustWriteFile(t, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip\n")
}

// Import replaced a hand-written spec of the same name without a word
// (#1620).
func TestImport_StopsBeforeReplacingAHandWrittenSkill(t *testing.T) {
	importOverwriteProject(t)

	_, err := runCLI(t, "import", "claude")

	if err == nil {
		t.Fatal("import replaced a hand-written skill without stopping")
	}
	for _, want := range []string{".agnostic-ai/skills/review/SKILL.md (from claude)", "agnostic-ai import claude --overwrite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
	if _, err := os.Stat(".agnostic-ai/skills/deploy/SKILL.md"); err == nil {
		t.Error("import wrote the deploy skill before stopping")
	}
}

func TestImport_StopsBeforeReplacingAnMCPServer(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"github":{"command":"npx","args":["github-mcp"]}}}`)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	mine := "command: docker\nargs: [run, github-mcp]\n"
	mustWriteFile(t, ".agnostic-ai/mcps/github.yaml", mine)

	_, err := runCLI(t, "import", "claude")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/mcps/github.yaml (from claude)") {
		t.Fatalf("import did not stop on the edited MCP server: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/mcps/github.yaml"); got != mine {
		t.Errorf("github.yaml = %q, want it untouched", got)
	}
}

func TestImport_LeavesAnIdenticalSpecAlone(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", nativeSkill)
	stamp := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(".agnostic-ai/skills/review/SKILL.md", stamp, stamp); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import stopped on a spec it would not change: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("a second import stopped: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != nativeSkill {
		t.Errorf("review skill = %q, want %q", got, nativeSkill)
	}
	info, err := os.Stat(".agnostic-ai/skills/review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Error("import rewrote an identical skill")
	}
}

func TestImport_ProtectsASourceDirAtTheProjectRoot(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  rules: .\n")
	const mine = "---\ndescription: Mine.\n---\nMine\n"
	mustWriteFile(t, "style.md", mine)
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Native.\n---\nNative\n")

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Errorf("import did not protect the root source: %v", err)
	}
	if got := readFile(t, "style.md"); got != mine {
		t.Errorf("root source = %q, want %q", got, mine)
	}
}

func TestImport_OverwriteReplacesTheSpec(t *testing.T) {
	importOverwriteProject(t)

	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import --overwrite: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want the native one", got)
	}
}

// Only files there before the run count: a spec one source writes and a
// later source replaces is today's last-wins import.
func TestImport_SourcesInOneRunStillReplaceEachOther(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip\n")
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Claude style.\n---\nUse tabs.\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	if out, err := runCLI(t, "import", "claude", "cursor"); err != nil {
		t.Fatalf("import claude cursor: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/rules/style.md"); !strings.Contains(got, "Use spaces.") {
		t.Errorf("style rule = %q, want cursor's, imported last", got)
	}
}

func TestImport_OnlyTheLastSourcesContentCanBeReimported(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"fs":{"command":"npx"}}}`)
	mustWriteFile(t, ".codex/config.toml", "[mcp_servers.fs]\ncommand = \"uvx\"\n")
	if out, err := runCLI(t, "import", "claude", "codex"); err != nil {
		t.Fatalf("import claude codex: %v\n%s", err, out)
	}
	want := readFile(t, ".agnostic-ai/mcps/fs.yaml")

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Errorf("import replaced the last source's spec: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/mcps/fs.yaml"); got != want {
		t.Errorf("MCP server = %q, want the last source's content %q", got, want)
	}
}

func TestImport_StopsOnASpecSyncDidNotWrite(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		unmanaged bool
	}{
		{name: "keep-edits", flags: []string{"--keep-edits"}},
		{name: "json-keep-edits", flags: []string{"--json", "--keep-edits"}},
		{name: "unmanaged", unmanaged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
			runSyncOK(t)
			changed := strings.Replace(handSkill, "Mine\n", "Changed in the source\n", 1)
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", changed)
			mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
			if tc.unmanaged {
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsync:\n  unmanaged: [.claude/skills/review/SKILL.md]\n")
			}
			if out, err := runCLI(t, append([]string{"sync"}, tc.flags...)...); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if got := readFile(t, ".claude/skills/review/SKILL.md"); got != nativeSkill {
				t.Fatalf("sync did not keep the native edit: %q", got)
			}

			_, err := runCLI(t, "import", "claude")

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect the source that sync skipped: %v", err)
			}
			if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != changed {
				t.Errorf("source = %q, want source edit %q", got, changed)
			}
		})
	}
}

func TestImport_StopsOnASpecWithNoNativeOutput(t *testing.T) {
	for _, tc := range []struct {
		target string
		spec   string
		mine   string
		native string
		data   string
	}{
		{target: "codex", spec: ".agnostic-ai/commands/review.md", mine: "---\nname: review\ndescription: Mine.\n---\nOriginal\n", native: ".codex/prompts/review.md", data: "---\nname: review\ndescription: Native.\n---\nReplacement\n"},
		{target: "zed", spec: ".agnostic-ai/hooks/review.yaml", mine: "name: review\nevent: OnDemand\ncommand: original\n", native: ".zed/tasks.json", data: `[{"label":"review","command":"replacement"}]`},
		{target: "warp", spec: ".agnostic-ai/agents/review.md", mine: "---\nname: review\ndescription: Mine.\n---\nOriginal\n", native: ".warp/workflows/review.yaml", data: "name: review\ndescription: Native.\ncommand: replacement\n"},
		{target: "amp", spec: ".agnostic-ai/agents/review.md", mine: "---\nname: review\ndescription: Mine.\n---\nOriginal\n", native: ".agents/commands/review.md", data: "---\nname: review\ndescription: Native.\n---\nReplacement\n"},
		{target: "factory", spec: ".agnostic-ai/agents/review.md", mine: "---\nname: review\ndescription: Mine.\n---\n", native: ".factory/droids/review.md", data: "---\nname: review\ndescription: Native.\n---\nReplacement\n"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			mustWriteFile(t, tc.spec, tc.mine)
			runSyncOK(t)
			mustWriteFile(t, tc.native, tc.data)

			_, err := runCLI(t, "import", tc.target)

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect a spec with no native output: %v", err)
			}
			if got := readFile(t, tc.spec); got != tc.mine {
				t.Errorf("source = %q, want %q", got, tc.mine)
			}
		})
	}
}

func TestImport_StopsOnASkillAssetTheTargetDidNotCopy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		asset string
		meta  string
		link  bool
	}{
		{name: "codex-agent", asset: "agents/openai.yaml"},
		{name: "codex-only-script", asset: "scripts/tool.sh", meta: "x-codex:\n  assets: [scripts]\n"},
		{name: "linked-script", asset: "tool.sh", link: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link && runtime.GOOS == "windows" {
				t.Skip("symlinks need privileges on Windows")
			}
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review.\n"+tc.meta+"---\nReview.\n")
			asset := filepath.Join(".agnostic-ai/skills/review", tc.asset)
			if tc.link {
				mustWriteFile(t, "tool.sh", "Original\n")
				if err := os.Symlink("../../../tool.sh", asset); err != nil {
					t.Fatal(err)
				}
			} else {
				mustWriteFile(t, asset, "Original\n")
			}
			runSyncOK(t)
			mustWriteFile(t, filepath.Join(".claude/skills/review", tc.asset), "Replacement\n")

			_, err := runCLI(t, "import", "claude")

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect the omitted asset: %v", err)
			}
			if got := readFile(t, asset); got != "Original\n" {
				t.Errorf("asset = %q, want original content", got)
			}
		})
	}
}

func TestImport_StopsOnSettingsThatContributeNoNativeKeys(t *testing.T) {
	for _, tc := range []struct {
		target string
		native string
		data   string
	}{
		{target: "factory", native: ".factory/settings.json", data: `{"model":"native-model"}`},
		{target: "opencode", native: "opencode.json", data: `{"model":"native-model"}`},
		{target: "junie", native: ".junie/config.json", data: `{"model":"native-model"}`},
		{target: "qoder", native: ".qoder/settings.json", data: `{"model":{"name":"native-model"}}`},
		{target: "gemini", native: ".gemini/settings.json", data: `{"model":{"name":"native-model"}}`},
		{target: "copilot", native: ".github/copilot/settings.json", data: `{"model":"native-model"}`},
		{target: "kilo", native: "kilo.jsonc", data: `{"model":"native-model"}`},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			path := ".agnostic-ai/settings/" + tc.target + ".yaml"
			const original = "model:\n  claude: sonnet\n"
			mustWriteFile(t, path, original)
			runSyncOK(t)
			mustWriteFile(t, tc.native, tc.data)

			_, err := runCLI(t, "import", tc.target)

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect settings with no native keys: %v", err)
			}
			if got := readFile(t, path); got != original {
				t.Errorf("settings = %q, want %q", got, original)
			}
		})
	}
}

func TestImport_RollbackRestoresHardLinkedDestinations(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const original = "---\ndescription: Original.\n---\nOriginal\n"
	mustWriteFile(t, ".agnostic-ai/rules/a.md", original)
	if err := os.Link(".agnostic-ai/rules/a.md", ".agnostic-ai/rules/b.md"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".claude/rules/a.md", "---\ndescription: A.\n---\nNative A\n")
	mustWriteFile(t, ".claude/rules/b.md", "---\ndescription: B.\n---\nNative B\n")

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Errorf("import did not stop: %v", err)
	}
	for _, path := range []string{".agnostic-ai/rules/a.md", ".agnostic-ai/rules/b.md"} {
		if got := readFile(t, path); got != original {
			t.Errorf("%s = %q, want original content", path, got)
		}
	}
}

func TestImport_PreviewKeepsHardLinkIdentity(t *testing.T) {
	for _, flags := range [][]string{nil, {"--dry-run"}, {"--dry-run", "--diff"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			const original = "---\ndescription: Original.\n---\nOriginal\n"
			mustWriteFile(t, ".agnostic-ai/rules/a.md", original)
			if err := os.Link(".agnostic-ai/rules/a.md", ".agnostic-ai/rules/b.md"); err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, ".claude/rules/a.md", "---\ndescription: Replacement.\n---\nReplacement\n")
			mustWriteFile(t, ".claude/rules/b.md", original)

			if out, err := runCLI(t, append([]string{"import", "claude"}, flags...)...); err != nil {
				t.Errorf("import rejected unchanged linked destinations: %v\n%s", err, out)
			}
			for _, path := range []string{".agnostic-ai/rules/a.md", ".agnostic-ai/rules/b.md"} {
				if got := readFile(t, path); got != original {
					t.Errorf("%s = %q, want original content", path, got)
				}
			}
		})
	}
}

func TestImport_StopsOnSettingsWithNoNativeEffortOrPermissions(t *testing.T) {
	for _, tc := range []struct {
		target string
		file   string
		mine   string
		native string
		data   string
	}{
		{target: "claude", file: "claude", mine: "model:\n  codex: gpt-5.5\n", native: ".claude/settings.json", data: `{"effortLevel":"high"}`},
		{target: "codex", file: "codex", mine: "model:\n  claude: sonnet\n", native: ".codex/config.toml", data: "model_reasoning_effort = \"high\"\n"},
		{target: "windsurf", file: "permissions", mine: "model: native-model\n", native: ".devin/config.json", data: `{"permissions":{"allow":["read"]}}`},
		{target: "augment", file: "permissions-augment", mine: "model: native-model\n", native: ".augment/settings.json", data: `{"toolPermissions":[{"toolName":"read","permission":{"type":"allow"}}]}`},
	} {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			path := ".agnostic-ai/settings/" + tc.file + ".yaml"
			mustWriteFile(t, path, tc.mine)
			runSyncOK(t)
			mustWriteFile(t, tc.native, tc.data)

			_, err := runCLI(t, "import", tc.target)

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect settings with no native output: %v", err)
			}
			if got := readFile(t, path); got != tc.mine {
				t.Errorf("settings = %q, want %q", got, tc.mine)
			}
		})
	}
}

func TestSync_RecordsSettingsWithOnlyNativeMCPServers(t *testing.T) {
	for _, target := range []string{"augment", "gemini", "qoder"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			path := ".agnostic-ai/settings/" + target + ".yaml"
			mustWriteFile(t, path, "x-"+target+":\n  mcpServers:\n    docs:\n      command: docs-server\n")

			runSyncOK(t)

			state := readStateFile(".")
			if got := state.SpecFileSums[path].Targets; !slices.Contains(got, target) {
				t.Errorf("settings holders = %v, want %s", got, target)
			}
		})
	}
}

func TestImport_ProtectsItsConfiguredSourceDirBeforeImportWrites(t *testing.T) {
	for _, flags := range [][]string{nil, {"--dry-run"}, {"--dry-run", "--diff"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			const original = "version: 1\ntargets: [zed]\nsources:\n  hooks: .\n"
			mustWriteFile(t, "agnostic-ai.yaml", original)
			mustWriteFile(t, ".zed/tasks.json", `[{"label":"agnostic-ai","command":"replacement"}]`)

			_, err := runCLI(t, append([]string{"import", "zed"}, flags...)...)

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Errorf("import did not protect its original source dir: %v", err)
			}
			if got := readFile(t, "agnostic-ai.yaml"); got != original {
				t.Errorf("config = %q, want %q", got, original)
			}
		})
	}
}

// A dry-run fails as the real run would, instead of counting the files
// it would write.
func TestImport_DryRunStopsOnAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)
	var err error

	out := captureStdout(t, func() {
		_, err = runCLI(t, "import", "claude", "--dry-run")
	})

	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), ".agnostic-ai/skills/review/SKILL.md (from claude)") {
		t.Fatalf("import --dry-run did not stop on the replaced spec: %v", err)
	}
	if strings.Contains(out, "would be written") {
		t.Errorf("dry-run counted files as written while the import would stop:\n%s", out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("dry-run changed the review skill: %q", got)
	}
}

func TestImport_DiffStopsOnAnExistingSpecItWouldReplace(t *testing.T) {
	importOverwriteProject(t)
	var err error

	captureStdout(t, func() {
		_, err = runCLI(t, "import", "claude", "--dry-run", "--diff")
	})

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import --dry-run --diff did not stop on the replaced spec: %v", err)
	}
}

func TestImport_DryRunWithOverwriteListsTheReplacedSpec(t *testing.T) {
	importOverwriteProject(t)

	out := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--overwrite"); err != nil {
			t.Errorf("import --dry-run --overwrite: %v", err)
		}
	})

	if !strings.Contains(out, "replaces .agnostic-ai/skills/review/SKILL.md") {
		t.Errorf("dry-run did not name the replaced spec:\n%s", out)
	}
}

func TestInitFrom_StopsBeforeReplacingAHandWrittenSkill(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "init", "--from", "claude")

	if err == nil || !strings.Contains(err.Error(), "agnostic-ai import claude --overwrite") {
		t.Fatalf("init --from did not stop with the overwrite remedy: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

func TestUse_StopsBeforeReplacingAHandWrittenRule(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	mine := "---\ndescription: Mine.\n---\nUse tabs.\n"
	mustWriteFile(t, ".agnostic-ai/rules/style.md", mine)
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	_, err := runCLI(t, "use", "cursor")

	if err == nil {
		t.Fatal("use replaced a hand-written rule without stopping")
	}
	for _, want := range []string{".agnostic-ai/rules/style.md (from cursor)", "agnostic-ai import cursor --overwrite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != mine {
		t.Errorf("style rule = %q, want it untouched", got)
	}
	cfg, lerr := config.Load(".")
	if lerr != nil {
		t.Fatal(lerr)
	}
	for _, target := range cfg.Targets {
		if target == "cursor" {
			t.Errorf("use left cursor in targets after stopping: %v", cfg.Targets)
		}
	}
}

// Copilot and kiro write an agent's tools in another shape, so a sync
// and import with no edit in between change the spec bytes. The spec
// still matches what the last sync rendered, so nothing stops.
func TestImport_UneditedRoundTripNeedsNoFlag(t *testing.T) {
	const agent = "---\nname: reviewer\ndescription: Review the diff.\ntools: [Read, Grep]\neffort: high\n---\n\nReview what changed.\n"
	for _, target := range []string{"copilot", "kiro"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", agent)
			runSyncOK(t)

			if out, err := runCLI(t, "import", target); err != nil {
				t.Fatalf("import %s after sync: %v\n%s", target, err, out)
			}
		})
	}
}

// claudeSyncedReviewSkill syncs a review skill to claude, then edits the
// native copy, the documented way to bring a native edit back.
func claudeSyncedReviewSkill(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)
}

func TestImport_ReimportsANativeEditWithoutAFlag(t *testing.T) {
	claudeSyncedReviewSkill(t)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import after a native edit: %v\n%s", err, out)
	}

	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want the native edit", got)
	}
}

func TestImport_StopsWhenTheSpecChangedSinceTheLastSync(t *testing.T) {
	claudeSyncedReviewSkill(t)
	edited := strings.Replace(handSkill, "Mine\n", "Mine, edited\n", 1)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", edited)

	_, err := runCLI(t, "import", "claude")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/skills/review/SKILL.md (from claude)") {
		t.Fatalf("import did not stop on a spec edited since the last sync: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != edited {
		t.Errorf("review skill = %q, want the edit kept", got)
	}
}

// A synced spec is exempt only for a tool sync wrote it for: a rule
// scoped to claude never reached cursor, so cursor's own rule of the
// same name would replace it unseen.
func TestImport_StopsOnASyncedSpecTheSourceNeverReceived(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mine := "---\nname: style\ndescription: Mine.\ntargets: [claude]\n---\nUse tabs.\n"
	mustWriteFile(t, ".agnostic-ai/rules/style.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")

	_, err := runCLI(t, "import", "cursor")

	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/rules/style.md (from cursor; now holds what sync wrote for claude)") {
		t.Fatalf("import did not stop on a spec cursor never received: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != mine {
		t.Errorf("style rule = %q, want it untouched", got)
	}
}

// Codex was the only target at the last sync, so the skill never reached
// claude: claude's own skill of the same name would replace it unseen.
func TestUse_StopsOnASyncedSpecTheNewToolNeverReceived(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "use", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace || !strings.Contains(err.Error(), "agnostic-ai import claude --overwrite") {
		t.Fatalf("use did not stop on a spec claude never received: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != handSkill {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

func TestImport_StopsOnASpecScopedToAnotherTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mine := "---\nname: review\ndescription: Mine.\ntargets: [codex]\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import claude did not stop on a skill scoped to codex: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != mine {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

// A comment is an edit: the spec no longer holds the bytes sync rendered.
func TestImport_StopsOnACommentAddedSinceTheLastSync(t *testing.T) {
	claudeSyncedReviewSkill(t)
	edited := strings.Replace(handSkill, "description: Mine.\n", "description: Mine.\n# keep the review short\n", 1)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", edited)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import did not stop on a spec with a new comment: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != edited {
		t.Errorf("review skill = %q, want the comment kept", got)
	}
}

// mcpImportedFromClaude imports claude's fs server, next to codex's own
// fs server with a different command.
func mcpImportedFromClaude(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"fs":{"command":"npx","args":["fs-mcp"]}}}`)
	mustWriteFile(t, ".codex/config.toml", "[mcp_servers.fs]\ncommand = \"uvx\"\nargs = [\"fs-mcp\"]\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}
}

// What an import of claude wrote, and nothing changed since, is claude's
// own config: a later claude import brings back an edit made there.
func TestImport_ReplacesWhatAnEarlierImportOfTheSameToolWrote(t *testing.T) {
	mcpImportedFromClaude(t)
	mustWriteFile(t, ".mcp.json", `{"mcpServers":{"fs":{"command":"bunx","args":["fs-mcp"]}}}`)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import of claude: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/mcps/fs.yaml"); !strings.Contains(got, "bunx") {
		t.Errorf("fs.yaml = %q, want claude's edited server", got)
	}
}

// Codex never held claude's server: replacing it would make the next
// sync write codex's server into claude's .mcp.json.
func TestImport_StopsOnWhatAnotherToolsImportWrote(t *testing.T) {
	mcpImportedFromClaude(t)

	_, err := runCLI(t, "import", "codex")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace ||
		!strings.Contains(err.Error(), ".agnostic-ai/mcps/fs.yaml (from codex; now holds what import claude wrote)") {
		t.Fatalf("import codex did not stop on claude's server: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/mcps/fs.yaml"); !strings.Contains(got, "npx") {
		t.Errorf("fs.yaml = %q, want claude's server kept", got)
	}
}

func TestImport_ReimportsAChangedSettingsOverlay(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/settings.json", `{"statusLine": {"type": "command", "command": "first"}}`+"\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	mustWriteFile(t, ".claude/settings.json", `{"statusLine": {"type": "command", "command": "second"}}`+"\n")

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import after a settings change: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/overlays/claude.settings.json"); !strings.Contains(got, "second") {
		t.Errorf("overlay = %q, want the new statusLine", got)
	}
}

// The guard restores what a stopped run wrote instead of running the
// import in a copy first, so a real import never copies the project: it
// works with no usable temp directory.
func TestImport_RealRunDoesNotCopyTheProject(t *testing.T) {
	importOverwriteProject(t)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import without a temp dir: %v", err)
	}
	if _, err := os.Stat(".agnostic-ai/skills/deploy"); !os.IsNotExist(err) {
		t.Errorf("the stopped import left the deploy skill folder: %v", err)
	}
	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import --overwrite without a temp dir: %v\n%s", err, out)
	}
}

// The singular `target:` key scopes a spec as `targets:` does.
func TestImport_StopsOnASpecWithATargetKeyForAnotherTool(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mine := "---\nname: review\ndescription: Mine.\ntarget: codex\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", mine)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import claude did not stop on a skill with target: codex: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != mine {
		t.Errorf("review skill = %q, want it untouched", got)
	}
}

// A skill an earlier import wrote comes back from a later native edit.
func TestImport_ReimportsASkillAnEarlierImportWrote(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", nativeSkill)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("first import: %v\n%s", err, out)
	}
	edited := strings.Replace(nativeSkill, "Native\n", "Native, edited\n", 1)
	mustWriteFile(t, ".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\n---\nShip, edited\n")
	mustWriteFile(t, ".claude/skills/review/SKILL.md", edited)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import of an imported skill: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/deploy/SKILL.md"); !strings.Contains(got, "Ship, edited") {
		t.Errorf("deploy skill = %q, want the native edit", got)
	}
}

// Two tools' own rules of one name: the second import may not replace
// what the first one wrote.
func TestImport_StopsOnARuleAnotherToolsImportWrote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".claude/rules/style.md", "---\ndescription: Claude style.\n---\nUse tabs.\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import claude: %v\n%s", err, out)
	}
	claudeRule := readFile(t, ".agnostic-ai/rules/style.md")

	_, err := runCLI(t, "import", "cursor")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace ||
		!strings.Contains(err.Error(), ".agnostic-ai/rules/style.md (from cursor; now holds what import claude wrote)") {
		t.Fatalf("import cursor did not stop on claude's rule: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/style.md"); got != claudeRule {
		t.Errorf("style rule = %q, want claude's kept", got)
	}
}

// An import that finds a synced spec already in its tool's files records
// that tool too, so the tool's next edit comes back without a flag.
func TestImport_RecordsAToolThatAlreadyHeldASyncedSpec(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", handSkill)
	runSyncOK(t)
	mustWriteFile(t, ".claude/skills/review/SKILL.md", handSkill)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import of an identical skill: %v\n%s", err, out)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("re-import after a claude edit: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); !strings.Contains(got, "Native") {
		t.Errorf("review skill = %q, want claude's edit", got)
	}
}

// import writes the state file to keep its records, but no sync ran: the
// first-sync picker and why's "run sync first" still apply.
func TestImport_StateFileItWritesDoesNotCountAsASync(t *testing.T) {
	importOverwriteProject(t)
	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(stateFilePath(".")); err != nil {
		t.Fatalf("import kept no record: %v", err)
	}

	if !shouldPromptTargetSelection(".", &config.Config{Targets: allTargetNames()}) {
		t.Error("the first-sync picker is skipped after an import with no sync")
	}
	if err := whyNotTrackedError("CLAUDE.md", "."); !strings.Contains(err.Error(), "Run `agnostic-ai sync` first") {
		t.Errorf("why does not say to sync first after an import: %v", err)
	}
}

const corruptState = "{not json\n"

// An unreadable ledger still counts as one (#1334), so import keeps it
// instead of writing a stub over it.
func TestImport_KeepsAStateFileThatDoesNotParse(t *testing.T) {
	importOverwriteProject(t)
	mustWriteFile(t, stateFilePath("."), corruptState)

	if out, err := runCLI(t, "import", "claude", "--overwrite"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if got := readFile(t, stateFilePath(".")); got != corruptState {
		t.Errorf("state file = %q, want it left as it was", got)
	}
	if ledgerMissing(".") {
		t.Error("the unreadable ledger no longer counts as one")
	}
}

func TestUse_RefusesToWriteOverAStateFileThatDoesNotParse(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".cursor/rules/style.mdc", "---\ndescription: Cursor style.\nalwaysApply: true\n---\nUse spaces.\n")
	mustWriteFile(t, stateFilePath("."), corruptState)

	_, err := runCLI(t, "use", "cursor")

	if err == nil || !strings.Contains(err.Error(), "fix or delete it, then run agnostic-ai use again") {
		t.Fatalf("use did not refuse the unreadable state file: %v", err)
	}
	if got := readFile(t, stateFilePath(".")); got != corruptState {
		t.Errorf("state file = %q, want it left as it was", got)
	}
}

// The signal handler exits without running defers, so it gives the
// streams back and removes the held files through releaseHeldOutput.
func TestWithHeldOutput_ReleaseRestoresStderrAndRemovesHeldFiles(t *testing.T) {
	stderr := os.Stderr
	var held []string
	_ = withHeldOutput(func() (bool, error) {
		held = []string{os.Stdout.Name(), os.Stderr.Name()}
		releaseHeldOutput()
		return false, nil
	})

	if os.Stderr != stderr {
		t.Error("stderr still points at the held file")
	}
	for _, p := range held {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("held file %s is still there: %v", p, err)
		}
	}
}

// A source directory configured as an absolute path is guarded like a
// relative one: the directory and a write into it compare as one key.
func TestReplacesSpec_GuardsAnAbsoluteSourceDir(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	skills := filepath.Join(dir, "specs", "skills")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  skills: "+filepath.ToSlash(skills)+"\n")
	spec := filepath.Join(skills, "review", "SKILL.md")
	mustWriteFile(t, spec, handSkill)

	for _, path := range []string{spec, "specs/skills/review/SKILL.md"} {
		e := importPreviewEntry{
			path: filepath.ToSlash(path), existed: true, replaced: true,
			before: []byte(handSkill), after: []byte(nativeSkill), sources: []string{"claude"},
		}
		if !replacesSpec(&e, importSpecDirs("."), nil) {
			t.Errorf("%s: a write into the absolute source dir does not count as replacing a spec", path)
		}
	}
}

// A skill folder linked out of the source still holds a spec: a write
// through the link counts as replacing it.
func TestImport_StopsBeforeReplacingASkillThroughALinkedFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "shared/review/SKILL.md", handSkill)
	if err := os.MkdirAll(".agnostic-ai/skills", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../shared/review", ".agnostic-ai/skills/review"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".claude/skills/review/SKILL.md", nativeSkill)

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("import did not stop on a skill behind a linked folder: %v", err)
	}
	if got := readFile(t, "shared/review/SKILL.md"); got != handSkill {
		t.Errorf("shared skill = %q, want it untouched", got)
	}
}

func TestImport_RollbackRestoresAFileWrittenThroughTwoPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const original = "---\ndescription: Original.\n---\nOriginal\n"
	mustWriteFile(t, ".agnostic-ai/rules/b.md", original)
	if err := os.Symlink("b.md", ".agnostic-ai/rules/a.md"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".claude/rules/a.md", "---\ndescription: A.\n---\nNative A\n")
	mustWriteFile(t, ".claude/rules/b.md", "---\ndescription: B.\n---\nNative B\n")

	_, err := runCLI(t, "import", "claude")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Errorf("import did not stop: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/b.md"); got != original {
		t.Errorf("rollback restored %q, want original %q", got, original)
	}
	if target, err := os.Readlink(".agnostic-ai/rules/a.md"); err != nil || target != "b.md" {
		t.Errorf("rollback changed the alias: target %q, error %v", target, err)
	}
}

func TestImportTransaction_RollbackRemovesNewFileWrittenThroughDirectoryAlias(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	external := t.TempDir()
	shared := filepath.Join(external, "shared")
	alias := filepath.Join(external, "alias")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := createImportSourceAlias(shared, alias); err != nil {
		t.Fatal(err)
	}
	viaAlias := filepath.Join(alias, "foo.md")
	viaTarget := filepath.Join(shared, "foo.md")
	txn := importTransaction{files: map[string]*savedImportFile{}}
	if err := txn.saveFile(viaAlias); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, viaAlias, "First import.\n")
	if err := txn.saveFile(viaTarget); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, viaTarget, "Second import.\n")
	entries, err := txn.entries(&importRecorder{writes: []importPlannedWrite{{path: viaAlias}, {path: viaTarget}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.existed {
			t.Errorf("new destination was marked as existing: %s", entry.path)
		}
	}
	if err := txn.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(viaTarget); !os.IsNotExist(err) {
		t.Errorf("rollback left a new file through its alias: %v", err)
	}
}

func TestImport_LeavesADanglingDestinationLinkAndItsTargetAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/rules/z.md", "---\ndescription: Mine.\n---\nMine\n")
	if err := os.Symlink("../../missing.md", ".agnostic-ai/rules/a.md"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".claude/rules/a.md", "---\ndescription: A.\n---\nNative A\n")
	mustWriteFile(t, ".claude/rules/z.md", "---\ndescription: Z.\n---\nNative Z\n")

	for _, flags := range [][]string{nil, {"--dry-run"}, {"--dry-run", "--diff"}} {
		_, err := runCLI(t, append([]string{"import", "claude"}, flags...)...)
		if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/rules/a.md") {
			t.Errorf("import %v did not name the dangling destination: %v", flags, err)
		}
		if target, err := os.Readlink(".agnostic-ai/rules/a.md"); err != nil || target != "../../missing.md" {
			t.Errorf("import %v changed the dangling link: target %q, error %v", flags, target, err)
		}
		if _, err := os.Stat("missing.md"); !os.IsNotExist(err) {
			t.Errorf("import %v created the dangling link's target: %v", flags, err)
		}
	}
}

func TestGuardedImport_InterruptDuringVerificationRestoresWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("interrupts and named pipes need Unix")
	}
	if os.Getenv("AGNOSTIC_AI_TEST_IMPORT_SIGNAL") == "1" {
		err := runGuardedImport(false, importOverwriteRemedy, func() error {
			importRecording.beginSource("claude")
			if err := importWriteFile(".agnostic-ai/rules/keep.md", []byte("Native\n"), 0o644); err != nil {
				return err
			}
			blocked := ".agnostic-ai/rules/blocked.md"
			if err := importWriteFile(blocked, []byte("Native\n"), 0o644); err != nil {
				return err
			}
			if err := os.Remove(blocked); err != nil {
				return err
			}
			if err := exec.Command("mkfifo", blocked).Run(); err != nil {
				return err
			}
			return os.WriteFile("verifying", nil, 0o644)
		})
		t.Fatalf("guard returned before an interrupt: %v", err)
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	mustWriteFile(t, ".agnostic-ai/rules/keep.md", "Original\n")
	cmd := exec.Command(os.Args[0], "-test.run=^TestGuardedImport_InterruptDuringVerificationRestoresWrites$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AGNOSTIC_AI_TEST_IMPORT_SIGNAL=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	deadline := time.After(10 * time.Second)
	for {
		if _, err := os.Stat("verifying"); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("child stopped before verification: %v", err)
		case <-deadline:
			t.Fatal("child never reached verification")
		case <-time.After(10 * time.Millisecond):
		}
	}
	// The FIFO keeps verification blocked after the importer returns.
	time.Sleep(100 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("child succeeded after an interrupt")
		}
	case <-deadline:
		t.Fatal("interrupted child did not stop")
	}
	if got := readFile(t, ".agnostic-ai/rules/keep.md"); got != "Original\n" {
		t.Errorf("interrupt left %q, want original content", got)
	}
	if _, err := os.Lstat(".agnostic-ai/rules/blocked.md"); !os.IsNotExist(err) {
		t.Errorf("interrupt left the new destination behind: %v", err)
	}
}

// A sync of one target keeps what an earlier sync wrote for the others,
// so cursor's own edit to a cursor-only rule still comes back. Codex
// would merge a skill rather than replace it, so cursor shows it.
func TestImport_PartialSyncKeepsWhatAnEarlierSyncWroteForOtherTools(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, ".agnostic-ai/rules/review.md", "---\nname: review\ndescription: Mine.\ntargets: [cursor]\n---\nMine\n")
	runSyncOK(t)
	if out, err := runCLI(t, "sync", "--only", "claude"); err != nil {
		t.Fatalf("sync --only claude: %v\n%s", err, out)
	}
	native := ".cursor/rules/review.mdc"
	mustWriteFile(t, native, strings.Replace(readFile(t, native), "Mine", "Mine, edited in cursor", 1))

	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import cursor after a partial sync: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/rules/review.md"); !strings.Contains(got, "edited in cursor") {
		t.Errorf("review rule = %q, want cursor's edit", got)
	}
}

// A target taken out of the config loses what sync wrote for it, so its
// own rule of the same name, written later, may not replace the spec.
func TestUse_StopsOnASpecSyncedForAToolSinceRemoved(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mine := "---\nname: r\ndescription: Mine.\n---\nMine\n"
	mustWriteFile(t, ".agnostic-ai/rules/r.md", mine)
	runSyncOK(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	runSyncOK(t)
	if err := os.RemoveAll(".cursor"); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, ".cursor/rules/r.mdc", "---\ndescription: Unrelated.\nalwaysApply: true\n---\nSomething else.\n")

	_, err := runCLI(t, "use", "cursor")

	if errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Fatalf("use cursor did not stop on a rule cursor no longer held: %v", err)
	}
	if got := readFile(t, ".agnostic-ai/rules/r.md"); got != mine {
		t.Errorf("rule = %q, want it untouched", got)
	}
}

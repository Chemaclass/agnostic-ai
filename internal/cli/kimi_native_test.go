package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportKimi_NativeRoundTrip(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n")
	for path, body := range map[string]string{
		".kimi-code/AGENTS.md":                  "# Project\n\nUse small changes.\n",
		"backend/.kimi-code/AGENTS.md":          "Keep explicit errors.\n",
		".kimi-code/agents/reviewer.md":         "---\nname: reviewer\ndescription: Review changes\ntools: []\ndisallowedTools: [Bash]\n---\nReview the patch.\n",
		".kimi-code/skills/check/SKILL.md":      "---\nname: check\ndescription: Check changes\ndisableModelInvocation: true\narguments: [path]\n---\nCheck the patch.\n",
		".kimi-code/skills/check/reference.txt": "reference bytes\n",
		".kimi-code/skills/manual.md":           "---\nname: manual\ndescription: A manual skill\ndisable_model_invocation: true\n---\nManual body.\n",
		".kimi-code/mcp.json":                   "{\"mcpServers\":{\"docs\":{\"transport\":\"sse\",\"url\":\"https://example.com/mcp\",\"enabled\":false,\"enabledTools\":[],\"bearerTokenEnvVar\":\"TOKEN\"},\"literal\":{\"command\":\"server\",\"env\":{\"MESSAGE\":\"${LITERAL}\"}}}}\n",
		"profile/config.toml":                   "default_model='existing-model'\n[thinking]\neffort='high'\n[[hooks]]\nevent='PreToolUse'\ncommand='check'\nmatcher='Bash'\ntimeout=5\n",
	} {
		writeFile(t, path, body)
	}
	execCLI(t, "import", "kimi")
	execCLI(t, "sync", "-t", "kimi")
	for path, want := range map[string]string{".kimi-code/AGENTS.md": "Use small changes.", "backend/.kimi-code/AGENTS.md": "Keep explicit errors.", ".kimi-code/agents/reviewer.md": "tools: []", ".kimi-code/skills/check/SKILL.md": "disableModelInvocation: true", ".kimi-code/skills/check/reference.txt": "reference bytes\n", ".kimi-code/skills/manual/SKILL.md": "disableModelInvocation: true", "profile/config.toml": "event = \"PreToolUse\""} {
		if got := readFile(t, filepath.Join(dir, path)); !strings.Contains(got, want) {
			t.Errorf("%s missing %q: %s", path, want, got)
		}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, ".kimi-code/mcp.json")), &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	docs := servers["docs"].(map[string]any)
	if docs["enabled"] != false || docs["transport"] != "sse" || docs["bearerTokenEnvVar"] != "TOKEN" {
		t.Errorf("native server changed: %#v", docs)
	}
	if got := servers["literal"].(map[string]any)["env"].(map[string]any)["MESSAGE"]; got != "${LITERAL}" {
		t.Errorf("literal changed: %v", got)
	}
	execCLI(t, "sync", "--check", "-t", "kimi")
	execCLI(t, "lint")
}
func TestSyncGlobal_KimiRestoresConfigAndUserHooks(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "current-kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := "# user settings\n[thinking]\neffort='low'\nenabled=true\n[[hooks]]\nevent='Stop'\ncommand='user-hook'\n"
	mustWriteGlobalTest(t, path, original)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings/kimi.yaml"), "model:\n  kimi: existing-model\nx-kimi:\n  telemetry: false\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\ntarget: kimi\nevent: PreToolUse\ncommand: check\nmatcher: Bash\ntimeout: 5\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents/reviewer.md"), "---\nname: reviewer\ndescription: Review changes\ncan: []\n---\nReview changes.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ncommand: server\ndisabled: true\n")
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("sync: %v %s", err, w)
	}
	got := readGlobalTest(t, path)
	for _, want := range []string{original, "default_model = \"existing-model\"", "telemetry = false", "event = \"PreToolUse\""} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "name =") {
		t.Errorf("hook name is not a native field: %s", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Error("private mode widened")
	}
	if _, _, err := runGlobalAgentTest("--only", "kimi", "--check"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"settings/kimi.yaml", "hooks/guard.yaml", "agents/reviewer.md", "mcps/docs.yaml"} {
		if err := os.Remove(filepath.Join(source, f)); err != nil {
			t.Fatal(err)
		}
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("remove: %v %s", err, w)
	}
	if got := readGlobalTest(t, path); got != original {
		t.Errorf("config bytes changed: got %q want %q", got, original)
	}
	if _, err := os.Stat(filepath.Join(home, ".kimi")); !os.IsNotExist(err) {
		t.Errorf("legacy home touched: %v", err)
	}
}

func TestImportGlobal_KimiAdoptsNativeHooksAndMCP(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi-current")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	before := "# mine\ndefault_model='existing-model'\n[[hooks]]\nevent='Stop'\ncommand='check'\ntimeout=5\n"
	mcp := filepath.Join(root, "mcp.json")
	beforeMCP := "{\"mcpServers\":{\"docs\":{\"transport\":\"sse\",\"url\":\"https://example.com/mcp\",\"enabled\":false,\"enabledTools\":[]}}}\n"
	mustWriteGlobalTest(t, path, before)
	mustWriteGlobalTest(t, mcp, beforeMCP)
	if _, w, err := runImportGlobalTest("kimi"); err != nil {
		t.Fatalf("import: %v %s", err, w)
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("adopt: %v %s", err, w)
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("config changed on adoption: %q", got)
	}
	if got := readGlobalTest(t, mcp); got != beforeMCP {
		t.Errorf("MCP changed on adoption: %q", got)
	}
	if _, _, err := runGlobalAgentTest("--only", "kimi", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_KimiEmptyMCPFilterIsNotUnrestricted(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "mcp.json")
	before := `{"mcpServers":{"docs":{"transport":"stdio","command":"server"}}}` + "\n"
	mustWriteGlobalTest(t, path, before)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps/docs.yaml"), "name: docs\ncommand: server\nx-kimi:\n  enabledTools: []\n")
	if _, _, err := runGlobalAgentTest("--only", "kimi"); err == nil {
		t.Fatal("empty allowlist was adopted as unrestricted user server")
	}
	if got := readGlobalTest(t, path); got != before {
		t.Errorf("conflict changed user server: %s", got)
	}
}

func TestImportKimi_RefusesCredentialBearingMetadata(t *testing.T) {
	for _, path := range []string{".kimi-code/agents/reviewer.md", ".kimi-code/skills/reviewer/SKILL.md", "profile/config.toml"} {
		t.Run(path, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\noutputs:\n  kimi:\n    conf-file: profile/config.toml\n")
			body := "---\nname: reviewer\ndescription: Review changes\napi_key: sk-live-abcdef1234567890\n---\nReview changes.\n"
			if strings.HasSuffix(path, ".toml") {
				body = "api_key='sk-live-abcdef1234567890'\n"
			}
			writeFile(t, path, body)
			if _, err := runCLI(t, "import", "kimi"); err == nil {
				t.Fatal("credential-bearing native metadata imported")
			}
		})
	}
}
func TestSyncGlobal_KimiCreatedConfigRemovedAndEffortLeftAlone(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	specPath := filepath.Join(source, "settings/kimi.yaml")
	mustWriteGlobalTest(t, specPath, "model: existing-model\neffort: high\nx-kimi:\n  telemetry: false\n  thinking:\n    enabled: false\n")
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("create: %v %s", err, w)
	}
	got := readGlobalTest(t, path)
	if strings.Contains(got, "thinking") || strings.Contains(got, "effort") {
		t.Errorf("global nested table was owned: %s", got)
	}
	if err := os.Remove(specPath); err != nil {
		t.Fatal(err)
	}
	if _, w, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatalf("remove: %v %s", err, w)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("created settings remains: %v", err)
	}
}
func TestImportKimi_FolderSkillWinsAndFlatDescriptionMatchesNative(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
	writeFile(t, ".kimi-code/skills/review/SKILL.md", "---\nname: review\ndescription: Folder description\n---\nFolder body.\n")
	writeFile(t, ".kimi-code/skills/review.md", "Flat body.\n")
	writeFile(t, ".kimi-code/skills/plain.md", "Plain skill description.\nMore text.\n")
	execCLI(t, "import", "kimi")
	execCLI(t, "sync", "-t", "kimi")
	if got := readFile(t, ".kimi-code/skills/review/SKILL.md"); !strings.Contains(got, "Folder body.") || strings.Contains(got, "Flat body.") {
		t.Errorf("flat skill replaced folder: %s", got)
	}
	if got := readFile(t, ".kimi-code/skills/plain/SKILL.md"); !strings.Contains(got, "description: Plain skill description.") {
		t.Errorf("native flat description changed: %s", got)
	}
}

func TestImportKimi_DropsPrivateAndGlobalInstructionBlocks(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
	writeFile(t, ".kimi-code/AGENTS.md", globalStart+"\nPersonal global instruction.\n"+globalEnd+"\n# Project\n\nPublic instruction.\n<!-- agnostic-ai:local:start -->\nPersonal local instruction.\n<!-- agnostic-ai:local:end -->\n")
	execCLI(t, "import", "kimi")
	source := readFile(t, ".agnostic-ai/AGNOSTIC_AI.md")
	if strings.Contains(source, "Personal") {
		t.Errorf("private block entered project source: %s", source)
	}
	if !strings.Contains(source, "Public instruction.") {
		t.Errorf("public instruction lost: %s", source)
	}
}

func TestSyncKimi_SharesPeerInstructionsOnFullAndPartialSync(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi, codex]\n")
	writeFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "# Project\n\nShared project instruction.\n")
	writeFile(t, ".agnostic-ai/rules/root.md", "---\nname: root\n---\nShared root rule.\n")
	writeFile(t, ".agnostic-ai/rules/backend.md", "---\nname: backend\nscope: backend\n---\nShared backend rule.\n")
	writeFile(t, ".agnostic-ai/rules/frontend.md", "---\nname: frontend\nglobs: frontend/**\n---\nShared frontend rule.\n")
	execCLI(t, "sync", "--all")
	if got := readFile(t, "frontend/AGENTS.md"); !strings.Contains(got, "Shared frontend rule.") {
		t.Errorf("shared nested glob rule missing: %s", got)
	}
	first := readFile(t, "AGENTS.md")
	scoped := readFile(t, "backend/AGENTS.md")
	for _, got := range []string{first, scoped} {
		if strings.Count(got, "Shared root rule.") > 1 || strings.Count(got, "Shared backend rule.") > 1 {
			t.Errorf("duplicated common rule: %s", got)
		}
	}
	if !strings.Contains(first, "Shared project instruction.") || !strings.Contains(first, "Shared root rule.") || !strings.Contains(scoped, "Shared backend rule.") {
		t.Fatalf("shared content missing: %s %s", first, scoped)
	}
	for _, path := range []string{".kimi-code/AGENTS.md", "backend/.kimi-code/AGENTS.md"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("duplicate branded instruction exists at %s: %v", path, err)
		}
	}
	execCLI(t, "sync", "-t", "kimi")
	execCLI(t, "sync", "--check", "-t", "kimi")
	if got := readFile(t, "AGENTS.md"); got != first {
		t.Errorf("partial sync changed peer instructions: %s", got)
	}
	if got := readFile(t, "backend/AGENTS.md"); got != scoped {
		t.Errorf("partial sync changed peer scoped instructions: %s", got)
	}
	writeFile(t, ".agnostic-ai/rules/peer-only.md", "---\nname: peer-only\ntarget: codex\n---\nCodex-only instruction.\n")
	if _, err := runCLI(t, "sync", "-t", "kimi"); err == nil {
		t.Fatal("incompatible root target filters accepted")
	}
	if got := readFile(t, "AGENTS.md"); got != first {
		t.Errorf("failed partial sync erased peer content: %s", got)
	}
}

func TestSyncKimi_BrandedInstructionsRetireWhenPeerStartsSharing(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
	writeFile(t, ".agnostic-ai/rules/project.md", "---\nname: project\n---\nShared rule.\n")
	execCLI(t, "sync", "-t", "kimi")
	if got := readFile(t, ".kimi-code/AGENTS.md"); !strings.Contains(got, "Shared rule.") {
		t.Fatal("initial branded rule missing")
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi, codex]\n")
	execCLI(t, "sync", "--all")
	if _, err := os.Stat(".kimi-code/AGENTS.md"); !os.IsNotExist(err) {
		t.Errorf("old branded instructions still load: %v", err)
	}
	if got := readFile(t, "AGENTS.md"); strings.Count(got, "Shared rule.") != 1 {
		t.Errorf("shared rule count changed: %s", got)
	}
}

func TestImportKimi_RefusesExternalNativeFileLinks(t *testing.T) {
	for _, path := range []string{".kimi-code/agents/reviewer.md", ".kimi-code/skills/reviewer.md"} {
		t.Run(path, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
			outside := filepath.Join(t.TempDir(), "reviewer.md")
			writeFile(t, outside, "---\nname: reviewer\ndescription: Review changes\n---\nOutside project instruction.\n")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dir, path)); err != nil {
				t.Fatal(err)
			}
			if _, err := runCLI(t, "import", "kimi"); err == nil {
				t.Fatal("external native file link imported")
			}
		})
	}
}

func TestImportKimi_KeepsNativeSkillAliasOrder(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\n")
	writeFile(t, ".kimi-code/skills/check/SKILL.md", "---\nname: check\ndescription: Check changes\ndisable_model_invocation: false\ndisableModelInvocation: true\nwhen-to-use: Earlier condition\nwhenToUse: Later condition\n---\nCheck changes.\n")
	execCLI(t, "import", "kimi")
	execCLI(t, "sync", "-t", "kimi")
	got := readFile(t, ".kimi-code/skills/check/SKILL.md")
	if !strings.Contains(got, "disableModelInvocation: true") || !strings.Contains(got, "whenToUse: Later condition") {
		t.Errorf("native alias order changed: %s", got)
	}
}

func TestSyncGlobal_KimiPlansOneConfigWriteAndRejectsConcurrentEdit(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "kimi")
	t.Setenv("KIMI_CODE_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := "# user\n[thinking]\neffort='low'\n"
	mustWriteGlobalTest(t, path, original)
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "existing-model"}}, {Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"target": "kimi", "event": "PreToolUse", "command": "check"}}})
	writes, _, err := buildGlobalWrites(home, source, []string{"kimi"}, nil, bundle, globalState{Version: globalStateVersion}, nil, io.Discard, "warn")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, w := range writes {
		if w.path != path {
			continue
		}
		count++
		if w.planned == nil || string(w.planned.data) != original {
			t.Fatal("combined settings/hooks plan lost original disk snapshot")
		}
		if !strings.Contains(string(w.data), "default_model") || !strings.Contains(string(w.data), "[[hooks]]") {
			t.Errorf("combined config write incomplete: %s", w.data)
		}
	}
	if count != 1 {
		t.Fatalf("planned %d config writes", count)
	}
	edited := "# user changed while sync ran\n"
	mustWriteGlobalTest(t, path, edited)
	if _, err := applyGlobalChanges(writes, nil, false, nil); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Fatalf("stale plan accepted: %v", err)
	}
	if got := readGlobalTest(t, path); got != edited {
		t.Errorf("concurrent edit overwritten: %s", got)
	}
}
func TestSyncGlobal_KimiFallbackRootAndMove(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIMI_CODE_HOME", "")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks/guard.yaml"), "name: guard\ntarget: kimi\nevent: Stop\ncommand: check\n")
	if _, _, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".kimi-code/config.toml")
	if got := readGlobalTest(t, path); !strings.Contains(got, "[[hooks]]") {
		t.Errorf("fallback output missing hook: %s", got)
	}
	moved := filepath.Join(home, "custom-kimi")
	t.Setenv("KIMI_CODE_HOME", moved)
	if _, _, err := runGlobalAgentTest("--only", "kimi"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("created old-root config remains: %v", err)
	}
	if got := readGlobalTest(t, filepath.Join(moved, "config.toml")); !strings.Contains(got, "[[hooks]]") {
		t.Errorf("relocated output missing hook: %s", got)
	}
}

func TestImportKimi_ReadsCRLFFrontmatter(t *testing.T) {
	data, err := normalizeKimiMarkdown("skill", []byte("---\r\nname: check\r\ndescription: Check changes\r\ndisableModelInvocation: true\r\n---\r\nCheck changes.\r\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") || !strings.Contains(string(data), "Check changes.") {
		t.Errorf("CRLF native skill changed: %s", data)
	}
}

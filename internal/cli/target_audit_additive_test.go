package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestAuditCodexModelObservers_AuthoredAndImportedHooksValidate(t *testing.T) {
	for _, imported := range []bool{false, true} {
		name := "authored"
		if imported {
			name = "imported"
		}
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
			if imported {
				writeFile(t, ".codex/hooks.json", `{"hooks":{"PreModelRequest":[{"hooks":[{"type":"command","command":"echo before"}]}],"PostModelRequest":[{"hooks":[{"type":"command","command":"echo after"}]}]}}`)
				execCLI(t, "import", "codex")
			} else {
				writeFile(t, ".agnostic-ai/hooks/before.yaml", "name: before\nevent: PreModelRequest\ncommand: echo before\n")
				writeFile(t, ".agnostic-ai/hooks/after.yaml", "name: after\nevent: PostModelRequest\ncommand: echo after\n")
			}
			for _, args := range [][]string{{"validate"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
				if out, err := runCLI(t, args...); err != nil {
					t.Errorf("%v: %v\n%s", args, err, out)
				}
			}
			body := readFile(t, ".codex/hooks.json")
			for _, event := range []string{"PreModelRequest", "PostModelRequest"} {
				if !strings.Contains(body, event) {
					t.Errorf("missing %s: %s", event, body)
				}
			}
		})
	}
}

func TestAuditJunieMCP_TimeoutMillisecondsRoundTrip(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [junie]\n")
	native := `{"mcpServers":{"local":{"command":"server","args":["--stdio"],"timeout":900000},"http":{"type":"http","url":"https://example.test/mcp","timeout":120000},"sse":{"type":"sse","url":"https://example.test/sse","timeout":240000}}}`
	writeFile(t, ".junie/mcp/mcp.json", native)
	execCLI(t, "import", "junie")
	for name, timeout := range map[string]string{"local": "900000", "http": "120000", "sse": "240000"} {
		body := readFile(t, filepath.Join(".agnostic-ai", "mcps", name+".yaml"))
		if !strings.Contains(body, "timeout: "+timeout) {
			t.Errorf("timeout lost: %s", body)
		}
	}
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
		execCLI(t, args...)
	}
	var want any
	var actual any
	if err := json.Unmarshal([]byte(native), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(readFile(t, ".junie/mcp/mcp.json")), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("native MCP fields changed: got %#v, want %#v", actual, want)
	}

}

func TestAuditCrushGoose_SharedManualSkillAndCommand(t *testing.T) {
	for _, separate := range []bool{false, true} {
		name := "shared"
		if separate {
			name = "separate"
		}
		t.Run(name, func(t *testing.T) {
			commandsAsSkillsProject(t, "crush, goose")
			config := "version: 1\ntargets: [crush, goose]\n"
			if separate {
				config += "outputs:\n  crush:\n    skills-dir: .crush/skills\n"
			}
			writeFile(t, "agnostic-ai.yaml", config)
			writeFile(t, ".agnostic-ai/skills/manual/SKILL.md", "---\nname: manual\ndescription: Manual task.\ndisable-model-invocation: true\n---\nRun when asked.\n")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			shared := readFile(t, ".agents/skills/manual/SKILL.md")
			if strings.Contains(shared, "disable-model-invocation:") || strings.Contains(shared, "user-invocable:") {
				t.Errorf("native invocation metadata reached Goose: %s", shared)
			}
			if separate {
				crush := readFile(t, ".crush/skills/manual/SKILL.md")
				if !strings.Contains(crush, "disable-model-invocation: true") {
					t.Errorf("Crush invocation metadata lost: %s", crush)
				}
			}
			commandPath := ".agents/skills/review/SKILL.md"
			if separate {
				if body := readFile(t, ".crush/skills/review/SKILL.md"); !strings.Contains(body, "disable-model-invocation: true") {
					t.Errorf("command lost manual policy: %s", body)
				}
			}
			if _, err := os.Stat(commandPath); !os.IsNotExist(err) {
				t.Errorf("Goose cannot preserve the command invocation policy: %v", err)
			}
			execCLI(t, "sync", "--check")
		})
	}
}

func TestAuditKiloGlobal_TrustedShellOnly(t *testing.T) {
	for _, mode := range []string{"warn", "error"} {
		t.Run(mode, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: "+mode+"\n")
			mustWriteGlobalTest(t, filepath.Join(source, "skills", "probe", "SKILL.md"), "---\nname: probe\ndescription: Show context.\n---\nLive: !`printf audit-probe`\n")
			_, warnings, err := runGlobalAgentTest("--only", "kilo")
			if err != nil {
				t.Fatalf("trusted global shell: %v", err)
			}
			if strings.Contains(warnings, "command does not run") {
				t.Errorf("false shell diagnostic: %s", warnings)
			}
			if body := readFile(t, filepath.Join(home, ".kilo", "skills", "probe", "SKILL.md")); !strings.Contains(body, "!`printf audit-probe`") {
				t.Errorf("shell placeholder lost: %s", body)
			}
			if _, _, err := runGlobalAgentTest("--only", "kilo", "--check"); err != nil {
				t.Errorf("check: %v", err)
			}
		})
	}
}

func TestAuditKiloGlobal_ArgumentPlaceholdersRemainUnsupported(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: error\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "probe", "SKILL.md"), globalSyntaxSkill)
	_, _, err := runGlobalAgentTest("--only", "kilo")
	if err == nil || !strings.Contains(err.Error(), "$ARGUMENTS") {
		t.Errorf("want unsupported arguments, got %v", err)
	}
}

func TestAuditKiloProject_OutputOverrideDoesNotTrustShell(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kilo]\non-unsupported: error\noutputs:\n  kilo:\n    skills-dir: .kilo/skills\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "probe", "SKILL.md"), "---\nname: probe\ndescription: Show context.\n---\n!`printf audit-probe`\n")
	if _, err := runCLI(t, "sync"); err == nil || !strings.Contains(err.Error(), "plain text") {
		t.Errorf("project output must remain unsupported: %v", err)
	}
}

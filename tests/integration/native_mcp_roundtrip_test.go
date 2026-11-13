package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMCPGlobal_ClientSettingsRoundTrip(t *testing.T) {
	binary := builtBinary(t)
	for _, target := range []string{"copilot", "pi"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			native := filepath.Join(dir, "native")
			rootEnv, filename := "COPILOT_HOME", "mcp-config.json"
			server := `{"type":"http","url":"https://example.invalid/mcp","oauthClientId":"public-client","timeout":45000,"tools":["*"]}`
			if target == "pi" {
				rootEnv, filename = "PI_CODING_AGENT_DIR", "mcp.json"
				server = `{"type":"http","url":"https://example.invalid/mcp","auth":{"provider":"radius"},"oauth":{"clientRegistration":"dcr"},"enabled":false}`
			}
			path := filepath.Join(native, filename)
			original := "{\n  \"unrelated\": true,\n  \"mcpServers\": {\"remote\": " + server + "}\n}\n"
			must(t, os.MkdirAll(native, 0o755))
			must(t, os.WriteFile(path, []byte(original), 0o600))
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "AGNOSTIC_AI_HOME="+source, rootEnv+"="+native)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			run("import", "--global", target)
			imported, err := os.ReadFile(filepath.Join(source, "mcps/remote.yaml"))
			must(t, err)
			if target == "pi" && !strings.Contains(string(imported), "x-pi:") {
				t.Errorf("auth not scoped: %s", imported)
			}
			run("sync", "--global", "--only", target)
			run("sync", "--global", "--only", target)
			run("sync", "--global", "--only", target, "--check")
			got, err := os.ReadFile(path)
			must(t, err)
			if string(got) != original {
				t.Errorf("native user content changed: %s", got)
			}
		})
	}
}

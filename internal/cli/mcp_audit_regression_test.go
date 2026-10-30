package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportKimi_RejectsInvalidExecutor(t *testing.T) {
	for _, executor := range []string{"kaos", "KAOS", "remote"} {
		t.Run(executor, func(t *testing.T) {
			for _, global := range []bool{false, true} {
				name := "project"
				if global {
					name = "global"
				}
				t.Run(name, func(t *testing.T) {
					home, source := globalAgentTestHome(t)
					native := `{"mcpServers":{"obsolete":{"transport":"stdio","command":"example-mcp","executor":"` + executor + `"}}}` + "\n"
					var err error
					var path, specPath string
					if global {
						path = filepath.Join(home, ".kimi-code/mcp.json")
						specPath = filepath.Join(source, "mcps/obsolete.yaml")
						mustWriteGlobalTest(t, path, native)
						_, _, err = runImportGlobalTest("kimi")
					} else {
						writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\nbuiltins: []\n")
						path = ".kimi-code/mcp.json"
						specPath = ".agnostic-ai/mcps/obsolete.yaml"
						writeFile(t, path, native)
						_, err = runCLI(t, "import", "kimi")
					}
					if err == nil {
						t.Error("invalid executor was imported")
					} else if !strings.Contains(err.Error(), executor) || !strings.Contains(err.Error(), "remove it or set executor: local") {
						t.Errorf("error must explain the invalid executor and supported option: %v", err)
					}
					if _, err := os.Stat(specPath); !os.IsNotExist(err) {
						t.Errorf("invalid executor published an MCP spec: %v", err)
					}
					if got := readGlobalTest(t, path); got != native {
						t.Error("import changed native configuration")
					}
				})
			}
		})
	}
}

func TestImportKimi_LocalExecutorAndRuntimeIDRoundTrip(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [kimi]\nbuiltins: []\n")
	writeFile(t, ".kimi-code/mcp.json", `{"mcpServers":{"local":{"transport":"stdio","command":"example-mcp","executor":"local","runtime_id":"workspace"}}}`)
	for _, args := range [][]string{{"import", "kimi"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	for _, want := range []string{`"executor": "local"`, `"runtime_id": "workspace"`} {
		if data := readFile(t, ".kimi-code/mcp.json"); !strings.Contains(data, want) {
			t.Errorf("round-trip MCP lacks %q: %s", want, data)
		}
	}
}

func TestImportGemini_OAuthOptionsRoundTrip(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\nbuiltins: []\n")
	oauth := map[string]any{"enabled": true, "allowPrivateNetwork": true, "authorizationResponseIssParameterSupported": false, "clientId": "public-client"}
	native, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"docs": map[string]any{"httpUrl": "https://docs.example/mcp", "oauth": oauth}}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, ".gemini/settings.json", string(native))
	for _, args := range [][]string{{"import", "gemini"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readFile(t, ".gemini/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Servers["docs"]["oauth"]; !reflect.DeepEqual(got, oauth) {
		t.Errorf("round-trip OAuth options = %#v, want %#v", got, oauth)
	}
	if imported := readFile(t, ".agnostic-ai/mcps/docs.yaml"); !strings.Contains(imported, "x-gemini:") {
		t.Errorf("native OAuth options must remain Gemini-scoped: %s", imported)
	}
}

func TestImportGemini_OAuthCredentialsStayOutOfSpecs(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := `{"mcpServers":{"secret":{"httpUrl":"https://docs.example/mcp","oauth":{"enabled":true,"clientSecret":"literal-test-secret"}},"default-secret":{"httpUrl":"https://default.example/mcp","oauth":{"enabled":true,"clientSecret":"${OAUTH_SECRET:-literal-test-secret}"}},"reference":{"httpUrl":"https://reference.example/mcp","oauth":{"enabled":true,"clientSecret":"${OAUTH_SECRET}"}}}}`
			log := captureSummary(t)
			var warnings string
			var specDir, path string
			if global {
				path = filepath.Join(home, ".gemini/settings.json")
				specDir = filepath.Join(source, "mcps")
				mustWriteGlobalTest(t, path, native)
				var err error
				if _, warnings, err = runImportGlobalTest("gemini"); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\nbuiltins: []\n")
				path = ".gemini/settings.json"
				specDir = ".agnostic-ai/mcps"
				writeFile(t, path, native)
				if out, err := runCLI(t, "import", "gemini"); err != nil {
					t.Fatalf("import: %v\n%s", err, out)
				}
			}
			for _, name := range []string{"secret", "default-secret"} {
				if _, err := os.Stat(filepath.Join(specDir, name+".yaml")); !os.IsNotExist(err) {
					t.Errorf("literal OAuth credential was published: %v", err)
				}
			}
			if imported := readFile(t, filepath.Join(specDir, "reference.yaml")); !strings.Contains(imported, "${OAUTH_SECRET}") || !strings.Contains(imported, "x-gemini:") {
				t.Errorf("OAuth reference was lost or unscoped: %s", imported)
			}
			if got := readGlobalTest(t, path); got != native {
				t.Error("import changed native configuration")
			}
			reported := log.String() + warnings
			if strings.Contains(reported, "literal-test-secret") {
				t.Error("import exposed a literal OAuth credential")
			}
			if !strings.Contains(reported, "oauth.clientSecret") {
				t.Error("import did not name the skipped OAuth credential field")
			}
		})
	}
}

func TestImportGemini_UnbracedOAuthReferenceRoundTrip(t *testing.T) {
	for _, global := range []bool{false, true} {
		name := "project"
		if global {
			name = "global"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			native := `{"mcpServers":{"reference":{"httpUrl":"https://docs.example/mcp","oauth":{"enabled":true,"allowPrivateNetwork":false,"clientId":"public-client","clientSecret":"$OAUTH_SECRET"}}}}` + "\n"
			var path, specPath string
			if global {
				path = filepath.Join(home, ".gemini/settings.json")
				specPath = filepath.Join(source, "mcps/reference.yaml")
				mustWriteGlobalTest(t, path, native)
				if _, warnings, err := runImportGlobalTest("gemini"); err != nil {
					t.Fatalf("import: %v %s", err, warnings)
				}
			} else {
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\nbuiltins: []\n")
				path = ".gemini/settings.json"
				specPath = ".agnostic-ai/mcps/reference.yaml"
				writeFile(t, path, native)
				if out, err := runCLI(t, "import", "gemini"); err != nil {
					t.Fatalf("import: %v\n%s", err, out)
				}
			}
			if imported := readFile(t, specPath); !strings.Contains(imported, "$OAUTH_SECRET") || !strings.Contains(imported, "x-gemini:") {
				t.Errorf("native OAuth reference was lost or unscoped: %s", imported)
			}
			if got := readGlobalTest(t, path); got != native {
				t.Error("import changed native configuration")
			}
			if global {
				for _, args := range [][]string{{"--only", "gemini"}, {"--only", "gemini", "--check"}} {
					if _, warnings, err := runGlobalAgentTest(args...); err != nil {
						t.Fatalf("%v: %v %s", args, err, warnings)
					}
				}
				if got := readGlobalTest(t, path); got != native {
					t.Error("global adoption changed native OAuth reference bytes")
				}
			} else {
				for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
					if out, err := runCLI(t, args...); err != nil {
						t.Fatalf("%v: %v\n%s", args, err, out)
					}
				}
				var doc struct {
					Servers map[string]map[string]any `json:"mcpServers"`
				}
				if err := json.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"enabled": true, "allowPrivateNetwork": false, "clientId": "public-client", "clientSecret": "$OAUTH_SECRET"}
				if got := doc.Servers["reference"]["oauth"]; !reflect.DeepEqual(got, want) {
					t.Errorf("native OAuth options changed: got %#v want %#v", got, want)
				}
			}
		})
	}
}

func TestSyncGlobal_VibeSamplingDefaultControlsOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, nativeSampling, specSampling string
		conflict                           bool
	}{
		{name: "omitted"},
		{name: "explicit false", nativeSampling: "sampling_enabled = false\n"},
		{name: "explicit true conflicts", nativeSampling: "sampling_enabled = true\n", conflict: true},
		{name: "explicit true spec matches", nativeSampling: "sampling_enabled = true\n", specSampling: "x-vibe:\n  sampling_enabled: true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			root := filepath.Join(home, "vibe")
			t.Setenv("VIBE_HOME", root)
			path := filepath.Join(root, "config.toml")
			prefix := "# User configuration\nactive_model = 'audit-model'\n\n"
			original := prefix + "[[mcp_servers]]\nname = 'audit'\ntransport = 'stdio'\ncommand = 'example-mcp'\n" + tc.nativeSampling
			mustWriteGlobalTest(t, path, original)
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			specPath := filepath.Join(source, "mcps/audit.yaml")
			mustWriteGlobalTest(t, specPath, "name: audit\ncommand: example-mcp\n"+tc.specSampling)
			for _, args := range [][]string{{"--only", "vibe"}, {"--only", "vibe", "--check"}} {
				_, warnings, err := runGlobalAgentTest(args...)
				if tc.conflict {
					if err == nil {
						t.Errorf("different native sampling was adopted: %s", warnings)
					}
				} else if err != nil {
					t.Errorf("matching native sampling was refused: %v %s", err, warnings)
				}
				if got := readGlobalTest(t, path); got != original {
					t.Error("adoption or check changed user bytes")
				}
			}
			if err := os.Remove(specPath); err != nil {
				t.Fatal(err)
			}
			if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
				t.Fatalf("remove: %v %s", err, warnings)
			}
			want := prefix
			if tc.conflict {
				want = original
			}
			if got := readGlobalTest(t, path); got != want {
				t.Errorf("removal changed wrong user bytes: got %q want %q", got, want)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Error("global sync widened private file permissions")
			}
		})
	}
}

func TestSyncGlobal_VibeSamplingUpgradeProtectsOwnedTrue(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := filepath.Join(home, "vibe")
	t.Setenv("VIBE_HOME", root)
	path := filepath.Join(root, "config.toml")
	original := "# User configuration\n\n[[mcp_servers]]\nname = 'audit'\ntransport = 'stdio'\ncommand = 'example-mcp'\nsampling_enabled = true\n"
	mustWriteGlobalTest(t, path, original)
	specPath := filepath.Join(source, "mcps/audit.yaml")
	base := "name: audit\ncommand: example-mcp\n"
	mustWriteGlobalTest(t, specPath, base+"x-vibe:\n  sampling_enabled: true\n")
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("adopt: %v %s", err, warnings)
	}
	mustWriteGlobalTest(t, specPath, base)
	statePath := filepath.Join(source, "state/global.json")
	stateBefore := readGlobalTest(t, statePath)
	for _, args := range [][]string{{"--only", "vibe", "--dry-run"}, {"--only", "vibe", "--check"}, {"--only", "vibe"}} {
		_, _, err := runGlobalAgentTest(args...)
		if err == nil || !strings.Contains(err.Error(), "sampling_enabled") || !strings.Contains(err.Error(), path) {
			t.Errorf("upgrade must diagnose owned sampling true: %v", err)
		}
		if got := readGlobalTest(t, path); got != original {
			t.Error("upgrade deleted user sampling setting")
		}
		if got := readGlobalTest(t, statePath); got != stateBefore {
			t.Error("upgrade changed ownership state")
		}
	}
	mustWriteGlobalTest(t, specPath, base+"x-vibe:\n  sampling_enabled: false\n")
	if _, warnings, err := runGlobalAgentTest("--only", "vibe"); err != nil {
		t.Fatalf("explicit change: %v %s", err, warnings)
	}
	if got := readGlobalTest(t, path); !strings.Contains(got, "sampling_enabled = false") {
		t.Errorf("explicit sampling choice was not applied: %s", got)
	}
}

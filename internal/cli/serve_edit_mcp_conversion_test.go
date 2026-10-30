package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func mcpConversionProject(t *testing.T, targets, path, native string) (string, http.Handler, map[string]any) {
	t.Helper()
	nativeMCPHome(t)
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	mustWrite(t, path, native)
	request := mcpEditIdentity("docs", "shared", "project", filepath.ToSlash(path))
	return project, newEditHandler(project, false), request
}

func mcpConversionDoc(t *testing.T, preview mcpEditTestReply) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(preview.Spec), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestServeEditMCP_KiloNativeEntriesPreviewApplyAndSync(t *testing.T) {
	for _, tc := range []struct {
		name, native string
		credential   bool
		remote       bool
	}{
		{name: "local", native: `{"mcp":{"docs":{"type":"local","command":["docs-mcp","--stdio"],"environment":{"MODE":"production"},"enabled":false}}}`},
		{name: "credential", native: `{"mcp":{"docs":{"type":"local","command":["docs-mcp","--stdio"],"environment":{"GITHUB_TOKEN":"ghp_synthetic123"}}}}`, credential: true},
		{name: "remote", native: `{"mcp":{"docs":{"type":"remote","url":"https://example.test/mcp"}}}`, remote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project, h, request := mcpConversionProject(t, "kilo, claude", "kilo.jsonc", tc.native)
			request["target"] = "kilo"
			preview := mcpEditPreview(t, h, request)
			doc := mcpConversionDoc(t, preview)
			if tc.remote {
				if doc["type"] != "http" || doc["url"] != "https://example.test/mcp" {
					t.Errorf("remote spec = %s", preview.Spec)
				}
			} else {
				if doc["type"] != nil && doc["type"] != "stdio" || doc["command"] != "docs-mcp" || !reflect.DeepEqual(doc["args"], []any{"--stdio"}) {
					t.Errorf("local spec = %s", preview.Spec)
				}
				env, _ := doc["env"].(map[string]any)
				if tc.credential {
					if env["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" || !slices.Contains(preview.Variables, "GITHUB_TOKEN") {
						t.Errorf("credential spec = %#v", preview)
					}
				} else if env["MODE"] != "${MODE}" || !slices.Contains(preview.Variables, "MODE") || doc["disabled"] != true {
					t.Errorf("environment and disabled state = %s", preview.Spec)
				}
			}
			for _, key := range []string{"environment", "enabled"} {
				if _, exists := doc[key]; exists {
					t.Errorf("native key %s remains: %s", key, preview.Spec)
				}
			}
			mcpEditApply(t, h, request, preview)
			if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
				t.Errorf("saved spec = %s", got)
			}
			if got := readOptional(t, "kilo.jsonc"); got != tc.native {
				t.Errorf("apply changed native config: %s", got)
			}
			if _, err := os.Stat(".mcp.json"); !os.IsNotExist(err) {
				t.Errorf("apply wrote Claude config: %v", err)
			}
			syncProject(t)
			claude := readNativeMCPServers(filepath.Join(project, ".mcp.json"), "json", "mcpServers")["docs"]
			if tc.remote {
				if claude["type"] != "http" || claude["url"] != "https://example.test/mcp" {
					t.Errorf("Claude remote output = %#v", claude)
				}
			} else if claude["command"] != "docs-mcp" || !reflect.DeepEqual(claude["args"], []any{"--stdio"}) {
				t.Errorf("Claude local output = %#v", claude)
			}
			if tc.credential {
				env, _ := claude["env"].(map[string]any)
				if env["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" {
					t.Errorf("Claude environment = %#v", env)
				}
			} else {
				kilo := readNativeMCPServers(filepath.Join(project, "kilo.jsonc"), "json", "mcp")["docs"]
				if tc.remote {
					if kilo["type"] != "remote" || kilo["url"] != "https://example.test/mcp" {
						t.Errorf("Kilo remote output = %#v", kilo)
					}
				} else if kilo["type"] != "local" || kilo["enabled"] != false || !reflect.DeepEqual(kilo["command"], []any{"docs-mcp", "--stdio"}) {
					t.Errorf("Kilo local output = %#v", kilo)
				}
			}
		})
	}
}

func TestServeEditMCP_WindsurfSSESurvivesPreviewApplyAndSync(t *testing.T) {
	const native = `{"mcpServers":{"docs":{"transport":"sse","url":"https://example.test/events"}}}`
	project, h, request := mcpConversionProject(t, "windsurf, claude", windsurfMCPFile, native)
	request["target"] = "windsurf"
	preview := mcpEditPreview(t, h, request)
	doc := mcpConversionDoc(t, preview)
	if doc["type"] != "sse" || doc["transport"] != nil {
		t.Errorf("SSE spec = %s", preview.Spec)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, windsurfMCPFile); got != native {
		t.Errorf("apply changed native config: %s", got)
	}
	syncProject(t)
	windsurf := readNativeMCPServers(filepath.Join(project, windsurfMCPFile), "json", "mcpServers")["docs"]
	claude := readNativeMCPServers(filepath.Join(project, ".mcp.json"), "json", "mcpServers")["docs"]
	if windsurf["transport"] != "sse" || claude["type"] != "sse" {
		t.Errorf("SSE output: Windsurf = %#v, Claude = %#v", windsurf, claude)
	}
}

func TestServeEditMCP_ClaudeDisabledProjectServerStaysDisabled(t *testing.T) {
	const native = `{"mcpServers":{"docs":{"type":"http","url":"https://example.test/mcp"}}}`
	project, h, request := mcpConversionProject(t, "claude", ".mcp.json", native)
	const settings = `{"disabledMcpjsonServers":["docs"]}`
	mustWrite(t, ".claude/settings.json", settings)
	preview := mcpEditPreview(t, h, request)
	if mcpConversionDoc(t, preview)["disabled"] != true {
		t.Errorf("disabled project server became enabled: %s", preview.Spec)
	}
	mcpEditApply(t, h, request, preview)
	if got := readOptional(t, filepath.Join(project, preview.File)); got != preview.Spec {
		t.Errorf("saved spec = %s", got)
	}
	if readOptional(t, ".mcp.json") != native || readOptional(t, ".claude/settings.json") != settings {
		t.Error("apply changed Claude's native files")
	}
}

func TestServeEditMCP_ClaudeDisabledChangeInvalidatesPreview(t *testing.T) {
	_, h, request := mcpConversionProject(t, "claude", ".mcp.json", `{"mcpServers":{"docs":{"type":"http","url":"https://example.test/mcp"}}}`)
	mustWrite(t, ".claude/settings.json", `{"disabledMcpjsonServers":[]}`)
	preview := mcpEditPreview(t, h, request)
	mustWrite(t, ".claude/settings.json", `{"disabledMcpjsonServers":["docs"]}`)
	request["action"], request["expected"] = "apply", preview.Expected
	rec := mcpEditRequest(t, h, request)
	if rec.Code != http.StatusConflict {
		t.Errorf("changed disabled state = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "mcps", "docs.yaml")); !os.IsNotExist(err) {
		t.Errorf("stale preview wrote a spec: %v", err)
	}
}

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromOpencode_V2MCPReconstructsNativeConfiguration(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	native := `{
  "mcp": {
    "timeout": {"catalog": 3000, "execution": 9000},
    "codemode": false,
    "servers": {
      "docs": {
        "type": "local",
        "command": ["node", "server.js"],
        "cwd": "tools/docs",
        "environment": {"DOCS_TOKEN": "{env:DOCS_TOKEN}"},
        "disabled": true,
        "timeout": {"catalog": 2000, "execution": 5000},
        "vendor_option": {"enabled": false}
      },
      "remote": {
        "type": "remote",
        "url": "https://example.test/mcp",
        "headers": {"Authorization": "Bearer {env:MCP_TOKEN}"},
        "disabled": false,
        "oauth": false
      }
    }
  },
  "permissions": [{"action": "shell", "resource": "git push *", "effect": "deny"}]
}`
	writeFile(t, opencodeMCPFile, native)
	command := NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode", "--dry-run", "--diff"})
	if err := command.Execute(); err != nil {
		t.Fatalf("preview native V2 MCP: %v", err)
	}
	if got := readFile(t, opencodeMCPFile); got != native {
		t.Error("preview changes native input")
	}
	if _, err := os.Stat(".agnostic-ai"); !os.IsNotExist(err) {
		t.Errorf("preview writes source tree: %v", err)
	}
	command = NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode"})
	if err := command.Execute(); err != nil {
		t.Fatalf("import native V2 MCP: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "mcps", "servers.yaml")); !os.IsNotExist(err) {
		t.Errorf("V2 envelope imported as a portable server: %v", err)
	}
	if err := os.Remove(opencodeMCPFile); err != nil {
		t.Fatal(err)
	}
	command = NewRootCmd("test")
	command.SetArgs([]string{"sync"})
	if err := command.Execute(); err != nil {
		t.Fatalf("reconstruct native configuration from imported specs: %v", err)
	}
	var want, got map[string]any
	if err := json.Unmarshal([]byte(native), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(readFile(t, opencodeMCPFile)), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["mcp"], want["mcp"]) {
		t.Errorf("reconstructed MCP = %#v, want %#v", got["mcp"], want["mcp"])
	}
	for _, args := range [][]string{{"sync", "--check"}, {"lint"}} {
		command = NewRootCmd("test")
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Errorf("%v after native import: %v", args, err)
		}
	}
}

func TestOpencodeMCPReview_V1NamedServersRemovalStopsServer(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	serverPath := filepath.Join(".agnostic-ai", "mcps", "servers.yaml")
	writeFile(t, serverPath, "name: servers\ncommand: retired-server\n")
	writeFile(t, filepath.Join(".agnostic-ai", "mcps", "other.yaml"), "name: other\ncommand: active-server\n")
	command := NewRootCmd("test")
	command.SetArgs([]string{"sync"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(serverPath); err != nil {
		t.Fatal(err)
	}
	command = NewRootCmd("test")
	command.SetArgs([]string{"sync"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal([]byte(readFile(t, opencodeMCPFile)), &native); err != nil {
		t.Fatal(err)
	}
	mcp := native["mcp"].(map[string]any)
	if _, active := mcp["servers"]; active {
		t.Errorf("removed V1 servers spec still starts a server: %#v", mcp["servers"])
	}
	if _, active := mcp["other"]; !active {
		t.Error("removing one server removes an unrelated server")
	}
}

func TestOpencodeMCPReview_V1NamedServersAcceptsURLHeader(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"mcp":{"servers":{"type":"local","command":["srv"],"headers":{"url":"https://example.test"}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatalf("valid V1 server with URL header: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "mcps", "servers.yaml")); !strings.Contains(got, "command: srv") {
		t.Errorf("V1 server was not adopted: %s", got)
	}
}

func TestOpencodeMCPReview_RepeatedPreviewWithAbsoluteSettings(t *testing.T) {
	dir := t.TempDir()
	settingsDir := filepath.Join(t.TempDir(), "settings")
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [opencode]\nsources:\n  settings: %q\n", filepath.ToSlash(settingsDir)))
	writeFile(t, opencodeMCPFile, `{"mcp":{"servers":{"docs":{"type":"local","command":["srv"]}}}}`)
	command := NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode"})
	if err := command.Execute(); err != nil {
		t.Fatalf("initial absolute-source import: %v", err)
	}
	settingsPath := filepath.Join(settingsDir, "opencode-mcp.yaml")
	before := readFile(t, settingsPath)
	nativeBefore := readFile(t, opencodeMCPFile)
	command = NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode", "--dry-run", "--diff"})
	if err := command.Execute(); err != nil {
		t.Errorf("preview incorrectly conflicts with its own imported settings: %v", err)
	}
	if got := readFile(t, settingsPath); got != before {
		t.Error("preview changes the absolute source")
	}
	if got := readFile(t, opencodeMCPFile); got != nativeBefore {
		t.Error("preview changes native input")
	}
}

func TestOpencodeMCPReview_RefusesLocalPortableOwnerBeforeAdoption(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	localPath := filepath.Join(".agnostic-ai", "local", "mcps", "private.yaml")
	local := "name: private\ncommand: srv\ntarget: opencode\n"
	writeFile(t, localPath, local)
	native := `{"mcp":{"servers":{"docs":{"type":"local","command":["docs-srv"]}}}}`
	writeFile(t, opencodeMCPFile, native)
	command := NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "conflict") || !strings.Contains(err.Error(), "private.yaml") {
		t.Errorf("import with local portable owner = %v, want refusal naming private.yaml", err)
	}
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "settings", "opencode-mcp.yaml")); !os.IsNotExist(err) {
		t.Errorf("conflicting import adopts shared native settings: %v", err)
	}
	if got := readFile(t, localPath); got != local {
		t.Error("refusal changes the local MCP spec")
	}
	if got := readFile(t, opencodeMCPFile); got != native {
		t.Error("refusal changes native input")
	}
}

func TestOpencodeMCPReview_RefusesLocalNativeSettingsOwner(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	localPath := filepath.Join(".agnostic-ai", "local", "settings", "private.yaml")
	local := "target: opencode\nx-opencode:\n  mcp:\n    servers:\n      private:\n        type: local\n        command: [srv]\n"
	writeFile(t, localPath, local)
	writeFile(t, opencodeMCPFile, `{"mcp":{"servers":{"docs":{"type":"local","command":["docs-srv"]}}}}`)
	command := NewRootCmd("test")
	command.SetArgs([]string{"import", "opencode"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "conflict") || !strings.Contains(err.Error(), "private.yaml") {
		t.Errorf("import with local settings owner = %v, want refusal naming private.yaml", err)
	}
	if got := readFile(t, localPath); got != local {
		t.Error("refusal changes the local settings owner")
	}
}

func TestImportFromOpencode_V2MCPOwnershipPreservesUserEdits(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	writeFile(t, opencodeMCPFile, `{"mcp":{"servers":{"docs":{"type":"local","command":["srv"]},"retire":{"type":"local","command":["old"]}}},"permissions":[{"effect":"deny","action":"shell","resource":"*"}]}`)
	for _, args := range [][]string{{"import", "opencode"}, {"sync"}, {"import", "opencode"}} {
		command := NewRootCmd("test")
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	var native map[string]any
	if err := json.Unmarshal([]byte(readFile(t, opencodeMCPFile)), &native); err != nil {
		t.Fatal(err)
	}
	mcp := native["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	servers["user"] = map[string]any{"type": "local", "command": []any{"user-srv"}}
	servers["retire"] = map[string]any{"type": "local", "command": []any{"edited-old"}}
	mcp["timeout"] = map[string]any{"catalog": 42}
	raw, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, opencodeMCPFile, string(raw))
	settingsPath := filepath.Join(".agnostic-ai", "settings", "opencode-mcp.yaml")
	var settings map[string]any
	if err := yaml.Unmarshal([]byte(readFile(t, settingsPath)), &settings); err != nil {
		t.Fatal(err)
	}
	managed := settings["x-opencode"].(map[string]any)["mcp"].(map[string]any)["servers"].(map[string]any)
	delete(managed, "retire")
	managed["docs"].(map[string]any)["command"] = []any{"updated-srv"}
	raw, err = yaml.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, settingsPath, string(raw))
	command := NewRootCmd("test")
	command.SetArgs([]string{"sync"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, opencodeMCPFile)
	for _, want := range []string{"updated-srv", "user-srv", "edited-old", `"catalog": 42`, `"permissions"`, `"action": "shell"`, `"resource": "*"`, `"effect": "deny"`} {
		if !strings.Contains(got, want) {
			t.Errorf("sync loses %q: %s", want, got)
		}
	}
}

func TestImportFromOpencode_V2MCPRefusesBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		mcp  string
		want string
	}{
		{"mixed versions", `{"servers":{"docs":{"type":"local","command":["srv"]}},"legacy":{"type":"local","command":["old"]}}`, "mixes V2"},
		{"mixed envelope", `{"servers":{"command":["legacy"],"docs":{"type":"local","command":["srv"]}}}`, "mixes a V1 entry"},
		{"OAuth credential", `{"servers":{"docs":{"type":"remote","url":"https://example.test/mcp","oauth":{"client_secret":"TOPSECRET"}}}}`, "credential import cannot rewrite"},
		{"default credential", `{"servers":{},"api_key":"TOPSECRET"}`, "credential import cannot rewrite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			original := `{"mcp":` + tc.mcp + `}`
			writeFile(t, filepath.Join(dir, opencodeMCPFile), original)
			err := importFromOpencode(dir, rootSources())
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), opencodeMCPFile) {
				t.Errorf("import error = %v, want path and %q", err, tc.want)
			}
			if err != nil && strings.Contains(err.Error(), "TOPSECRET") {
				t.Error("refusal exposes a credential")
			}
			if got := readFile(t, filepath.Join(dir, opencodeMCPFile)); got != original {
				t.Errorf("refusal changes native input: %s", got)
			}
			for _, source := range []string{"mcps", "settings", "rules", "agents", "skills", "commands"} {
				if _, err := os.Stat(filepath.Join(dir, source)); !os.IsNotExist(err) {
					t.Errorf("refusal created source %s: %v", source, err)
				}
			}
		})
	}
}

func TestImportFromOpencode_V1ServerNamedServers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"mcp":{"servers":{"type":"local","command":["srv"]}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "mcps", "servers.yaml")); !strings.Contains(got, "command: srv") {
		t.Errorf("V1 server named servers was not imported: %s", got)
	}
}

func TestImportFromOpencode_V2MCPCredentialsBecomeNativeReferences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"mcp":{"servers":{"docs":{"type":"local","command":["srv"],"environment":{"DOCS_TOKEN":"TOPSECRET"},"headers":{"Authorization":"Bearer TOPSECRET"}}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "settings", "opencode-mcp.yaml"))
	if strings.Contains(got, "TOPSECRET") {
		t.Error("native MCP settings retain a literal credential")
	}
	for _, want := range []string{"{env:DOCS_TOKEN}", "Bearer {env:"} {
		if !strings.Contains(got, want) {
			t.Errorf("native MCP settings lack %q: %s", want, got)
		}
	}
}

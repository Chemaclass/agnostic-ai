package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const zedMCPNative = `{
  "context_servers": {
    "local": {"command": "node", "args": ["server.js"], "enabled": false, "remote": true},
    "web": {"url": "https://example.test/mcp", "timeout": 90, "oauth": {"client_id": "abc"}}
  }
}`

func zedServers(t *testing.T) map[string]map[string]any {
	t.Helper()
	var got struct {
		Servers map[string]map[string]any `json:"context_servers"`
	}
	if err := json.Unmarshal([]byte(readFile(t, zedSettings)), &got); err != nil {
		t.Fatal(err)
	}
	return got.Servers
}

func zedMCPProject(t *testing.T, native string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [zed]\n")
	writeFile(t, zedSettings, native)
}

func TestImportZedMCP_RoundTripsEnabledTimeoutOAuthAndRemote(t *testing.T) {
	zedMCPProject(t, zedMCPNative)
	execCLI(t, "import", "zed")
	execCLI(t, "sync", "-t", "zed")

	got := zedServers(t)
	if got["local"]["enabled"] != false || got["local"]["remote"] != true {
		t.Errorf("local lost enabled or remote: %#v", got["local"])
	}
	if got["web"]["timeout"] != float64(90) {
		t.Errorf("web timeout = %#v", got["web"]["timeout"])
	}
	if want := map[string]any{"client_id": "abc"}; !reflect.DeepEqual(got["web"]["oauth"], want) {
		t.Errorf("web oauth = %#v, want %#v", got["web"]["oauth"], want)
	}
	if _, ok := got["web"]["enabled"]; ok {
		t.Errorf("an enabled server must not gain an enabled key: %#v", got["web"])
	}
	execCLI(t, "sync", "--check")
}

func TestImportZedMCP_DeletedKeysDropOnNextImport(t *testing.T) {
	zedMCPProject(t, zedMCPNative)
	execCLI(t, "import", "zed")
	execCLI(t, "sync", "-t", "zed")

	writeFile(t, zedSettings, `{
  "context_servers": {
    "local": {"command": "node", "args": ["server.js"], "remote": true},
    "web": {"url": "https://example.test/mcp", "timeout": 90, "oauth": {"client_id": "abc"}}
  }
}`)
	execCLI(t, "import", "zed")
	execCLI(t, "sync", "-t", "zed")
	got := zedServers(t)
	if _, ok := got["local"]["enabled"]; ok {
		t.Errorf("removed enabled key came back: %#v", got["local"])
	}
	if got["local"]["remote"] != true || got["web"]["timeout"] != float64(90) {
		t.Errorf("kept keys lost: %#v", got)
	}

	writeFile(t, zedSettings, `{
  "context_servers": {
    "local": {"command": "node", "args": ["server.js"]},
    "web": {"url": "https://example.test/mcp"}
  }
}`)
	execCLI(t, "import", "zed")
	execCLI(t, "sync", "-t", "zed")
	got = zedServers(t)
	for name, entry := range got {
		for _, key := range []string{"enabled", "remote", "timeout", "oauth"} {
			if _, ok := entry[key]; ok {
				t.Errorf("%s: removed key %s came back: %#v", name, key, entry)
			}
		}
	}
}

func TestImportZedMCP_LeavesOutServerWithOAuthSecret(t *testing.T) {
	zedMCPProject(t, `{
  "context_servers": {
    "secure": {"url": "https://example.test/mcp", "oauth": {"client_id": "abc", "client_secret": "s3cr3t-literal-value-9f8e7d"}},
    "plain": {"url": "https://example.test/plain", "timeout": 30}
  }
}`)
	execCLI(t, "import", "zed")

	if _, err := os.Stat(filepath.Join(".agnostic-ai", "mcps", "secure.yaml")); !os.IsNotExist(err) {
		t.Errorf("server with an OAuth secret must be left out: %v", err)
	}
	if !strings.Contains(readFile(t, filepath.Join(".agnostic-ai", "mcps", "plain.yaml")), "timeout: 30") {
		t.Errorf("other servers must still import")
	}
	_ = filepath.WalkDir(".agnostic-ai", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(readFile(t, path), "s3cr3t-literal") {
			t.Errorf("secret written to %s", path)
		}
		return nil
	})
}

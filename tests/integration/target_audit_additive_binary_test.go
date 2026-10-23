package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditAdditive_BuiltBinaryImportsAndChecksNativeConfiguration(t *testing.T) {
	binary := buildEllipsisBinary(t)
	for _, tc := range []struct {
		target, nativePath, native, expected string
	}{
		{"codex", ".codex/hooks.json", `{"hooks":{"PreModelRequest":[{"hooks":[{"type":"command","command":"echo before"}]}],"PostModelRequest":[{"hooks":[{"type":"command","command":"echo after"}]}]}}`, "PostModelRequest"},
		{"junie", ".junie/mcp/mcp.json", `{"mcpServers":{"slow":{"type":"http","url":"https://example.test/mcp","timeout":900000}}}`, `"timeout": 900000`},
	} {
		t.Run(tc.target, func(t *testing.T) {
			dir, home := t.TempDir(), t.TempDir()
			writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nbuiltins: []\n")
			writeProjectFile(t, dir, tc.nativePath, tc.native)
			for _, args := range [][]string{{"import", tc.target}, {"validate"}, {"sync"}, {"sync", "--check"}, {"lint"}} {
				cmd := exec.Command(binary, args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "AGNOSTIC_AI_HOME="+filepath.Join(home, "source"), "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			body := readProjectFile(t, dir, tc.nativePath)
			if !strings.Contains(body, tc.expected) {
				t.Errorf("native field lost: %s", body)
			}
		})
	}
}

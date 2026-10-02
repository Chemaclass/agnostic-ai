package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportContinueMCP_CredentialsBecomeReferences(t *testing.T) {
	for _, tc := range []struct {
		name, file, body string
	}{
		{"flat YAML", "gh.yaml", "name: gh\ncommand: gh-mcp\nenv:\n  TOKEN: harmless-token\n  KEEP: ${{ secrets.KEEP }}\nheaders:\n  Authorization: Bearer harmless-header\n"},
		{"block YAML", "gh.yaml", "name: gh\nversion: 1.0.0\nschema: v1\nmcpServers:\n  - name: gh\n    command: gh-mcp\n    env:\n      TOKEN: harmless-token\n      KEEP: ${{secrets.KEEP}}\n    requestOptions:\n      headers:\n        Authorization: Bearer harmless-header\n"},
		{"bare JSON", "gh.json", `{"command":"gh-mcp","env":{"TOKEN":"harmless-token","KEEP":"${{ secrets.KEEP }}"},"headers":{"Authorization":"Bearer harmless-header"}}`},
		{"named JSON", "servers.json", `{"mcpServers":{"gh":{"command":"gh-mcp","env":{"TOKEN":"harmless-token","KEEP":"${{ secrets.KEEP }}"},"headers":{"Authorization":"Bearer harmless-header"}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			log := captureLog(t)
			writeFile(t, filepath.Join(dir, continueMCPServersDir, tc.file), tc.body)
			if count, err := importContinueMCPs(dir, filepath.Join(dir, "mcps")); err != nil || count != 1 {
				t.Fatalf("import = %d, %v", count, err)
			}
			got := readFile(t, filepath.Join(dir, "mcps", "gh.yaml"))
			for _, secret := range []string{"harmless-token", "harmless-header"} {
				if strings.Contains(got+log.String(), secret) {
					t.Errorf("literal credential survived import")
				}
			}
			for _, want := range []string{"TOKEN: ${TOKEN}", "KEEP: ${KEEP}", "Authorization: Bearer ${GH_AUTHORIZATION}"} {
				if !strings.Contains(got, want) {
					t.Errorf("spec lacks %q: %s", want, got)
				}
			}
			for _, want := range []string{"MCP server gh: env TOKEN", "set TOKEN", "MCP server gh: headers Authorization", "set GH_AUTHORIZATION", ".continue/.env"} {
				if !strings.Contains(log.String(), want) {
					t.Errorf("output lacks %q", want)
				}
			}
		})
	}
}

func TestImportContinueMCP_VariableNamesDoNotCollideAcrossFiles(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureLog(t)
	for name, value := range map[string]string{"a": "harmless-one", "b": "harmless-two"} {
		writeFile(t, filepath.Join(dir, continueMCPServersDir, name+".yaml"),
			"name: "+name+"\ncommand: server\nenv:\n  TOKEN: "+value+"\n")
	}
	if _, err := importContinueMCPs(dir, filepath.Join(dir, "mcps")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		got := readFile(t, filepath.Join(dir, "mcps", name+".yaml"))
		if want := "TOKEN: ${" + strings.ToUpper(name) + "_TOKEN}"; !strings.Contains(got, want) {
			t.Errorf("spec lacks %q: %s", want, got)
		}
	}
}

func TestImportContinueMCP_YAMLScalarCredentialsBecomeReferences(t *testing.T) {
	dir := testutil.TempCwd(t)
	log := captureLog(t)
	writeFile(t, filepath.Join(dir, continueMCPServersDir, "gh.yaml"),
		"name: gh\ncommand: gh-mcp\nenv:\n  TOKEN: 87654321\nrequestOptions:\n  1: option\n  headers:\n    123: harmless-header\n    Authorization: Bearer harmless-token\n    Keep: ${{ secrets.KEEP }}\n")
	if _, err := importContinueMCPs(dir, filepath.Join(dir, "mcps")); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "mcps/gh.yaml"))
	for _, secret := range []string{"87654321", "harmless-header", "harmless-token"} {
		if strings.Contains(got+log.String(), secret) {
			t.Error("scalar credential survived import")
		}
	}
	for _, want := range []string{"TOKEN: ${TOKEN}", "${GH_123}", "Authorization: Bearer ${GH_AUTHORIZATION}", "Keep: ${KEEP}"} {
		if !strings.Contains(got, want) {
			t.Errorf("spec lacks %q", want)
		}
	}
}

func TestImportContinueMCP_InvalidCredentialMapsFailBeforeWriting(t *testing.T) {
	for _, field := range []string{
		"env: harmless-secret\n", "headers: [{Authorization: harmless-secret}]\n",
		"requestOptions: [{headers: {Authorization: harmless-secret}}]\n",
		"env:\n  TOKEN: !!int harmless-secret\n",
		"env: {123: harmless-one, \"123\": harmless-two}\n",
	} {
		t.Run(field, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			log := captureLog(t)
			writeFile(t, filepath.Join(dir, continueMCPServersDir, "gh.yaml"), "name: gh\ncommand: gh-mcp\n"+field)
			count, err := importContinueMCPs(dir, filepath.Join(dir, "mcps"))
			if err == nil || count != 0 {
				t.Errorf("invalid credential map import = %d, %v", count, err)
			}
			if err != nil && strings.Contains(err.Error()+log.String(), "harmless-secret") {
				t.Error("parse failure exposed a credential")
			}
			if _, err := os.Stat(filepath.Join(dir, "mcps", "gh.yaml")); !os.IsNotExist(err) {
				t.Errorf("invalid credential map wrote a spec: %v", err)
			}
		})
	}
}

func TestImportContinueMCP_RejectsInvalidBlockWrappers(t *testing.T) {
	for _, body := range []string{
		"name: gh\nmcpServers:\n  gh:\n    command: gh-mcp\n    env:\n      TOKEN: harmless-secret\n",
		"name: gh\nmcpServers: []\n",
		"name: gh\nmcpServers:\n  - name: first\n    command: gh-mcp\n  - name: second\n    command: gh-mcp\n    env:\n      TOKEN: harmless-secret\n",
	} {
		t.Run(body, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			writeFile(t, filepath.Join(dir, continueMCPServersDir, "gh.yaml"), body)
			if count, err := importContinueMCPs(dir, filepath.Join(dir, "mcps")); err == nil || count != 0 {
				t.Errorf("invalid block import = %d, %v", count, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "mcps/gh.yaml")); !os.IsNotExist(err) {
				t.Errorf("invalid wrapper wrote a spec: %v", err)
			}
		})
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const opencodeProviderNative = `{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "anthropic": {
      "models": {
        "claude-x": {
          "limit": {
            "context": 1000000,
            "output": 64000
          },
          "settings": {
            "cache": {
              "ttl": "1h"
            }
          }
        }
      },
      "options": {
        "apiKey": "{env:ANTHROPIC_API_KEY}"
      }
    }
  }
}
`

func TestImportFromOpencode_ProviderBlockRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	writeFile(t, opencodeMCPFile, opencodeProviderNative)

	runOpencodeProviderCmd(t, "import", "opencode")
	var spec struct {
		Target    string `yaml:"target"`
		XOpencode struct {
			Provider map[string]any `yaml:"provider"`
		} `yaml:"x-opencode"`
	}
	raw := readFile(t, filepath.Join(".agnostic-ai", "settings", opencodeProviderSpecFile))
	if err := yaml.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("parse imported settings spec: %v", err)
	}
	if spec.Target != "opencode" || spec.XOpencode.Provider["anthropic"] == nil {
		t.Errorf("imported spec does not hold the provider block for opencode:\n%s", raw)
	}
	for _, want := range []string{"ttl: 1h", "context: 1000000"} {
		if !strings.Contains(raw, want) {
			t.Errorf("imported spec lacks %q:\n%s", want, raw)
		}
	}

	runOpencodeProviderCmd(t, "sync")
	if got := readFile(t, opencodeMCPFile); got != opencodeProviderNative {
		t.Errorf("sync after import changed opencode.json:\n%s", got)
	}
	runOpencodeProviderCmd(t, "sync", "--check")
	runOpencodeProviderCmd(t, "lint")

	if err := os.Remove(opencodeMCPFile); err != nil {
		t.Fatal(err)
	}
	runOpencodeProviderCmd(t, "sync")
	if got := readFile(t, opencodeMCPFile); got != opencodeProviderNative {
		t.Errorf("sync from imported specs did not rebuild opencode.json:\n%s", got)
	}
}

func TestImportFromOpencode_ProviderWithLiteralCredentialStaysNative(t *testing.T) {
	dir := t.TempDir()
	captured := captureLogOut(t)
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"provider":{"anthropic":{"options":{"apiKey":"sk-ant-api03-TOPSECRETTOPSECRETTOPSECRET"},"models":{"claude-x":{"settings":{"cache":{"ttl":"1h"}}}}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings", opencodeProviderSpecFile)); !os.IsNotExist(err) {
		t.Errorf("import copied a provider block holding a credential: %v", err)
	}
	if got := captured.String(); !strings.Contains(got, "provider.anthropic.options.apiKey") || strings.Contains(got, "TOPSECRET") {
		t.Errorf("warning = %q, want the credential path without its value", got)
	}
}

func TestImportFromOpencode_ProviderReferencesAreNotCredentials(t *testing.T) {
	dir := t.TempDir()
	captureLogOut(t)
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"provider":{"a":{"options":{"apiKey":"{file:~/.config/key.txt}"}},"b":{"options":{"apiKey":"{env:A}{env:B}"}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, "settings", opencodeProviderSpecFile))
	for _, want := range []string{"{file:~/.config/key.txt}", "{env:A}{env:B}"} {
		if !strings.Contains(got, want) {
			t.Errorf("imported spec lacks %q:\n%s", want, got)
		}
	}
}

func TestImportFromOpencode_ProviderReimportReplacesItsOwnSpec(t *testing.T) {
	dir := t.TempDir()
	captureLogOut(t)
	native := filepath.Join(dir, opencodeMCPFile)
	writeFile(t, native, `{"provider":{"a":{"models":{"m":{"settings":{"cache":{"ttl":"1h"}}}}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	writeFile(t, native, `{"provider":{"a":{"models":{"m":{"settings":{"cache":{"ttl":"5m"}}}}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "settings", opencodeProviderSpecFile)); !strings.Contains(got, "ttl: 5m") {
		t.Errorf("re-import kept the old block:\n%s", got)
	}
}

func TestImportFromOpencode_ProviderMatchingOwnerSpecIsKept(t *testing.T) {
	dir := t.TempDir()
	captureLogOut(t)
	base := "x-opencode:\n  provider:\n    a:\n      limit:\n        context: 1000000\n"
	writeFile(t, filepath.Join(dir, "settings", "base.yaml"), base)
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"provider":{"a":{"limit":{"context":1000000}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatalf("import of a block a spec already sets: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings", opencodeProviderSpecFile)); !os.IsNotExist(err) {
		t.Errorf("import wrote a second owner of the provider block: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "settings", "base.yaml")); got != base {
		t.Errorf("import changed the owner spec:\n%s", got)
	}
}

func TestImportFromOpencode_ProviderEnvDefaultIsACredential(t *testing.T) {
	dir := t.TempDir()
	captureLogOut(t)
	writeFile(t, filepath.Join(dir, opencodeMCPFile), `{"provider":{"a":{"options":{"apiKey":"{env:API_KEY:-sk-ant-api03-TOPSECRETTOPSECRETTOPSECRET}"}}}}`)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings", opencodeProviderSpecFile)); !os.IsNotExist(err) {
		t.Errorf("import copied a literal fallback credential: %v", err)
	}
}

func TestImportFromOpencode_ProviderRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, file, body, want string
	}{
		{"another spec owns provider", "base.yaml", "x-opencode:\n  provider:\n    a: {}\n", "differs from settings spec"},
		{"older spec would replace a credential", opencodeProviderSpecFile, "target: opencode\nx-opencode:\n  provider:\n    a: {}\n", "provider.a.options.apiKey holds a credential"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			captureLogOut(t)
			writeFile(t, filepath.Join(dir, "settings", tc.file), tc.body)
			original := `{"provider":{"a":{"options":{"apiKey":"sk-ant-api03-TOPSECRETTOPSECRETTOPSECRET"}}}}`
			if tc.file == "base.yaml" {
				original = `{"provider":{"a":{"models":{"m":{"settings":{"cache":{"ttl":"1h"}}}}}}}`
			}
			writeFile(t, filepath.Join(dir, opencodeMCPFile), original)
			err := importFromOpencode(dir, rootSources())
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "TOPSECRET") {
				t.Errorf("import error = %v, want %q without the credential", err, tc.want)
			}
			if got := readFile(t, filepath.Join(dir, "settings", tc.file)); got != tc.body {
				t.Errorf("refusal changed %s:\n%s", tc.file, got)
			}
			for _, source := range []string{"rules", "agents", "mcps"} {
				if _, err := os.Stat(filepath.Join(dir, source)); !os.IsNotExist(err) {
					t.Errorf("refusal created source %s: %v", source, err)
				}
			}
		})
	}
}

func runOpencodeProviderCmd(t *testing.T, args ...string) {
	t.Helper()
	command := NewRootCmd("test")
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestGeneratedFilesMatchMetadata(t *testing.T) {
	if err := generate("../../docs/site", adapters.Names(), config.DefaultTargets(), true); err != nil {
		t.Fatal(err)
	}
}

func TestGenerate_CountsFollowSupportedAndDefaultMembership(t *testing.T) {
	root := fixtureSite(t)
	cases := []struct {
		name      string
		supported []string
		defaults  []string
		wantData  string
		wantClaim string
		wantLink  string
	}{
		{"initial", []string{"claude", "codex"}, []string{"claude"}, "supported = 2\ndefault = 1\n", "supports 2 coding tools,\nwith 1 enabled by default", "all 2 tools"},
		{"add opt-in", []string{"claude", "codex", "grok"}, []string{"claude"}, "supported = 3\ndefault = 1\n", "supports 3 coding tools,\nwith 1 enabled by default", "all 3 tools"},
		{"enable existing target", []string{"claude", "codex", "grok"}, []string{"claude", "codex"}, "supported = 3\ndefault = 2\n", "supports 3 coding tools,\nwith 2 enabled by default", "all 3 tools"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := generate(root, tc.supported, tc.defaults, false); err != nil {
				t.Fatal(err)
			}
			data := readFile(t, filepath.Join(root, "data/targets.toml"))
			if !bytes.HasSuffix(data, []byte(tc.wantData)) {
				t.Errorf("target data = %s, want counts %s", data, tc.wantData)
			}
			llms := readFile(t, filepath.Join(root, "static/llms.txt"))
			for _, want := range []string{tc.wantClaim, tc.wantLink} {
				if !strings.Contains(string(llms), want) {
					t.Errorf("llms.txt does not contain %q", want)
				}
			}
		})
	}
}

func TestGenerate_IsIdempotentAndCheckRejectsStaleOutput(t *testing.T) {
	root := fixtureSite(t)
	supported, defaults := []string{"codex", "claude", "grok"}, []string{"claude", "codex"}
	if err := generate(root, supported, defaults, false); err != nil {
		t.Fatal(err)
	}
	paths := []string{"data/targets.toml", "static/llms.txt"}
	before := make(map[string][]byte)
	for _, path := range paths {
		before[path] = readFile(t, filepath.Join(root, path))
	}
	if err := generate(root, []string{"grok", "claude", "codex"}, defaults, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if got := readFile(t, filepath.Join(root, path)); !bytes.Equal(got, before[path]) {
			t.Errorf("%s changed on a second generation", path)
		}
	}
	if err := generate(root, supported, defaults, true); err != nil {
		t.Fatalf("fresh output failed check: %v", err)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			fullPath := filepath.Join(root, path)
			if err := os.WriteFile(fullPath, []byte("stale\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := generate(root, supported, defaults, true)
			if err == nil || !strings.Contains(err.Error(), filepath.FromSlash(path)) {
				t.Errorf("stale %s check = %v, want an error naming the file", path, err)
			}
			if got := string(readFile(t, fullPath)); got != "stale\n" {
				t.Errorf("check rewrote %s", path)
			}
			if err := generate(root, supported, defaults, false); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(fullPath); err != nil {
				t.Fatal(err)
			}
			err = generate(root, supported, defaults, true)
			if err == nil || !strings.Contains(err.Error(), filepath.FromSlash(path)) {
				t.Errorf("missing %s check = %v, want an error naming the file", path, err)
			}
			if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
				t.Errorf("check created missing %s", path)
			}
			if err := generate(root, supported, defaults, false); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := generate(root, append(supported, "kimi"), defaults, true); err == nil {
		t.Error("changed registry membership passed the stale output check")
	}
	if err := generate(root, supported, []string{"claude"}, true); err == nil {
		t.Error("changed default membership passed the stale output check")
	}
}

func fixtureSite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := readFile(t, "../../docs/site/llms.txt.tmpl")
	if err := os.WriteFile(filepath.Join(root, "llms.txt.tmpl"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

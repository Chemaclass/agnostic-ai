package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_AddThenRemoveRestoresTheInstructionsFileBytes(t *testing.T) {
	shapes := []struct {
		name string
		text string
	}{
		{"crlf with a leading blank line and no final newline", "\r\n# Mine\r\nline one\r\nline two"},
		{"crlf with a final newline", "# Mine\r\nline one\r\n"},
		{"leading blank lines", "\n\n# Mine\nline one\n"},
		{"no final newline", "# Mine\nline one"},
		{"extra trailing blank lines", "# Mine\nline one\n\n\n"},
		{"one final newline", "Personal notes.\n"},
	}
	files := []struct {
		target, file string
	}{
		{"claude", filepath.Join(".claude", "CLAUDE.md")},
		{"codex", filepath.Join(".codex", "AGENTS.md")},
	}
	for _, f := range files {
		for _, shape := range shapes {
			t.Run(f.target+"/"+shape.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(home, "source"))
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
				intro := filepath.Join(home, "source", "AGNOSTIC_AI.md")
				mustWriteGlobalTest(t, intro, "Shared instructions\n")
				path := filepath.Join(home, f.file)
				mustWriteGlobalTest(t, path, shape.text)

				syncGlobalOnly(t, f.target)
				added := readGlobalTestFile(t, path)
				inserted, ok := strings.CutPrefix(added, shape.text)
				if !ok {
					t.Fatalf("add changed the user's text:\n got %q\nwant prefix %q", added, shape.text)
				}
				block := strings.TrimLeft(inserted, "\n")
				if len(inserted)-len(block) > 1 || !strings.HasPrefix(block, globalStart) || !strings.HasSuffix(block, globalEnd+"\n") {
					t.Errorf("add inserted more than one separator and the block: %q", inserted)
				}

				if err := os.Remove(intro); err != nil {
					t.Fatal(err)
				}
				syncGlobalOnly(t, f.target)
				if got := readGlobalTestFile(t, path); got != shape.text {
					t.Errorf("add then remove = %q, want %q", got, shape.text)
				}
			})
		}
	}
}

func syncGlobalOnly(t *testing.T, target string) {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--global", "--only", target})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func readGlobalTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMergeGlobalBlock_TouchesOnlyTheBlockAndItsSeparator(t *testing.T) {
	block := func(body string) string { return globalStart + "\n" + body + "\n" + globalEnd }
	removal := block("")
	for _, tc := range []struct {
		name, file, managed, want string
	}{
		{"removes from a file the previous version wrote", "Mine\n\n" + block("old") + "\n", removal, "Mine\n"},
		{"keeps text after the block on update", "top\n\n" + block("old") + "\nbottom", block("new"), "top\n\n" + block("new") + "\nbottom"},
		{"keeps text after the block on removal", "top\n\n" + block("old") + "\n\nbottom\n", removal, "top\n\nbottom\n"},
		{"removes a block an editor turned to crlf", "Mine\r\n\r\n" + block("old") + "\r\n", removal, "Mine\r\n"},
		{"adds after a whitespace-only file", "\n", block("new"), "\n\n" + block("new") + "\n"},
		{"removes from a whitespace-only file", "\n\n" + block("old") + "\n", removal, "\n"},
		{"removes the whole file sync created", block("old") + "\n", removal, ""},
		{"leaves a file without a block as it is", "Mine", removal, "Mine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "CLAUDE.md")
			mustWriteGlobalTest(t, path, tc.file)
			got, err := mergeGlobalBlock(path, tc.managed)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("merge = %q, want %q", got, tc.want)
			}
		})
	}
}

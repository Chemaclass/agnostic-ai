package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// A run shares one render per target among its readers and writes with
// a second emit of the same bundle, so an emit must leave the bundle as
// it found it: a second render of it gives the same files.
func TestRender_LeavesTheBundleAsItFound(t *testing.T) {
	names := adapters.Names()
	for _, tc := range []struct {
		name   string
		shared string
	}{
		{"translated and per-target permissions", "model: lite\npermissions:\n  allow: [Read]\n  deny: [\"Bash(rm *)\"]\n"},
		{"per-target permissions only", "model: lite\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			chdir(t, root)
			t.Setenv("HOME", t.TempDir())
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			writeBenchProject(t, root, 2)
			settings := tc.shared
			for _, name := range names {
				settings += "x-" + name + ":\n  permissions:\n    deny: [\"Bash(rm *)\"]\n"
			}
			mustWriteFile(t, filepath.Join(root, ".agnostic-ai", "settings", "team.yaml"), settings)
			mustWriteFile(t, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(names, ", ")+"]\nsync:\n  collision-policy: prefer-spec\n")
			cfg, b, err := loadProject(".")
			if err != nil {
				t.Fatal(err)
			}
			defer adapters.SetAsideNotes()()

			for _, name := range names {
				adapter, err := adapters.Resolve(name)
				if err != nil {
					t.Fatal(err)
				}
				before, err := json.Marshal(b)
				if err != nil {
					t.Fatal(err)
				}
				first, firstErr := captureAdapterFiles(adapters.NewSession(), adapter, b, cfg)
				if after, _ := json.Marshal(b); !bytes.Equal(before, after) {
					t.Errorf("%s: rendering changed the bundle", name)
				}
				second, secondErr := captureAdapterFiles(adapters.NewSession(), adapter, b, cfg)

				if (firstErr == nil) != (secondErr == nil) || (firstErr != nil && firstErr.Error() != secondErr.Error()) {
					t.Errorf("%s: first render returned %v, second %v", name, firstErr, secondErr)
					continue
				}
				if reflect.DeepEqual(first, second) {
					continue
				}
				if len(first) != len(second) {
					t.Errorf("%s: %d files on the first render, %d on the second", name, len(first), len(second))
				}
				for i := range min(len(first), len(second)) {
					if first[i].Path != second[i].Path || first[i].Content != second[i].Content {
						t.Errorf("%s: %s changed on the second render:\n%s", name, first[i].Path, labeledDiff("first", "second", splitLines(first[i].Content), splitLines(second[i].Content), 20))
						break
					}
				}
			}
		})
	}
}

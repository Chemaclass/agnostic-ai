package builtins_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/builtins"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestToolBuiltins_PreservePinnedPackSources(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	packs := filepath.Join(packageDir, "..", "..", "docs", "examples", "rtk-and-caveman", "packs")
	for _, name := range []string{"rtk", "caveman"} {
		t.Run(name, func(t *testing.T) {
			cacheForTest(t)
			root, cleanup, err := builtins.Materialize(name)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := cleanup(); err != nil {
					t.Error(err)
				}
			})
			err = filepath.WalkDir(filepath.Join(packs, name), func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				relative, err := filepath.Rel(filepath.Join(packs, name), path)
				if err != nil {
					return err
				}
				want, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				got, err := os.ReadFile(filepath.Join(root, relative))
				if err != nil {
					return err
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs from pinned example", relative)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Skills: "skills", Hooks: "hooks"}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.All()) != 1 {
				t.Fatalf("entries = %+v, want one independent spec", bundle.All())
			}
			if name == "rtk" {
				if len(bundle.HooksFor("claude")) != 1 || len(bundle.HooksFor("codex")) != 0 {
					t.Errorf("RTK must have one Claude hook and no Codex hook: %+v", bundle.Hooks)
				}
				return
			}
			if len(bundle.Skills) != 1 || bundle.Skills[0].Name != "caveman" {
				t.Fatalf("skills = %+v", bundle.Skills)
			}
			var provenance struct {
				Revision string `json:"revision"`
				Files    map[string]struct {
					SHA256 string `json:"sha256"`
				} `json:"files"`
			}
			data, err := os.ReadFile(filepath.Join(root, "UPSTREAM.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &provenance); err != nil {
				t.Fatal(err)
			}
			if provenance.Revision != "2e08b9177c07bb7249a8a2d1a6758e5db281d002" || len(provenance.Files) != 6 {
				t.Fatalf("unexpected pinned provenance: %+v", provenance)
			}
			for file, source := range provenance.Files {
				data, err := os.ReadFile(filepath.Join(root, "skills", "caveman", file))
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(data)
				if hex.EncodeToString(digest[:]) != source.SHA256 {
					t.Errorf("%s differs from pinned digest", file)
				}
			}
		})
	}
}

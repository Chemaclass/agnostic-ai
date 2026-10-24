package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBundleMatchesSources(t *testing.T) {
	data, err := os.ReadFile("embedded/.manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version int               `json:"version"`
		Inputs  map[string]string `json:"inputs"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || len(manifest.Inputs) == 0 || len(manifest.Files) == 0 {
		t.Fatal("invalid bundled UI manifest")
	}
	inputs := []string{"index.html", "package.json", "package-lock.json", "tsconfig.json", "vite.config.ts"}
	for _, directory := range []string{"src", "scripts"} {
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				inputs = append(inputs, filepath.ToSlash(path))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(inputs) != len(manifest.Inputs) {
		t.Error("UI input list changed; run npm run bundle in ui/")
	}
	for _, path := range inputs {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != manifest.Inputs[path] {
			t.Errorf("%s changed; run npm run bundle in ui/", path)
		}
	}
	var assets []string
	err = fs.WalkDir(Files(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			assets = append(assets, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(assets, "index.html") || len(assets) != len(manifest.Files) {
		t.Fatal("bundled UI asset list does not match its manifest")
	}
	for _, path := range assets {
		data, err := fs.ReadFile(Files(), path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != manifest.Files[path] {
			t.Errorf("bundled asset %s changed; run npm run bundle in ui/", path)
		}
	}
}

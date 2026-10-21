package integration

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const landingExplorerData = "../../docs/site/data/explorer.toml"

type landingExplorerFile struct {
	Path string `toml:"path"`
	Part string `toml:"part,omitempty"`
	Body string `toml:"body"`
}

// landingExplorerTools is the tab row every explorer entry shows, in order.
var landingExplorerTools = []string{"Claude Code", "Codex", "Gemini CLI"}

const (
	rulesBlockStart = "<!-- agnostic-ai:rules:start -->"
	rulesBlockEnd   = "<!-- agnostic-ai:rules:end -->"
)

// TestSiteDocs_LandingExplorerMatchesSync keeps the landing explorer honest.
// Every source file it shows comes from fixtures/landing, and every generated
// file is what a real sync of that fixture writes today, trimmed only as
// forLandingDisplay says. An adapter change that moves a path or a key fails
// here instead of leaving the landing page wrong.
//
// Regenerate with: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_LandingExplorer
func TestSiteDocs_LandingExplorerMatchesSync(t *testing.T) {
	var landing struct {
		Explorer struct {
			Entries []struct {
				ID      string   `toml:"id"`
				File    string   `toml:"file"`
				Files   []string `toml:"files"`
				Outputs []struct {
					Tool string `toml:"tool"`
					Path string `toml:"path"`
					Part string `toml:"part"`
					Note string `toml:"note"`
				} `toml:"outputs"`
			} `toml:"entries"`
		} `toml:"explorer"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode landing data: %v", err)
	}
	if len(landing.Explorer.Entries) == 0 {
		t.Fatal("no explorer entries in the landing data")
	}

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	fixture := filepath.Join("fixtures", "landing")
	dir := t.TempDir()
	copyLandingFixture(t, fixture, dir)
	testutil.Chdir(t, dir)
	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync", "--gitignore=off"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync the landing fixture: %v", err)
	}

	files := map[landingExplorerKey]string{}
	read := func(entry, rel, part string) {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("explorer entry %q shows %s, which a sync of fixtures/landing does not write", entry, rel)
			return
		}
		body, err := forLandingDisplay(string(data), part)
		if err != nil {
			t.Errorf("explorer entry %q shows %s: %v", entry, rel, err)
			return
		}
		files[landingExplorerKey{path: rel, part: part}] = body
	}
	for _, entry := range landing.Explorer.Entries {
		sources := entry.Files
		if entry.File != "" {
			sources = append([]string{entry.File}, sources...)
		}
		if len(sources) == 0 {
			t.Errorf("explorer entry %q names no source file", entry.ID)
		}
		for _, source := range sources {
			read(entry.ID, ".agnostic-ai/"+source, "")
		}
		tools := make([]string, 0, len(entry.Outputs))
		for _, output := range entry.Outputs {
			tools = append(tools, output.Tool)
			switch {
			case (output.Path == "") == (output.Note == ""):
				t.Errorf("explorer entry %q tab %q needs either a path sync writes or a note, not both or neither", entry.ID, output.Tool)
			case output.Note != "":
				if output.Part != "" {
					t.Errorf("explorer entry %q tab %q has a note, so it shows no part of a file", entry.ID, output.Tool)
				}
			default:
				read(entry.ID, output.Path, output.Part)
			}
		}
		if strings.Join(tools, ", ") != strings.Join(landingExplorerTools, ", ") {
			t.Errorf("explorer entry %q shows tabs %v, want %v", entry.ID, tools, landingExplorerTools)
		}
	}
	if t.Failed() {
		return
	}

	keys := make([]landingExplorerKey, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].part < keys[j].part
	})
	var data struct {
		Files []landingExplorerFile `toml:"files"`
	}
	for _, key := range keys {
		data.Files = append(data.Files, landingExplorerFile{Path: key.path, Part: key.part, Body: files[key]})
	}
	var want bytes.Buffer
	want.WriteString("# Generated from tests/integration/fixtures/landing by a real sync. Do not edit.\n")
	want.WriteString("# Regenerate with: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_LandingExplorer\n\n")
	if err := toml.NewEncoder(&want).Encode(data); err != nil {
		t.Fatalf("encode explorer data: %v", err)
	}

	target := filepath.Join(packageDir, landingExplorerData)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(target, want.Bytes(), 0o644); err != nil {
			t.Fatalf("write explorer data: %v", err)
		}
		t.Logf("explorer data updated: %s (%d files)", target, len(data.Files))
		return
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read explorer data: %v (run UPDATE_GOLDEN=1 to create it)", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("%s no longer matches a sync of fixtures/landing; run UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestSiteDocs_LandingExplorer and review the diff", landingExplorerData)
	}
}

type landingExplorerKey struct {
	path string
	part string
}

// forLandingDisplay is the only trimming between a synced file and the landing
// pane. The pane is narrow and readers never edit these files, so it drops
// markup that explains nothing on its own:
//   - the "Generated by agnostic-ai" banner line every output carries;
//   - the agnostic-ai:rules start and end marker lines around the rules block;
//   - with part "instructions", the whole rules block, because the rules/
//     entry shows where rules land.
//
// Each dropped line also takes the blank line after it.
func forLandingDisplay(body, part string) (string, error) {
	switch part {
	case "":
	case "instructions":
		if before, _, found := strings.Cut(body, rulesBlockStart); found {
			body = strings.TrimRight(before, "\n") + "\n"
		}
	default:
		return "", fmt.Errorf("unknown part %q; the landing shows a whole file or its instructions", part)
	}
	return withoutLines(body, func(line string) bool {
		return isProvenanceBanner(line) || line == rulesBlockStart || line == rulesBlockEnd
	}), nil
}

// withoutProvenanceBanner drops the banner line sync writes into every output,
// and the blank line after it.
func withoutProvenanceBanner(body string) string {
	return withoutLines(body, isProvenanceBanner)
}

func isProvenanceBanner(line string) bool {
	return strings.Contains(line, "Generated by agnostic-ai. Do not edit this file directly")
}

// withoutLines drops every line drop matches, and the blank line after each.
func withoutLines(body string, drop func(string) bool) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	for index := 0; index < len(lines); index++ {
		if drop(lines[index]) {
			if index+1 < len(lines) && lines[index+1] == "" {
				index++
			}
			continue
		}
		kept = append(kept, lines[index])
	}
	return strings.Join(kept, "\n")
}

func copyLandingFixture(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	}); err != nil {
		t.Fatalf("copy landing fixture: %v", err)
	}
}

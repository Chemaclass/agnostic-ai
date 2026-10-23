package integration

import (
	"bytes"
	"encoding/json"
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

const rulesBlockStart = "<!-- agnostic-ai:rules:start -->"

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
		body := string(data)
		if strings.HasPrefix(rel, ".agnostic-ai/") {
			files[landingExplorerKey{path: rel, part: part}] = body
			return
		}
		body, err = forLandingDisplay(body, part)
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

// Excerpts select generated values, then omit sync's provenance comments.
func forLandingDisplay(body, part string) (string, error) {
	if format, fields, found := strings.Cut(part, ":"); found {
		if format == "rule" {
			heading := "### " + fields + "\n"
			_, section, found := strings.Cut(body, heading)
			if !found {
				return "", fmt.Errorf("rule %q is missing from generated output", fields)
			}
			section, _, _ = strings.Cut(section, "\n### ")
			section, _, _ = strings.Cut(section, "<!-- agnostic-ai:rules:end -->")
			body = heading + strings.TrimRight(section, "\n") + "\n"
		} else {
			return landingConfigExcerpt(body, format, strings.Split(fields, ","))
		}
		part = ""
	}
	switch part {
	case "":
	case "instructions":
		if before, _, found := strings.Cut(body, rulesBlockStart); found {
			body = strings.TrimRight(before, "\n") + "\n"
		}
	default:
		return "", fmt.Errorf("unknown landing excerpt %q", part)
	}
	body = withoutLines(body, func(line string) bool {
		return isProvenanceBanner(line) || strings.HasPrefix(line, "<!-- agnostic-ai:") || strings.HasPrefix(line, "<!-- source: ")
	})
	return withoutRepeatedBlankLines(body), nil
}

func landingConfigExcerpt(body, format string, fields []string) (string, error) {
	var config map[string]any
	switch format {
	case "json":
		if err := json.Unmarshal([]byte(body), &config); err != nil {
			return "", fmt.Errorf("parse JSON excerpt: %w", err)
		}
	case "toml":
		if _, err := toml.Decode(body, &config); err != nil {
			return "", fmt.Errorf("parse TOML excerpt: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown excerpt format %q", format)
	}
	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		value, found := config[field]
		if !found {
			return "", fmt.Errorf("excerpt field %q is missing from generated output", field)
		}
		selected[field] = value
	}
	if format == "json" {
		data, err := json.MarshalIndent(selected, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode JSON excerpt: %w", err)
		}
		return string(data) + "\n", nil
	}
	var output bytes.Buffer
	if err := toml.NewEncoder(&output).Encode(selected); err != nil {
		return "", fmt.Errorf("encode TOML excerpt: %w", err)
	}
	return output.String(), nil
}

func withoutRepeatedBlankLines(body string) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	for index, line := range lines {
		if line == "" && index > 0 && lines[index-1] == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
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

func TestSiteDocs_LandingExplorerDropsOnlySyncComments(t *testing.T) {
	synced := "<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\n" +
		"# Billing API\n\n<!-- keep this note -->\n\n" + rulesBlockStart + "\n\n## Rules\n\n### testing\n\n" +
		"<!-- source: .agnostic-ai/rules/testing.md scope: 1 -->\nOne behavior per test.\n\n\n<!-- agnostic-ai:rules:end -->\n"
	got, err := forLandingDisplay(synced, "")
	if err != nil {
		t.Fatalf("forLandingDisplay: %v", err)
	}
	want := "# Billing API\n\n<!-- keep this note -->\n\n## Rules\n\n### testing\n\nOne behavior per test.\n"
	if got != want {
		t.Errorf("forLandingDisplay kept or dropped the wrong lines:\ngot  %q\nwant %q", got, want)
	}
}

func TestSiteDocs_LandingExplorerFocusesOnSelectedSource(t *testing.T) {
	for _, tt := range []struct {
		name, body, part, want, unrelated string
	}{
		{
			name:      "local rule",
			body:      "# Billing API\n\n## Rules\n\n### testing\n\nRun tests.\n\n### my-style\n\n<!-- source: .agnostic-ai/local/rules/my-style.md -->\nAnswer in short sentences.\n\n<!-- agnostic-ai:rules:end -->\n",
			part:      "rule:my-style",
			want:      "### my-style\n\nAnswer in short sentences.\n",
			unrelated: "Billing API",
		},
		{
			name:      "hook settings",
			body:      `{"model":"example","hooks":{"PostToolUse":[]}}`,
			part:      "json:hooks",
			want:      "{\n  \"hooks\": {\n    \"PostToolUse\": []\n  }\n}\n",
			unrelated: "model",
		},
		{
			name:      "notification overlay",
			body:      "model = \"example\"\n\n[tui]\nnotification_condition = \"always\"\n",
			part:      "toml:tui",
			want:      "[tui]\n  notification_condition = \"always\"\n",
			unrelated: "model",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := forLandingDisplay(tt.body, tt.part)
			if err != nil {
				t.Fatalf("select excerpt: %v", err)
			}
			if got != tt.want {
				t.Errorf("excerpt = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, tt.unrelated) {
				t.Errorf("excerpt includes unrelated content %q", tt.unrelated)
			}
		})
	}
}

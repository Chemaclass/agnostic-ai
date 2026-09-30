package integration

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// fidelityDir holds the import fidelity corpus: native config copied from
// public repositories at a pinned commit (see corpus.tsv and
// scripts/fidelity-corpus.sh). Hand-written config uses layouts and
// fields no fixture of ours thinks to cover.
const fidelityDir = "../../tests/fidelity"

type fidelityEntry struct {
	name    string
	targets []string
}

// TestImportFidelity_RealRepositories imports each corpus entry with its
// tool, syncs, and compares every original file with what sync left:
// parsed JSON, TOML, YAML, and frontmatter compare as values, and our
// provenance header is ignored, so only a change a tool would read shows.
// The result must equal the entry's expected.txt, the list of known gaps.
// A new gap fails, and so does a fixed one until the list drops it.
// Regenerate with: UPDATE_FIDELITY=1 go test ./tests/integration/ -run TestImportFidelity
func TestImportFidelity_RealRepositories(t *testing.T) {
	for _, entry := range readFidelityCorpus(t) {
		t.Run(entry.name, func(t *testing.T) {
			base, err := filepath.Abs(filepath.Join(fidelityDir, entry.name))
			if err != nil {
				t.Fatal(err)
			}
			original := readCorpusTar(t, filepath.Join(base, "repo.tar"))
			dir := t.TempDir()
			for rel, f := range original {
				writeCorpusFile(t, dir, rel, f)
			}
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
				[]byte("version: 1\ntargets: ["+strings.Join(entry.targets, ", ")+"]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			testutil.Chdir(t, dir)
			for _, target := range entry.targets {
				runCmd(t, "import", target)
			}
			runCmd(t, "sync")

			got := fidelityReport(t, dir, original)
			expectedPath := filepath.Join(base, "expected.txt")
			if os.Getenv("UPDATE_FIDELITY") == "1" {
				if err := os.WriteFile(expectedPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("no %s; run: UPDATE_FIDELITY=1 go test ./tests/integration/ -run TestImportFidelity/%s", expectedPath, entry.name)
			}
			if got != string(want) {
				t.Errorf("import fidelity for %s moved; review, then run UPDATE_FIDELITY=1 to accept:\n%s",
					entry.name, unifiedDiffLines(string(want), got))
			}
		})
	}
}

func readFidelityCorpus(t *testing.T) []fidelityEntry {
	t.Helper()
	f, err := os.Open(filepath.Join(fidelityDir, "corpus.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var entries []fidelityEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 6 {
			t.Fatalf("corpus.tsv row has %d columns, want 6: %q", len(cols), line)
		}
		entries = append(entries, fidelityEntry{name: cols[0], targets: strings.Split(cols[4], ",")})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("corpus.tsv lists no entries")
	}
	return entries
}

type corpusFile struct {
	body []byte
	mode fs.FileMode
}

func readCorpusTar(t *testing.T, name string) map[string]corpusFile {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("%v; run scripts/fidelity-corpus.sh", err)
	}
	defer func() { _ = f.Close() }()
	files := map[string]corpusFile{}
	r := tar.NewReader(f)
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		rel := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
			t.Fatalf("%s: entry %q leaves the corpus", name, h.Name)
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		files[rel] = corpusFile{body: body, mode: fs.FileMode(h.Mode).Perm()}
	}
	return files
}

func writeCorpusFile(t *testing.T, dir, rel string, f corpusFile) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, f.body, f.mode|0o600); err != nil {
		t.Fatal(err)
	}
}

// fidelityReport lists every original file sync changed or removed, and
// every file sync added next to them, one sorted "<status>\t<path>" line
// each. Our own source tree and bookkeeping files are not the tool's.
func fidelityReport(t *testing.T, dir string, original map[string]corpusFile) string {
	t.Helper()
	var lines []string
	roots := map[string]bool{}
	for rel, f := range original {
		roots[strings.SplitN(rel, "/", 2)[0]] = true
		now, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		switch {
		case err != nil:
			lines = append(lines, "missing\t"+rel)
		case normalizeNative(rel, now) != normalizeNative(rel, f.body):
			lines = append(lines, "changed\t"+rel)
			if os.Getenv("FIDELITY_VERBOSE") == "1" {
				t.Logf("%s:\n%s", rel, unifiedDiffLines(normalizeNative(rel, f.body), normalizeNative(rel, now)))
			}
		}
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
		if d.IsDir() {
			if rel == ".agnostic-ai" || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := original[rel]; ok || !roots[strings.SplitN(rel, "/", 2)[0]] {
			return nil
		}
		lines = append(lines, "added\t"+rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// normalizeNative prints a file the way a tool reads it: without our
// provenance header, and as a canonical value when it parses as JSON,
// TOML, or YAML, or carries YAML frontmatter. Text, and every string in a
// parsed value, compares with trailing spaces and edge blank lines trimmed.
func normalizeNative(rel string, body []byte) string {
	text := stripProvenance(string(body))
	switch strings.ToLower(path.Ext(rel)) {
	case ".json":
		var v any
		if json.Unmarshal([]byte(text), &v) == nil {
			return canonicalJSON(v)
		}
	case ".toml":
		var v map[string]any
		if _, err := toml.Decode(text, &v); err == nil {
			return canonicalJSON(v)
		}
	case ".yaml", ".yml":
		var v any
		if yaml.Unmarshal([]byte(text), &v) == nil {
			return canonicalJSON(v)
		}
	}
	if rest, ok := strings.CutPrefix(text, "---\n"); ok {
		if front, bodyText, ok := strings.Cut(rest, "\n---\n"); ok {
			var v any
			if yaml.Unmarshal([]byte(front), &v) == nil {
				return canonicalJSON(v) + "\n" + trimText(stripProvenance(bodyText))
			}
		}
	}
	return trimText(text)
}

func stripProvenance(text string) string {
	var out []string
	skipBlank := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.Contains(line, "Generated by agnostic-ai.") {
			skipBlank = true
			continue
		}
		if skipBlank && strings.TrimSpace(line) == "" {
			skipBlank = false
			continue
		}
		skipBlank = false
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func trimText(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// trimValues trims every string in a decoded document the way trimText
// trims a file: trailing spaces on each line and blank edge lines are not
// something a tool reads.
func trimValues(v any) any {
	switch x := v.(type) {
	case string:
		return trimText(x)
	case map[string]any:
		for k, e := range x {
			x[k] = trimValues(e)
		}
	case []any:
		for i, e := range x {
			x[i] = trimValues(e)
		}
	}
	return v
}

func canonicalJSON(v any) string {
	v = trimValues(v)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

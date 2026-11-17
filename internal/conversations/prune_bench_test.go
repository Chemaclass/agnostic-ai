package conversations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// marksFixture is a marks file of n entries as a long-used server keeps.
func marksFixture(b *testing.B, n int) []byte {
	b.Helper()
	file := marksFile{SchemaVersion: marksSchemaVersion, Since: "2026-01-01T00:00:00Z", Marks: map[string]markEntry{}}
	for i := range n {
		file.Marks[markKey(ToolClaude, fmt.Sprintf("%08d-1111-4111-8111-111111111111", i))] = markEntry{
			SeenAt: "2026-11-07T12:00:00Z", CompletedAt: "2026-11-07T12:00:00Z", Title: "Fix the parser", Source: sourceClaudeCode, Store: storeKey("/home/me/.claude/projects"),
			File: filepath.Join("-tmp-app", fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)+".jsonl"),
		}
	}
	raw, err := json.Marshal(file)
	if err != nil {
		b.Fatal(err)
	}
	return raw
}

// BenchmarkDecodeMarks reads the marks file as every scan does.
// status-quo decodes the fields it knows and drops the rest; proposed
// keeps unknown fields; baseline only checks the bytes are JSON.
func BenchmarkDecodeMarks(b *testing.B) {
	raw := marksFixture(b, 2000)
	type plainEntry struct {
		SeenAt          string     `json:"seenAt,omitempty"`
		CompletedAt     string     `json:"completedAt,omitempty"`
		HiddenAt        string     `json:"hiddenAt,omitempty"`
		ClosedWrittenAt string     `json:"closedWrittenAt,omitempty"`
		Title           string     `json:"title,omitempty"`
		Source          markSource `json:"source,omitempty"`
		Store           string     `json:"store,omitempty"`
		File            string     `json:"file,omitempty"`
		MissingSince    string     `json:"missingSince,omitempty"`
		LastMissAt      string     `json:"lastMissAt,omitempty"`
	}
	b.Run("status-quo", func(b *testing.B) {
		for b.Loop() {
			var file struct {
				SchemaVersion string                `json:"schemaVersion"`
				Since         string                `json:"since"`
				Marks         map[string]plainEntry `json:"marks"`
			}
			if err := json.Unmarshal(raw, &file); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("proposed", func(b *testing.B) {
		for b.Loop() {
			var file marksFile
			if err := decodeMarks(raw, &file); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("baseline", func(b *testing.B) {
		for b.Loop() {
			if !json.Valid(raw) {
				b.Fatal("invalid")
			}
		}
	})
}

// BenchmarkPruneMarks is the pass every scan makes over the marks once a
// source was read completely, when nothing changes.
func BenchmarkPruneMarks(b *testing.B) {
	var file marksFile
	if err := decodeMarks(marksFixture(b, 2000), &file); err != nil {
		b.Fatal(err)
	}
	ids := map[string]string{}
	for key := range file.Marks {
		id := key[len("claude:"):]
		ids[id] = filepath.Join("-tmp-app", id+".jsonl")
	}
	results := []sourceResult{{inventories: provenBy(sourceClaudeCode, ToolClaude, "/home/me/.claude/projects", ids)}}
	scanner := &Scanner{}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	// The first pass records the store's folder, as a server's first scan does.
	scanner.pruneMarks(&file, results, now)
	for b.Loop() {
		if scanner.pruneMarks(&file, results, now) {
			b.Fatal("a pass with every conversation listed changed the file")
		}
	}
}

// BenchmarkListTranscripts lists about 5,000 Claude Code transcripts in
// 100 project folders, each session with its own folder beside it, as
// every Claude Code scan does. status-quo is the glob the scan used,
// shipped is listTree, which also says whether every folder was read,
// and baseline is a bare os.ReadDir walk with no checks.
func BenchmarkListTranscripts(b *testing.B) {
	projects := filepath.Join(b.TempDir(), "projects")
	for p := range 100 {
		dir := filepath.Join(projects, fmt.Sprintf("-Users-me-code-project-%03d", p))
		for s := range 50 {
			id := fmt.Sprintf("%08d-1111-4111-8111-%012d", p, s)
			if err := os.MkdirAll(filepath.Join(dir, id, "subagents"), 0o755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), nil, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	const want = 5000
	b.Run("status-quo", func(b *testing.B) {
		for b.Loop() {
			if files, _ := filepath.Glob(filepath.Join(projects, "*", "*.jsonl")); len(files) != want {
				b.Fatal(len(files))
			}
		}
	})
	b.Run("shipped", func(b *testing.B) {
		for b.Loop() {
			if files, complete := listTree(projects, 1, "*.jsonl"); len(files) != want || !complete {
				b.Fatal(len(files))
			}
		}
	})
	b.Run("baseline", func(b *testing.B) {
		for b.Loop() {
			var files []string
			folders, _ := os.ReadDir(projects)
			for _, folder := range folders {
				entries, _ := os.ReadDir(filepath.Join(projects, folder.Name()))
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".jsonl") {
						files = append(files, filepath.Join(projects, folder.Name(), entry.Name()))
					}
				}
			}
			if len(files) != want {
				b.Fatal(len(files))
			}
		}
	})
}

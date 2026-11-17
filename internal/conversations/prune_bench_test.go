package conversations

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// marksFixture is a marks file of n entries as a long-used server keeps.
func marksFixture(b *testing.B, n int) []byte {
	b.Helper()
	file := marksFile{SchemaVersion: marksSchemaVersion, Since: "2026-01-01T00:00:00Z", Marks: map[string]markEntry{}}
	for i := range n {
		file.Marks[markKey(ToolClaude, fmt.Sprintf("%08d-1111-4111-8111-111111111111", i))] = markEntry{
			SeenAt: "2026-11-07T12:00:00Z", CompletedAt: "2026-11-07T12:00:00Z", Title: "Fix the parser", Source: sourceClaudeCode,
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
		MissingSince    string     `json:"missingSince,omitempty"`
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
	ids := map[string]bool{}
	for key := range file.Marks {
		ids[key[len("claude:"):]] = true
	}
	results := []sourceResult{{inventories: provenBy(sourceClaudeCode, ToolClaude, ids)}}
	scanner := &Scanner{}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	for b.Loop() {
		if scanner.pruneMarks(&file, results, now) {
			b.Fatal("a pass with every conversation listed changed the file")
		}
	}
}

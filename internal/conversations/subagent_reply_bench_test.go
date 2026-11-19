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

// benchSubagents is a session with many working subagents at once.
const benchSubagents = 50

// benchSubagentSession writes a session whose benchSubagents subagents
// all work, each transcript about 400 KB, past claudeReplyWindow, and
// ending in a 100 KB tool result after its latest message, so each
// reply needs the longer read.
func benchSubagentSession(b *testing.B) (file string, work *claudeWork, transcripts []string) {
	b.Helper()
	dir := b.TempDir()
	file = filepath.Join(dir, "projects", "demo", idSafari+".jsonl")
	subagents := claudeSubagentsDir(file)
	if err := os.MkdirAll(subagents, 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		b.Fatal(err)
	}
	line := func(value map[string]any) string {
		value["isSidechain"] = true
		data, _ := json.Marshal(value)
		return string(data) + "\n"
	}
	result := strings.Repeat("x", 100<<10)
	work = &claudeWork{agents: map[string]bool{}}
	for n := range benchSubagents {
		id := fmt.Sprintf("a%016x", n)
		var transcript strings.Builder
		transcript.WriteString(line(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": "Find the call sites"}}))
		for range 4 {
			transcript.WriteString(line(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "Looking in internal/cli first."}, {"type": "tool_use", "id": "t", "name": "Read"}}}}))
			transcript.WriteString(line(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": "t", "content": result}}}}))
		}
		path := filepath.Join(subagents, "agent-"+id+".jsonl")
		if err := os.WriteFile(path, []byte(transcript.String()), 0o644); err != nil {
			b.Fatal(err)
		}
		meta := `{"agentType":"Explore","description":"Find call sites ` + id + `"}`
		if err := os.WriteFile(filepath.Join(subagents, "agent-"+id+".meta.json"), []byte(meta), 0o644); err != nil {
			b.Fatal(err)
		}
		work.agents[id] = true
		transcripts = append(transcripts, path)
	}
	return file, work, transcripts
}

// BenchmarkClaudeHelpers lists a session's working subagents: while no
// transcript changed, and when every one was written since the last
// scan, as working subagents are. Run it on a revision before subagent
// replies for the status quo.
func BenchmarkClaudeHelpers(b *testing.B) {
	for _, written := range []bool{false, true} {
		name := "unchanged"
		if written {
			name = "each-written"
		}
		b.Run(name, func(b *testing.B) {
			file, work, transcripts := benchSubagentSession(b)
			scanner := &Scanner{}
			// A scan keeps the shown texts it checked for the next one.
			scan := func() []helper {
				scanner.shown.start()
				defer scanner.shown.finish()
				return scanner.claudeHelpers(file, work, map[string]bool{})
			}
			scan()
			at := time.Now()
			b.ResetTimer()
			for range b.N {
				if written {
					b.StopTimer()
					at = at.Add(time.Second)
					for _, path := range transcripts {
						_ = os.Chtimes(path, at, at)
					}
					b.StartTimer()
				}
				if len(scan()) != benchSubagents {
					b.Fatal("every subagent works")
				}
			}
		})
	}
}

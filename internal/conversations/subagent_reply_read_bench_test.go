package conversations

import (
	"os"
	"testing"
)

// BenchmarkReadClaudeAgentReply reads one subagent's reply from the end
// of its transcript, against the naive read of the whole file.
func BenchmarkReadClaudeAgentReply(b *testing.B) {
	_, _, transcripts := benchSubagentSession(b)
	path := transcripts[0]
	b.Run("tail", func(b *testing.B) {
		for range b.N {
			if readClaudeAgentReply(path) == "" {
				b.Fatal("the reply is in the last window")
			}
		}
	})
	b.Run("whole-file", func(b *testing.B) {
		for range b.N {
			data, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			replies := claudeReplies{subagent: true}
			eachClaudeLine(data, replies.visit)
			if replies.message == "" {
				b.Fatal("the whole file holds a reply")
			}
		}
	})
}

package conversations

import (
	"strings"
	"testing"
)

// BenchmarkShownCommand compares a stuck shell's command on a warm scan, read from shownCache, with the
// credential check and redaction a new command pays once.
func BenchmarkShownCommand(b *testing.B) {
	command := "npm run preview -- " + strings.Repeat("--watch src/reports ", 60)
	b.Run("warm", func(b *testing.B) {
		var cache shownCache
		cache.start()
		cache.get(texts{command: command})
		cache.finish()
		b.ResetTimer()
		for range b.N {
			cache.start()
			_ = cache.get(texts{command: command}).command
			cache.finish()
		}
	})
	b.Run("cold", func(b *testing.B) {
		for range b.N {
			_ = showCommand(command)
		}
	})
}

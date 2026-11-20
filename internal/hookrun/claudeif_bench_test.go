package hookrun

import (
	"strconv"
	"strings"
	"testing"
)

// BenchmarkBashRuleRuns_Nesting reads a command with depth nested `$(`
// substitutions against a rule that matches none of them. A flat list of
// the same number of commands is the baseline.
func BenchmarkBashRuleRuns_Nesting(b *testing.B) {
	for _, depth := range []int{10, 250, 1000, 4000} {
		nested := strings.Repeat("echo $(", depth) + "git push" + strings.Repeat(")", depth)
		flat := strings.Repeat("echo a; ", depth) + "git push"
		for _, subject := range []struct{ name, command string }{{"nested", nested}, {"flat", flat}} {
			b.Run(subject.name+"/"+strconv.Itoa(depth), func(b *testing.B) {
				for b.Loop() {
					bashRuleRuns("rm *", subject.command)
				}
			})
		}
	}
}

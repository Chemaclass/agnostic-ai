package hookrun

import (
	"strconv"
	"strings"
	"testing"
)

// BenchmarkBashRuleRuns_Nesting reads a command with depth nested `$(`
// substitutions. The flat command of the same length is the baseline.
func BenchmarkBashRuleRuns_Nesting(b *testing.B) {
	for _, depth := range []int{10, 1000, 10000} {
		nested := strings.Repeat("echo $(", depth) + "git push" + strings.Repeat(")", depth)
		flat := strings.Repeat("echo a; ", depth) + "git push"
		for name, command := range map[string]string{"nested": nested, "flat": flat} {
			b.Run(name+"/"+strconv.Itoa(depth), func(b *testing.B) {
				for b.Loop() {
					bashRuleRuns("git *", command)
				}
			})
		}
	}
}

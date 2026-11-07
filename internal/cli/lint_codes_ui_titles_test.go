package cli

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// The web client leads each finding card with a plain title keyed by code.
func TestLintCodes_EachHasAPlainTitleInTheWebClient(t *testing.T) {
	src, err := os.ReadFile("../../ui/src/model/finding-title.ts")
	if err != nil {
		t.Fatal(err)
	}
	titled := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+(LINT\d{3}):`).FindAllStringSubmatch(string(src), -1) {
		titled[m[1]] = true
	}
	var missing, unknown []string
	for code := range lintCodes {
		if !titled[code] {
			missing = append(missing, code)
		}
	}
	for code := range titled {
		if _, ok := lintCodes[code]; !ok {
			unknown = append(unknown, code)
		}
	}
	slices.Sort(missing)
	slices.Sort(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		t.Errorf("ui/src/model/finding-title.ts: missing titles %v, titles for unknown codes %v", missing, unknown)
	}
}

package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// countProjectLoads counts the project loads that reach the spec loader.
func countProjectLoads(t *testing.T) *int {
	t.Helper()
	loads := 0
	original := loadSpecLayers
	loadSpecLayers = func(layers []spec.Layer) (spec.Bundle, error) {
		loads++
		return original(layers)
	}
	t.Cleanup(func() { loadSpecLayers = original })
	return &loads
}

func TestSync_LoadsTheProjectOnce(t *testing.T) {
	for _, args := range [][]string{
		{"sync"},
		{"sync", "--json"},
		{"sync", "--dry-run"},
		{"sync", "--check"},
		{"sync", "--plan"},
		{"sync", "--plan", "--check"},
		{"sync", "--dry-run", "--diff"},
		{"sync", "--json", "--dry-run"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			syncedFixture(t)
			loads := countProjectLoads(t)

			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}

			if *loads != 1 {
				t.Errorf("%v loaded the project %d times, want 1", args, *loads)
			}
		})
	}
}

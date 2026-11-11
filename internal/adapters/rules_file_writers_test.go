package adapters

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Sync stops writing a target's entry point and delivering its rules once
// the target writes outputs.<target>.rules-file, so a target missing from
// the list loses its rule file and one listed without a writer ends up
// with no instructions file at all (#3295).
func TestRulesFileWriters_MatchAdapters(t *testing.T) {
	const rulesFile = "probe-rules.md"
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "style", Body: "Use tabs."}})
			cfg := &config.Config{
				Targets: []string{name},
				Outputs: map[string]config.Output{name: {RulesFile: rulesFile}},
			}
			a, _ := Get(name)
			sess := NewSession()
			sess.StartCapture()

			err := EmitWithProvenance(sess, a, b, cfg, false)

			files := sess.StopCapture()
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			writes := false
			for _, f := range files {
				if filepath.ToSlash(f.Path) == rulesFile {
					writes = true
				}
			}
			if writes != WritesRulesFile(name) {
				t.Errorf("adapter writes rules-file: %v, WritesRulesFile: %v; update rulesFileWriters", writes, WritesRulesFile(name))
			}
		})
	}
}

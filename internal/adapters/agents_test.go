package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Global sync reuses each adapter's agent renderer, so agent-only output
// must match what project sync writes for the same agent, and must write
// nothing else.
func TestEmitAgents_MatchesProjectOutputForEveryEmitter(t *testing.T) {
	emitters := 0
	for _, name := range Names() {
		a, _ := Get(name)
		emitter, ok := a.(AgentEmitter)
		if !ok {
			continue
		}
		emitters++
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			testutil.Chdir(t, root)
			agents := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "Review changes", "tools": []any{"Read"}}, Body: "Find correctness bugs."}}
			agentsDir := filepath.Join(root, "personal")
			if err := emitter.EmitAgents(NewSession(), agents, agentsDir, false); err != nil {
				t.Fatal(err)
			}
			personal := map[string]string{}
			err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				rel, err := filepath.Rel(agentsDir, path)
				if err != nil || strings.HasPrefix(rel, "..") {
					t.Errorf("agent-only emission wrote outside its directory: %s", path)
					return nil
				}
				data, err := os.ReadFile(path)
				personal[filepath.ToSlash(rel)] = string(data)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(personal) == 0 {
				t.Fatal("wrote no agent definition")
			}
			for rel, content := range personal {
				if !strings.Contains(content, "Find correctness bugs.") {
					t.Errorf("%s lacks the agent body:\n%s", rel, content)
				}
			}
			if err := os.RemoveAll(agentsDir); err != nil {
				t.Fatal(err)
			}
			if err := a.Emit(NewSession(), spec.NewBundle(agents), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			project := map[string]string{}
			err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				data, err := os.ReadFile(path)
				project[filepath.ToSlash(path)] = string(data)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			for rel, content := range personal {
				matched := false
				for path, data := range project {
					if strings.HasSuffix(path, "/"+rel) && data == content {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("agent-only %s has no identical project file:\n%s", rel, content)
				}
			}
		})
	}
	if emitters == 0 {
		t.Fatal("no adapter implements AgentEmitter")
	}
}

package authority

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// demoRoot is the illustrative project the view is designed around.
const demoRoot = "testdata/demo-project"

// fixedNow keeps generated views comparable.
var fixedNow = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// writeProject writes files under a new temporary project and returns
// its root.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func loadProject(t *testing.T, root string) (*config.Config, spec.Bundle) {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	b, err := spec.LoadBundle(root, cfg)
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	return cfg, b
}

func buildView(t *testing.T, root string, opts Options) *View {
	t.Helper()
	cfg, b := loadProject(t, root)
	if opts.Root == "" {
		opts.Root = root
	}
	if opts.Now.IsZero() {
		opts.Now = fixedNow
	}
	v, err := Build(cfg, b, opts)
	if err != nil {
		t.Fatalf("build view: %v", err)
	}
	return v
}

func findResource(t *testing.T, v *View, id string) Resource {
	t.Helper()
	for _, r := range v.Resources {
		if r.ID == id {
			return r
		}
	}
	ids := make([]string, 0, len(v.Resources))
	for _, r := range v.Resources {
		ids = append(ids, r.ID)
	}
	t.Fatalf("no resource %s; have %s", id, strings.Join(ids, ", "))
	return Resource{}
}

func findOp(t *testing.T, v *View, id string) Operation {
	t.Helper()
	for _, r := range v.Resources {
		for _, op := range r.Operations {
			if op.ID == id {
				return op
			}
		}
	}
	t.Fatalf("no operation %s", id)
	return Operation{}
}

func enforcementOn(t *testing.T, op Operation, target string) Enforcement {
	t.Helper()
	for _, e := range op.Enforcement {
		if e.Target == target {
			return e
		}
	}
	t.Fatalf("operation %s has no enforcement for %s", op.ID, target)
	return Enforcement{}
}

func uncertaintiesWith(v *View, code string) []Uncertainty {
	var out []Uncertainty
	for _, u := range v.Uncertainties {
		if u.Code == code {
			out = append(out, u)
		}
	}
	return out
}

func hasResourceKind(v *View, kind string) bool {
	for _, r := range v.Resources {
		if r.Kind == kind {
			return true
		}
	}
	return false
}

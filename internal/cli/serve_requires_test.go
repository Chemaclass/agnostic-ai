package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
)

func healthFields(t *testing.T, root string, global bool) map[string]json.RawMessage {
	t.Helper()
	opts := authority.HandlerOptions{Root: root, Global: global}
	opts.Requires, opts.RequiresMet = servedRequires(root, global)
	srv := httptest.NewServer(newServeMux(nil, opts))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + authority.HealthPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("health %s: %v", body, err)
	}
	return fields
}

func TestHealthNamesRequiresAndWhetherTheRunningBuildMeetsIt(t *testing.T) {
	cases := []struct {
		name, requires, running string
		wantRequires, wantMet   string
	}{
		{name: "no requires", requires: "", running: "v0.80.0"},
		{name: "met", requires: ">=0.80.0", running: "v0.81.0", wantRequires: `">=0.80.0"`, wantMet: "true"},
		{name: "unmet", requires: ">=0.81.0", running: "v0.80.0", wantRequires: `">=0.81.0"`, wantMet: "false"},
		{name: "source build", requires: ">=0.81.0", running: "(devel)", wantRequires: `">=0.81.0"`, wantMet: "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requiresProject(t, tc.requires)
			setRunningVersion(t, tc.running)
			fields := healthFields(t, ".", false)
			if got := string(fields["requires"]); got != tc.wantRequires {
				t.Errorf("requires = %s, want %s", got, tc.wantRequires)
			}
			if got := string(fields["requiresMet"]); got != tc.wantMet {
				t.Errorf("requiresMet = %s, want %s", got, tc.wantMet)
			}
		})
	}
}

func TestHealthReadsTheGlobalRequiresWhereLocalReplacesShared(t *testing.T) {
	home := t.TempDir()
	mustWriteFile(t, filepath.Join(home, "agnostic-ai.yaml"), "version: 1\nrequires: \">=0.70.0\"\n")
	mustWriteFile(t, filepath.Join(home, "local", "agnostic-ai.yaml"), "requires: \">=0.90.0\"\n")
	setRunningVersion(t, "v0.80.0")
	fields := healthFields(t, home, true)
	if got := string(fields["requires"]); got != `">=0.90.0"` {
		t.Errorf("requires = %s", got)
	}
	if got := string(fields["requiresMet"]); got != "false" {
		t.Errorf("requiresMet = %s", got)
	}
}

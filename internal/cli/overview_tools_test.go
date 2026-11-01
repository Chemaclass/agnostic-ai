package cli

import (
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestOverviewTools_ReadsSharedPersonalAndDefaultLists(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\n")
	tools := overviewTools(".", false)
	if tools.Source != "default" || len(tools.Items) != len(adapters.Names()) {
		t.Fatalf("default catalog = %+v", tools)
	}
	find := func(name string) overview.ToolChoice {
		for _, item := range tools.Items {
			if item.Target == name {
				return item
			}
		}
		t.Fatalf("%s missing", name)
		return overview.ToolChoice{}
	}
	if !find("claude").Enabled || find("amp").Enabled || find("claude").Description != "Claude Code (Anthropic)" {
		t.Errorf("defaults: %+v %+v", find("claude"), find("amp"))
	}
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	if err := os.MkdirAll(".cursor", 0o755); err != nil {
		t.Fatal(err)
	}
	tools = overviewTools(".", false)
	if tools.Source != "shared" || !find("cursor").Enabled || !find("cursor").Detected || find("claude").Enabled {
		t.Errorf("shared: %+v %+v", find("cursor"), find("claude"))
	}
	mustWrite(t, "agnostic-ai.local.yaml", "targets: [cursor, codex]\n")
	tools = overviewTools(".", false)
	if tools.Source != "personal" || !find("codex").Enabled || !find("codex").Local || find("cursor").Local {
		t.Errorf("personal: %+v %+v", find("codex"), find("cursor"))
	}
	if tools.Items[0].Target != allTargets[0].Name {
		t.Errorf("order starts with %s, want %s", tools.Items[0].Target, allTargets[0].Name)
	}
}

func TestOverviewTools_GlobalListsOnlyUserLevelTargets(t *testing.T) {
	tools := overviewTools(t.TempDir(), true)
	if tools.Source != "default" || len(tools.Items) != len(globalTargetNames()) {
		t.Fatalf("global catalog = %+v", tools)
	}
	for _, item := range tools.Items {
		if !item.Enabled || item.Detected {
			t.Errorf("%s: enabled=%v detected=%v, want on and not detected", item.Target, item.Enabled, item.Detected)
		}
	}
}

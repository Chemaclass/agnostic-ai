package overview

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func entry(kind spec.Kind, name, path string, meta map[string]any) spec.Entry {
	return spec.Entry{Kind: kind, Name: name, Path: path, Layer: "project", Meta: meta}
}

// handcraftedBundle holds at least one spec of every kind, a settings
// spec for each of the two sections that split settings, and a built-in
// rule whose cache path must never reach the view.
func handcraftedBundle() spec.Bundle {
	scoped := entry(spec.KindRule, "api-style", ".agnostic-ai/rules/api/style.md", nil)
	scoped.Scope = "api"
	builtin := entry(spec.KindRule, "shared-memory-policy", "/home/someone/.cache/agnostic-ai/builtins/0a1b/rules/shared-memory-policy.md", map[string]any{"description": "Read and save the shared memory."})
	builtin.Layer = "builtin"
	return spec.Bundle{
		Rules:        []spec.Entry{entry(spec.KindRule, "testing", ".agnostic-ai/rules/testing.md", map[string]any{"description": "Run the tests before presenting work."}), scoped, builtin},
		Skills:       []spec.Entry{entry(spec.KindSkill, "review", ".agnostic-ai/skills/review/SKILL.md", map[string]any{"description": "Review a diff."})},
		Ignores:      []spec.Entry{entry(spec.KindIgnore, "secrets", ".agnostic-ai/ignore/secrets.yaml", nil)},
		Agents:       []spec.Entry{entry(spec.KindAgent, "reviewer", ".agnostic-ai/agents/reviewer.md", map[string]any{"description": "Reviews pull requests."})},
		Hooks:        []spec.Entry{entry(spec.KindHook, "format", ".agnostic-ai/hooks/format.yaml", nil)},
		Commands:     []spec.Entry{entry(spec.KindCommand, "release", ".agnostic-ai/commands/release.md", nil)},
		MCPs:         []spec.Entry{entry(spec.KindMCP, "github", ".agnostic-ai/mcps/github.yaml", map[string]any{"description": "Repositories and pull requests."})},
		Environments: []spec.Entry{entry(spec.KindEnvironment, "dev", ".agnostic-ai/environments/dev.yaml", nil)},
		Reviews:      []spec.Entry{entry(spec.KindReview, "checklist", ".agnostic-ai/reviews/checklist.md", nil)},
		Settings: []spec.Entry{
			entry(spec.KindSettings, "tiers", ".agnostic-ai/settings/tiers.yaml", map[string]any{"model": "example-model", "effort": "high"}),
			entry(spec.KindSettings, "permissions", ".agnostic-ai/settings/permissions.yaml", map[string]any{"permissions": map[string]any{"allow": []any{"read"}}}),
			entry(spec.KindSettings, "protected", ".agnostic-ai/settings/protected.yaml", map[string]any{"protected": map[string]any{"paths": []any{"migrations/**"}}}),
		},
	}
}

func sectionByID(t *testing.T, v *View, id string) Section {
	t.Helper()
	for _, s := range v.Sections {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no section %q in %v", id, v.Sections)
	return Section{}
}

func counts(s Section) map[string]int {
	out := map[string]int{}
	for _, k := range s.Kinds {
		out[k.Label] = k.Count
	}
	return out
}

func TestBuild_SectionsCountKindsAndItemsFromTheBundle(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}}
	v := Build(Inputs{Cfg: cfg, Bundle: handcraftedBundle()})

	var ids []string
	for i, s := range v.Sections {
		ids = append(ids, s.ID)
		if s.Order != i+1 {
			t.Errorf("%s order = %d, want %d", s.ID, s.Order, i+1)
		}
	}
	if want := []string{SectionContext, SectionMemory, SectionControl, SectionAuthority, SectionTools, SectionVerification}; !slices.Equal(ids, want) {
		t.Fatalf("sections = %v, want %v", ids, want)
	}

	want := map[string]map[string]int{
		SectionContext:      {"Rules": 3, "Skills": 1, "Ignore files": 1},
		SectionMemory:       {"Shared memory": 1, "Session handoffs": 0},
		SectionControl:      {"Agents": 1, "Hooks": 1, "Commands (human-invoked prompts)": 1, "Settings": 1},
		SectionAuthority:    {"Settings": 2},
		SectionTools:        {"MCP servers": 1, "Environments": 1},
		SectionVerification: {"Review instructions": 1},
	}
	for id, wantCounts := range want {
		s := sectionByID(t, v, id)
		got := counts(s)
		if len(got) != len(wantCounts) {
			t.Errorf("%s kinds = %v, want %v", id, got, wantCounts)
		}
		for label, n := range wantCounts {
			if got[label] != n {
				t.Errorf("%s %s = %d, want %d", id, label, got[label], n)
			}
		}
		total := 0
		for _, n := range wantCounts {
			total += n
		}
		if len(s.Items) != total {
			t.Errorf("%s items = %d, want %d: %+v", id, len(s.Items), total, s.Items)
		}
	}

	context := sectionByID(t, v, SectionContext)
	if context.Question != "What is the agent told?" || context.Status != "overview" || context.DeepView != "" {
		t.Errorf("context = %+v", context)
	}
	var plain, scoped, builtin SectionItem
	for _, item := range context.Items {
		switch item.Name {
		case "testing":
			plain = item
		case "api-style":
			scoped = item
		case "shared-memory-policy":
			builtin = item
		}
	}
	if plain != (SectionItem{Kind: "rule", Name: "testing", Label: "testing", Description: "Run the tests before presenting work.", Path: ".agnostic-ai/rules/testing.md", Layer: "project"}) {
		t.Errorf("testing item = %+v", plain)
	}
	if scoped.Scope != "api" {
		t.Errorf("scoped item = %+v", scoped)
	}
	if builtin.Path != "builtin:shared-memory-policy" || builtin.Layer != "builtin" {
		t.Errorf("builtin item = %+v, want a builtin: path with no cache path", builtin)
	}

	memory := sectionByID(t, v, SectionMemory)
	if len(memory.Items) != 1 || memory.Items[0].Label != "Shared memory" || memory.Items[0].Path != "builtin:memory" || memory.Items[0].Description == "" {
		t.Errorf("memory items = %+v", memory.Items)
	}

	authoritySection := sectionByID(t, v, SectionAuthority)
	if authoritySection.Status != "deep-view" || authoritySection.DeepView != "/authority" || authoritySection.Question != "What may the agent do?" {
		t.Errorf("authority = %+v", authoritySection)
	}
	var names []string
	for _, item := range authoritySection.Items {
		names = append(names, item.Name)
	}
	if !slices.Equal(names, []string{"permissions", "protected"}) {
		t.Errorf("authority items = %v, want the settings with permissions and protected paths", names)
	}
	control := sectionByID(t, v, SectionControl)
	for _, item := range control.Items {
		if item.Kind == "settings" && (item.Name != "tiers" || item.Description != "Sets the model and the effort.") {
			t.Errorf("control settings item = %+v, want only the model and effort spec", item)
		}
	}
}

func TestBuild_ViewMarshalsEveryListAsAnArray(t *testing.T) {
	data, err := json.Marshal(Build(Inputs{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("view holds a null list: %s", data)
	}
}

func TestBuild_SupportUsesTheCapabilityMatrixForConfiguredTargets(t *testing.T) {
	targets := []string{"claude", "aider", "my-external-tool"}
	v := Build(Inputs{Cfg: &config.Config{Targets: targets}, Bundle: handcraftedBundle()})
	caps := map[string][]spec.Kind{}
	for _, tc := range adapters.CapabilityMatrix() {
		caps[tc.Name] = tc.Supports
	}

	for _, s := range v.Sections {
		for _, ks := range s.Support {
			var supported, unsupported []string
			for _, target := range []string{"claude", "aider"} {
				if slices.Contains(caps[target], spec.Kind(ks.Kind)) {
					supported = append(supported, target)
				} else {
					unsupported = append(unsupported, target)
				}
			}
			if !slices.Equal(ks.Supported, supported) {
				t.Errorf("%s %s supported = %v, want %v", s.ID, ks.Kind, ks.Supported, supported)
			}
			if !slices.Equal(ks.Unsupported, unsupported) {
				t.Errorf("%s %s unsupported = %v, want %v", s.ID, ks.Kind, ks.Unsupported, unsupported)
			}
		}
	}
	context := sectionByID(t, v, SectionContext)
	var kinds []string
	for _, ks := range context.Support {
		kinds = append(kinds, ks.Kind)
	}
	if !slices.Equal(kinds, []string{"rule", "skill", "ignore"}) {
		t.Errorf("context support kinds = %v", kinds)
	}
	if len(sectionByID(t, v, SectionMemory).Support) != 0 {
		t.Errorf("memory support = %v, want none for built-in features", sectionByID(t, v, SectionMemory).Support)
	}

	if len(v.Targets) != len(targets) {
		t.Fatalf("targets = %+v", v.Targets)
	}
	for i, ts := range v.Targets {
		if ts.Target != targets[i] {
			t.Errorf("target %d = %s, want %s", i, ts.Target, targets[i])
		}
		var want []string
		for _, k := range spec.AllKinds {
			if slices.Contains(caps[ts.Target], k) {
				want = append(want, string(k))
			}
		}
		if !slices.Equal(ts.Kinds, want) {
			t.Errorf("%s kinds = %v, want %v", ts.Target, ts.Kinds, want)
		}
	}
	if v.Targets[0].Label != "Claude Code" || v.Targets[2].Label != "my-external-tool" {
		t.Errorf("labels = %q, %q", v.Targets[0].Label, v.Targets[2].Label)
	}
}

func TestBuild_LabelsEveryBuiltInTarget(t *testing.T) {
	labels := newLabeler(nil)
	for _, name := range adapters.Names() {
		if labels.of(name) == name {
			t.Errorf("target %s has no label", name)
		}
	}
}

func TestBuild_PrefersTheAuthorityViewsTargetLabels(t *testing.T) {
	view := &authority.View{Harness: authority.Harness{Target: "claude", Label: "Claude (local)", Available: []authority.HarnessOption{{Target: "cursor", Label: "Cursor IDE"}}}}
	v := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude", "cursor"}}, Authority: view})
	if v.Targets[0].Label != "Claude (local)" || v.Targets[1].Label != "Cursor IDE" {
		t.Errorf("targets = %+v", v.Targets)
	}
}

func TestBuild_SectionsListTheCardsFiledUnderThem(t *testing.T) {
	in := Inputs{
		Cfg:    &config.Config{Targets: []string{"claude"}},
		Bundle: handcraftedBundle(),
		Findings: []Finding{
			{Source: SourceLint, Code: "LINT038", Message: "permissions.allow: bare read now covers every file read (claude: Read); use read(src/**) to name paths", Path: ".agnostic-ai/settings/permissions.yaml"},
			{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
		},
	}
	v := Build(in)
	if len(v.Feed) != 2 {
		t.Fatalf("feed = %+v", v.Feed)
	}
	for _, c := range v.Feed {
		s := sectionByID(t, v, c.SectionID)
		if !slices.Contains(s.CardIDs, c.ID) {
			t.Errorf("section %s card ids = %v, missing %s", s.ID, s.CardIDs, c.ID)
		}
	}
	if got := sectionByID(t, v, SectionContext).CardIDs; len(got) != 0 {
		t.Errorf("context card ids = %v, want none", got)
	}
}

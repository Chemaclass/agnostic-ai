package overview

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
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

func fullInputs() Inputs {
	return Inputs{
		Cfg:          &config.Config{Targets: []string{"claude"}, Builtins: []string{"memory"}, Requires: ">=0.86.0", Models: map[string]config.ModelTier{"deep": {}}},
		Bundle:       handcraftedBundle(),
		Snapshot:     authority.Snapshot{Project: authority.Project{Root: "."}, ConfigFiles: []string{"agnostic-ai.yaml"}},
		Instructions: []Document{{Path: ".agnostic-ai/AGNOSTIC_AI.md", Layer: "project", Words: 120}},
	}
}

func TestBuild_SectionsCountKindsAndItemsFromTheBundle(t *testing.T) {
	v := Build(fullInputs())

	var ids []string
	for i, s := range v.Sections {
		ids = append(ids, s.ID)
		if s.Order != i+1 {
			t.Errorf("%s order = %d, want %d", s.ID, s.Order, i+1)
		}
	}
	if want := []string{SectionContext, SectionAuthority, SectionTools, SectionLifecycle, SectionSettings}; !slices.Equal(ids, want) {
		t.Fatalf("sections = %v, want %v", ids, want)
	}

	want := map[string]map[string]int{
		SectionContext:   {"Instructions": 1, "Rules": 3, "Agents": 1, "Skills": 1, "Commands": 1, "Review instructions": 1, "Ignore files": 1},
		SectionAuthority: {"Permissions": 2},
		SectionTools:     {"MCP servers": 1, "Environments": 1},
		SectionLifecycle: {"Hooks": 1},
		SectionSettings:  {"Built-ins": 1, "Config": 1, "Other settings": 1},
	}
	for id, wantCounts := range want {
		s := sectionByID(t, v, id)
		got := counts(s)
		if len(got) != len(wantCounts) {
			t.Errorf("%s kinds = %v, want %v", id, got, wantCounts)
		}
		total := 0
		for label, n := range wantCounts {
			total += n
			if got[label] != n {
				t.Errorf("%s %s = %d, want %d", id, label, got[label], n)
			}
		}
		if id == SectionSettings {
			// Built-ins lists all five shipped built-ins and counts the enabled one.
			total += 4
		}
		if len(s.Items) != total {
			t.Errorf("%s items = %d, want %d: %+v", id, len(s.Items), total, s.Items)
		}
	}

	context := sectionByID(t, v, SectionContext)
	if context.Question != "What is the agent told?" || context.DeepView != "" {
		t.Errorf("context = %+v", context)
	}
	byName := map[string]SectionItem{}
	for _, item := range context.Items {
		byName[item.Name] = item
	}
	if it := byName["AGNOSTIC_AI.md"]; it.Kind != "instructions" || it.Path != ".agnostic-ai/AGNOSTIC_AI.md" || !strings.Contains(it.Description, "120 words") {
		t.Errorf("instructions item = %+v", it)
	}
	if it := byName["testing"]; it.Kind != "rule" || it.Description != "Run the tests before presenting work." || it.Path != ".agnostic-ai/rules/testing.md" || it.Layer != "project" {
		t.Errorf("testing item = %+v", it)
	}
	if byName["api-style"].Scope != "api" {
		t.Errorf("scoped item = %+v", byName["api-style"])
	}
	if it := byName["shared-memory-policy"]; it.Path != "builtin:shared-memory-policy" || it.Layer != "builtin" {
		t.Errorf("builtin item = %+v, want a builtin: path with no cache path", it)
	}

	authoritySection := sectionByID(t, v, SectionAuthority)
	if authoritySection.DeepView != "/authority" || authoritySection.Question != "What may the agent do?" {
		t.Errorf("authority = %+v", authoritySection)
	}
	var names []string
	for _, item := range authoritySection.Items {
		names = append(names, item.Name)
	}
	if !slices.Equal(names, []string{"permissions", "protected"}) {
		t.Errorf("authority items = %v, want the settings with permissions and protected paths", names)
	}

	settings := sectionByID(t, v, SectionSettings)
	var on []string
	for _, item := range settings.Items {
		switch item.Kind {
		case "builtin":
			if item.Enabled == nil || item.Description == "" || item.Path != "builtin:"+item.Name {
				t.Errorf("built-in item = %+v", item)
			}
			if item.Enabled != nil && *item.Enabled {
				on = append(on, item.Name)
			}
		case "config":
			want := []Value{
				{Key: "targets", Type: ValueList, List: []string{"claude"}},
				{Key: "requires", Type: ValueText, Value: ">=0.86.0"},
				{Key: "builtins", Type: ValueList, List: []string{"memory"}},
				{Key: "models.deep", Type: ValueList, List: []string{}},
			}
			if item.Path != "agnostic-ai.yaml" || !reflect.DeepEqual(item.Values, want) {
				t.Errorf("config values = %+v, want %+v", item.Values, want)
			}
		case "settings":
			want := []Value{{Key: "effort", Type: ValueText, Value: "high"}, {Key: "model", Type: ValueText, Value: "example-model"}}
			if item.Name != "tiers" || !reflect.DeepEqual(item.Values, want) {
				t.Errorf("other settings item = %+v, want the model and effort spec with its values", item)
			}
		}
	}
	if !slices.Equal(on, []string{"memory"}) {
		t.Errorf("enabled built-ins = %v, want memory only", on)
	}
}

func TestBuild_TypesEachValueAndRedactsCredentials(t *testing.T) {
	e := entry(spec.KindSettings, "native", ".agnostic-ai/settings/native.yaml", map[string]any{
		"name":        "native",
		"description": "Native keys.",
		"x-claude": map[string]any{
			"attribution": map[string]any{"commit": ""},
			"autoMode":    map[string]any{"allow": []any{"$defaults"}},
			"verbose":     true,
		},
		"env": map[string]any{"API_TOKEN": "plain-looking", "REF": "${API_TOKEN}"},
		"permissions": map[string]any{"allow": []any{
			"shell(ls)",
			"shell(curl -H 'Authorization: Bearer abcdefghijklmnop1234567890')",
			"shell(cp /private/tmp/claude-501/-Users-someone-Products-app/6837a0a0-6f58-448f-98a5-dd67b1da1e44/scratchpad/kw-build/package.json out.json)",
			"shell(export KEY=AbCdEfGhIjKlMnOpQrStUvWxYz0123456789)",
		}},
	})
	got := specValues(e.Meta)
	want := []Value{
		{Key: "env.API_TOKEN", Type: ValueText, Value: "<redacted>"},
		{Key: "env.REF", Type: ValueText, Value: "${API_TOKEN}"},
		{Key: "permissions.allow", Type: ValueList, List: []string{
			"shell(ls)",
			"<redacted>",
			"shell(cp /private/tmp/claude-501/-Users-someone-Products-app/6837a0a0-6f58-448f-98a5-dd67b1da1e44/scratchpad/kw-build/package.json out.json)",
			"<redacted>",
		}},
		{Key: "x-claude.attribution.commit", Type: ValueText, Value: `""`},
		{Key: "x-claude.autoMode.allow", Type: ValueList, List: []string{"$defaults"}},
		{Key: "x-claude.verbose", Type: ValueBool, Value: "true"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("values =\n%+v\nwant\n%+v", got, want)
	}
}

func TestBuild_ListsEveryLifecycleStageWithTheToolsThatRunIt(t *testing.T) {
	v := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude", "windsurf"}}})
	stages := sectionByID(t, v, SectionLifecycle).Stages
	var ids []string
	for _, st := range stages {
		ids = append(ids, st.ID)
		if st.Label == "" || st.Description == "" {
			t.Errorf("stage %s lacks a label or description", st.ID)
		}
	}
	if !slices.Equal(ids, spec.PortableHookEvents) {
		t.Fatalf("stages = %v, want every portable event in session order", ids)
	}
	tools := func(id string) []string {
		var out []string
		for _, st := range stages {
			if st.ID == id {
				for _, tg := range st.Targets {
					out = append(out, tg.Target+"="+tg.Event)
				}
			}
		}
		return out
	}
	if got := tools("session-start"); !slices.Equal(got, []string{"claude=SessionStart"}) {
		t.Errorf("session-start tools = %v, want only Claude Code: Windsurf has no session-start event", got)
	}
	if got := tools("before-tool"); len(got) != 2 {
		t.Errorf("before-tool tools = %v, want both", got)
	}
}

func TestBuild_GivesMarkdownAndYAMLItemsAnAbsolutePathToOpen(t *testing.T) {
	v := Build(fullInputs())
	open := map[string]string{}
	for _, s := range v.Sections {
		for _, it := range s.Items {
			open[it.Kind+":"+it.Name] = it.OpenPath
		}
	}
	for key, ext := range map[string]string{"rule:testing": ".md", "skill:review": ".md", "agent:reviewer": ".md", "settings:permissions": ".yaml", "mcp:github": ".yaml", "config:agnostic-ai.yaml": ".yaml"} {
		if p := open[key]; !filepath.IsAbs(filepath.FromSlash(p)) || !strings.HasSuffix(p, ext) {
			t.Errorf("%s open path = %q, want an absolute %s path", key, p, ext)
		}
	}
	for _, key := range []string{"rule:shared-memory-policy", "builtin:memory"} {
		if p := open[key]; p != "" {
			t.Errorf("%s open path = %q, want none for a built-in", key, p)
		}
	}
}

func TestBuild_ListsEverySettingsFieldWithTheSpecsThatSetIt(t *testing.T) {
	v := Build(fullInputs())
	keys := func(id string) map[string][]string {
		out := map[string][]string{}
		for _, f := range sectionByID(t, v, id).Fields {
			out[f.Key] = []string{}
			for _, src := range f.Set {
				out[f.Key] = append(out[f.Key], src.Spec)
			}
		}
		return out
	}
	authority := keys(SectionAuthority)
	if !slices.Equal(authority["permissions.allow"], []string{"permissions"}) || !slices.Equal(authority["protected"], []string{"protected"}) {
		t.Errorf("authority fields = %v", authority)
	}
	if got, ok := authority["permissions.deny"]; !ok || len(got) != 0 {
		t.Errorf("permissions.deny = %v, want listed and unset", got)
	}
	settings := keys(SectionSettings)
	if !slices.Equal(settings["model"], []string{"tiers"}) || !slices.Equal(settings["effort"], []string{"tiers"}) {
		t.Errorf("settings fields = %v", settings)
	}
	for _, key := range []string{"session-retention-days", "x-"} {
		if got, ok := settings[key]; !ok || len(got) != 0 {
			t.Errorf("%s = %v, want listed and unset", key, got)
		}
	}
	if len(sectionByID(t, v, SectionContext).Fields) != 0 {
		t.Error("context lists settings fields")
	}
}

func TestBuild_ShowsEachMCPServersCachedTools(t *testing.T) {
	mcp := func(name string, meta map[string]any) spec.Entry {
		return entry(spec.KindMCP, name, ".agnostic-ai/mcps/"+name+".yaml", meta)
	}
	fresh := map[string]any{"name": "fresh", "command": "fresh-mcp"}
	edited := map[string]any{"name": "edited", "command": "edited-mcp"}
	in := Inputs{
		Bundle: spec.Bundle{MCPs: []spec.Entry{mcp("fresh", fresh), mcp("edited", edited), mcp("broken", map[string]any{"name": "broken"}), mcp("never", map[string]any{"name": "never"})}},
		MCPTools: mcpprobe.Cache{Servers: map[string]mcpprobe.ServerResult{
			"fresh":  {Spec: mcpprobe.SpecFingerprint(fresh), Tools: []mcpprobe.Tool{{Name: "search", Description: "Find things."}}},
			"edited": {Spec: mcpprobe.SpecFingerprint(map[string]any{"name": "edited", "command": "old-mcp"}), Tools: []mcpprobe.Tool{{Name: "old"}}},
			"broken": {Error: "HTTP 401: the server needs authentication"},
		}},
		RefreshCommand: "agnostic-ai mcp tools --global",
	}
	got := map[string]*ToolListing{}
	for _, it := range sectionByID(t, Build(in), SectionTools).Items {
		got[it.Name] = it.Tools
	}
	if l := got["fresh"]; l == nil || l.State != ToolsListed || len(l.Tools) != 1 || l.Tools[0].Name != "search" || l.Command != "agnostic-ai mcp tools --global" {
		t.Errorf("fresh = %+v", l)
	}
	if l := got["edited"]; l == nil || l.State != ToolsStale || len(l.Tools) != 1 {
		t.Errorf("edited = %+v, want stale with the old listing", l)
	}
	if l := got["broken"]; l == nil || l.State != ToolsFailed || !strings.Contains(l.Error, "authentication") {
		t.Errorf("broken = %+v", l)
	}
	if l := got["never"]; l == nil || l.State != ToolsNotListed || len(l.Tools) != 0 {
		t.Errorf("never = %+v", l)
	}
}

func TestBuild_ShowsProjectOnlyKindsOnlyWhenPresent(t *testing.T) {
	v := Build(Inputs{Cfg: &config.Config{Targets: []string{"claude"}}})
	for id, want := range map[string][]string{
		SectionContext:   {"Instructions", "Rules", "Agents", "Skills"},
		SectionAuthority: {"Permissions"},
		SectionTools:     {"MCP servers"},
		SectionLifecycle: {"Hooks"},
		SectionSettings:  {"Built-ins", "Config", "Other settings"},
	} {
		var labels []string
		for _, k := range sectionByID(t, v, id).Kinds {
			labels = append(labels, k.Label)
		}
		if !slices.Equal(labels, want) {
			t.Errorf("%s kinds = %v, want %v", id, labels, want)
		}
	}
}

func TestBuild_GroupsHooksByLifecyclePoint(t *testing.T) {
	hook := func(name string, meta map[string]any) spec.Entry {
		return entry(spec.KindHook, name, ".agnostic-ai/hooks/"+name+".yaml", meta)
	}
	v := Build(Inputs{Bundle: spec.Bundle{Hooks: []spec.Entry{
		hook("native", map[string]any{"event": "PreToolUse"}),
		hook("summary", map[string]any{"on": "stop"}),
		hook("guard", map[string]any{"on": "before-tool"}),
		hook("greet", map[string]any{"on": "session-start"}),
	}}})
	var got []string
	for _, it := range sectionByID(t, v, SectionLifecycle).Items {
		got = append(got, it.Group+":"+it.Name)
	}
	want := []string{"session-start:greet", "before-tool:guard", "stop:summary", "native:PreToolUse:native"}
	if !slices.Equal(got, want) {
		t.Errorf("hooks = %v, want %v: session order, then native events", got, want)
	}
}

func TestBuild_PlacesFindingsOnTheItemsTheyName(t *testing.T) {
	in := fullInputs()
	in.Findings = []Finding{
		{Source: SourceLint, Code: "LINT038", Severity: "warn", Message: "bare read", Path: ".agnostic-ai/settings/permissions.yaml"},
		{Source: SourceLint, Code: "LINT019", Severity: "warn", Message: "line 3", Path: ".agnostic-ai/skills/review/scripts/run.sh", Line: 3},
		{Source: SourceLint, Code: "LINT011", Severity: "warn", Message: "budget", Path: ".agnostic-ai/AGNOSTIC_AI.md"},
		{Source: SourceDoctor, Code: "codex-hook-trust", Severity: "error", Message: "untrusted", Path: ".codex/hooks.json", Section: SectionLifecycle},
		{Source: SourceLint, Code: "LINT999", Severity: "error", Message: "somewhere else", Path: "elsewhere.txt"},
		{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
	}
	v := Build(in)
	on := func(section, name string) []string {
		var codes []string
		for _, it := range sectionByID(t, v, section).Items {
			if it.Name == name {
				for _, f := range it.Findings {
					codes = append(codes, f.Code)
				}
			}
		}
		return codes
	}
	for _, c := range []struct{ section, name, code string }{
		{SectionAuthority, "permissions", "LINT038"},
		{SectionContext, "review", "LINT019"},
		{SectionContext, "AGNOSTIC_AI.md", "LINT011"},
	} {
		if got := on(c.section, c.name); !slices.Equal(got, []string{c.code}) {
			t.Errorf("%s %s findings = %v, want %s", c.section, c.name, got, c.code)
		}
	}
	sectionCodes := func(id string) []string {
		var codes []string
		for _, f := range sectionByID(t, v, id).Findings {
			codes = append(codes, f.Code)
		}
		return codes
	}
	if got := sectionCodes(SectionLifecycle); !slices.Equal(got, []string{"codex-hook-trust"}) {
		t.Errorf("lifecycle findings = %v, want the doctor finding that names no item", got)
	}
	if got := sectionCodes(SectionSettings); !slices.Equal(got, []string{"LINT999"}) {
		t.Errorf("settings findings = %v, want the finding with no item and no section", got)
	}
	for _, s := range v.Sections {
		for _, f := range s.Findings {
			if f.Source == SourceDrift {
				t.Errorf("drift placed as a finding in %s", s.ID)
			}
		}
	}
}

func TestBuild_ReportsEachTargetsSyncState(t *testing.T) {
	v := Build(Inputs{
		Cfg:      &config.Config{Targets: []string{"claude", "codex", "cursor"}},
		Compared: []string{"claude", "codex"},
		Findings: []Finding{
			{Source: SourceDrift, Target: "claude", Path: "CLAUDE.md", Message: "stale"},
			{Source: SourceDrift, Target: "claude", Path: ".claude/settings.json", Message: "not generated"},
		},
	})
	got := map[string]TargetSync{}
	for _, ts := range v.Targets {
		got[ts.Target] = ts.Sync
	}
	if c := got["claude"]; c.State != SyncDrifted || len(c.Files) != 2 || c.Files[0].Path != ".claude/settings.json" || c.Files[0].State != "not generated" {
		t.Errorf("claude = %+v, want drifted with both files, sorted", c)
	}
	if got["codex"].State != SyncInSync || got["cursor"].State != SyncNotChecked {
		t.Errorf("codex = %+v, cursor = %+v, want in-sync and not-checked", got["codex"], got["cursor"])
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
	if !slices.Equal(kinds, []string{"rule", "agent", "skill", "command", "review", "ignore"}) {
		t.Errorf("context support kinds = %v, want only the spec kinds, not the instructions row", kinds)
	}
	var settingsKinds []string
	for _, ks := range sectionByID(t, v, SectionSettings).Support {
		settingsKinds = append(settingsKinds, ks.Kind)
	}
	if !slices.Equal(settingsKinds, []string{"settings"}) {
		t.Errorf("settings support kinds = %v, want none for built-ins and config", settingsKinds)
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
	labels := newLabeler()
	for _, name := range adapters.Names() {
		if labels.of(name) == name {
			t.Errorf("target %s has no label", name)
		}
	}
}

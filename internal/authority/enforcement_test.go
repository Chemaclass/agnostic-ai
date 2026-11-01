package authority

import (
	"slices"
	"strings"
	"testing"
)

func TestBuild_SameDenyRuleIsDeniedOnClaudeAndUnresolvedOnCodex(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  deny: [mcp:railway/delete]\n",
	})
	claude := findOp(t, buildView(t, root, Options{Target: "claude"}), "mcp-railway-scope:delete")
	codex := findOp(t, buildView(t, root, Options{Target: "codex"}), "mcp-railway-scope:delete")

	if claude.Declared != EffectDenied || codex.Declared != EffectDenied {
		t.Fatalf("declared = %s and %s, want denied on both: the declaration does not depend on the tool", claude.Declared, codex.Declared)
	}
	if claude.Effect != EffectDenied || claude.Resolution != ResolutionTranslated || !slices.Equal(claude.Native, []string{"mcp__railway__delete"}) {
		t.Errorf("claude = %s/%s %v, want denied, translated as mcp__railway__delete", claude.Effect, claude.Resolution, claude.Native)
	}
	if codex.Effect != EffectUnresolved || codex.Resolution != ResolutionNotExpressed {
		t.Errorf("codex = %s/%s, want unresolved: sync writes no rule for Codex", codex.Effect, codex.Resolution)
	}
	if e := enforcementOn(t, codex, "codex"); e.Note != "sync writes no rule for this tool; its own defaults apply" {
		t.Errorf("codex note = %q", e.Note)
	}
	if !strings.HasPrefix(codex.Explanation, "Unresolved on Codex.") {
		t.Errorf("codex explanation = %q", codex.Explanation)
	}
}

func TestBuild_AgentRestrictionOnATargetWithoutAgentToolListsIsUnresolved(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":               "version: 1\ntargets: [codex, claude]\n",
		".agnostic-ai/agents/scanner.md": "---\nname: scanner\ndescription: Scans.\ncan: [read]\n---\n\nScan.\n",
	})
	v := buildView(t, root, Options{AgentID: "scanner", Target: "codex"})

	shell := findOp(t, v, "shell-any:run")
	if shell.Declared != EffectUnavailable {
		t.Fatalf("declared = %s, want unavailable: can leaves shell out", shell.Declared)
	}
	if shell.Effect != EffectUnresolved || shell.Resolution != ResolutionAgentScope {
		t.Errorf("codex = %s/%s, want unresolved: Codex writes no agent tool list", shell.Effect, shell.Resolution)
	}
	if e := enforcementOn(t, shell, "codex"); e.Note != "this tool reads no per-agent tool list" {
		t.Errorf("note = %q", e.Note)
	}
	if e := enforcementOn(t, shell, "claude"); e.Effect != EffectUnavailable {
		t.Errorf("claude = %+v, want unavailable: Claude Code writes the agent's tools", e)
	}
}

func TestBuild_RuleFormsResolveAsTranslatedOnlyWhereTheirMatchingIsDocumented(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, qoder, kilo, opencode]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [shell(go test:*), shell(git * main), read, write]\n",
	})
	prefix := map[string]string{}
	for _, target := range []string{"claude", "qoder", "kilo", "opencode"} {
		v := buildView(t, root, Options{Target: target})
		prefix[target] = findOp(t, v, "shell-go-test:run").Resolution
		if read := findOp(t, v, "files-all:read"); read.Resolution != ResolutionTranslated || read.Effect != EffectAllowed {
			t.Errorf("%s whole-tool read = %s/%s, want translated and allowed", target, read.Resolution, read.Effect)
		}
		if target == "qoder" {
			op := findOp(t, v, "shell-go-test:run")
			if op.Effect != EffectUnresolved || len(op.Native) == 0 {
				t.Errorf("qoder prefix = %s %v, want unresolved with the written rule", op.Effect, op.Native)
			}
			if len(uncertaintiesWith(v, UncertaintyMatchingUnverified)) == 0 {
				t.Error("no target-matching-unverified uncertainty on qoder")
			}
		}
		if target == "claude" {
			glob := findOp(t, v, "shell-git-any-main-exact:run")
			if glob.Resolution != ResolutionTranslatedUnverified || !strings.Contains(enforcementOn(t, glob, "claude").Note, "literally") {
				t.Errorf("claude mid-command glob = %s %q, want unverified with the documented caveat", glob.Resolution, enforcementOn(t, glob, "claude").Note)
			}
		}
		if target == "opencode" {
			write := findOp(t, v, "files-all:write")
			if write.Resolution != ResolutionTranslatedUnverified || !strings.Contains(enforcementOn(t, write, "opencode").Note, "grants more than declared") {
				t.Errorf("opencode write = %s, want unverified: the translation widens", write.Resolution)
			}
		}
	}
	want := map[string]string{
		"claude":   ResolutionTranslated,
		"qoder":    ResolutionTranslatedUnverified,
		"kilo":     ResolutionTranslated,
		"opencode": ResolutionTranslated,
	}
	for target, resolution := range want {
		if prefix[target] != resolution {
			t.Errorf("%s prefix rule = %s, want %s", target, prefix[target], resolution)
		}
	}
}

func TestBuild_SettingsRuleOnAToolWithoutSettingsIsNotExpressed(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [aider]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:railway/deploy]\n",
	})
	op := findOp(t, buildView(t, root, Options{}), "mcp-railway-scope:deploy")
	if op.Resolution != ResolutionNotExpressed || !strings.Contains(enforcementOn(t, op, "aider").Note, "takes no settings specs") {
		t.Errorf("aider = %s %q, want not expressed: Aider takes no settings specs", op.Resolution, enforcementOn(t, op, "aider").Note)
	}
}

func TestBuild_KiroSettingsPermissionsAreNotExpressed(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [kiro]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  deny: [mcp:railway/deploy]\n",
	})
	op := findOp(t, buildView(t, root, Options{}), "mcp-railway-scope:deploy")
	if e := enforcementOn(t, op, "kiro"); op.Resolution != ResolutionNotExpressed || e.Status != "not-expressed" || len(e.Native) != 0 {
		t.Errorf("kiro = %s %+v, want not expressed with no native rule", op.Resolution, e)
	}
}

func TestBuild_EffectIsHypotheticalUntilTheCarryingFileIsInSync(t *testing.T) {
	files := []InstalledFile{{Path: ".claude/settings.json"}, {Path: ".mcp.json"}}
	for _, tc := range []struct {
		state        string
		hypothetical bool
		prefix       string
	}{
		{InstalledNotGenerated, true, "Would be permitted once .claude/settings.json is synced; sync has not written it yet."},
		{InstalledStale, true, "Would be permitted once .claude/settings.json is synced; the file on disk differs"},
		{InstalledInSync, false, "Permitted. Decided by allow mcp:railway/deploy"},
	} {
		installed := InstalledState{State: tc.state, Files: slices.Clone(files)}
		for i := range installed.Files {
			installed.Files[i].State = tc.state
		}
		v := buildView(t, demoRoot, Options{Target: "claude", Installed: map[string]InstalledState{"claude": installed}})
		deploy := findOp(t, v, "mcp-railway-scope:deploy")
		if deploy.Installed != tc.state || deploy.Hypothetical != tc.hypothetical {
			t.Errorf("%s: installed %s hypothetical %v", tc.state, deploy.Installed, deploy.Hypothetical)
		}
		if !strings.HasPrefix(deploy.Explanation, tc.prefix) {
			t.Errorf("%s: explanation %q, want prefix %q", tc.state, deploy.Explanation, tc.prefix)
		}
		if other := findOp(t, v, "mcp-railway-scope:*"); other.Hypothetical || other.Installed != "not-applicable" {
			t.Errorf("%s: a default effect is hypothetical %v, installed %s", tc.state, other.Hypothetical, other.Installed)
		}
		if v.Snapshot.Evidence.Installed != "checked" {
			t.Errorf("evidence = %q", v.Snapshot.Evidence.Installed)
		}
	}

	unchecked := buildView(t, demoRoot, Options{Target: "claude"})
	if op := findOp(t, unchecked, "mcp-railway-scope:deploy"); op.Installed != InstalledNotChecked || !op.Hypothetical {
		t.Errorf("unchecked deploy = %s hypothetical %v", op.Installed, op.Hypothetical)
	}
	if len(uncertaintiesWith(unchecked, UncertaintyInstalledNotChecked)) != 1 {
		t.Error("no installed-not-checked uncertainty")
	}
}

func TestClassifyRule_NamesEachRuleForm(t *testing.T) {
	for rule, want := range map[string]ruleForm{
		"read":                    formWholeTool,
		"web":                     formWholeTool,
		"mcp:github/get_issue":    formMCPTool,
		"mcp:github":              formMCPServer,
		"shell(git status)":       formShellExact,
		"shell(go test:*)":        formShellColon,
		"shell(go test *)":        formShellStar,
		"shell(git * main)":       formShellGlob,
		"edit(src/**)":            formPath,
		"WebFetch(domain:go.dev)": formWebDomain,
		"not a rule(":             formOther,
	} {
		if got := classifyRule(rule); got != want {
			t.Errorf("classifyRule(%q) = %s, want %s", rule, got, want)
		}
	}
}

func TestBuild_OverlappingRulesResolveOnlyWhereThePrecedenceIsDocumented(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, opencode]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  deny: [shell]\n  allow:\n    - shell(git push:*)\n",
	})
	pushOp := func(v *View) Operation {
		t.Helper()
		for _, res := range v.Resources {
			if res.Kind == ResourceCommandPattern && strings.HasPrefix(res.Label, "git push") {
				return res.Operations[0]
			}
		}
		t.Fatalf("no git push resource in %+v", v.Resources)
		return Operation{}
	}
	claude := pushOp(buildView(t, root, Options{Target: "claude"}))
	if claude.Declared != EffectDenied || claude.Effect != EffectDenied || claude.Resolution != ResolutionTranslated {
		t.Errorf("claude: declared %s, effect %s, resolution %s; want the documented deny-over-allow order", claude.Declared, claude.Effect, claude.Resolution)
	}
	opencode := pushOp(buildView(t, root, Options{Target: "opencode"}))
	if opencode.Declared != EffectDenied || opencode.Effect != EffectUnresolved || opencode.Resolution != ResolutionTranslatedUnverified {
		t.Errorf("opencode: declared %s, effect %s, resolution %s; want unresolved, since OpenCode applies the narrower rule written last", opencode.Declared, opencode.Effect, opencode.Resolution)
	}
	if note := enforcementOn(t, opencode, "opencode").Note; !strings.Contains(note, "overlapping rules") {
		t.Errorf("opencode note = %q, want the overlap named", note)
	}
	if len(uncertaintiesWith(buildView(t, root, Options{Target: "opencode"}), UncertaintyMatchingUnverified)) == 0 {
		t.Error("no target-matching-unverified uncertainty for the overlap on OpenCode")
	}
}

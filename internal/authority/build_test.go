package authority

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const railwayMCP = `name: railway
description: Railway projects, services, and deployments.
type: http
url: https://mcp.railway.app/mcp
headers:
  Authorization: Bearer ${RAILWAY_TOKEN}
`

func TestBuild_DenyBeatsAllowAndAskWhateverTheOrder(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                   "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/railway.yaml":     railwayMCP,
		".agnostic-ai/settings/allow.yaml":   "permissions:\n  allow: [mcp:railway/deploy, shell]\n",
		".agnostic-ai/settings/policy.yaml":  "permissions:\n  ask: [mcp:railway/deploy]\n  deny: [mcp:railway]\n",
		".agnostic-ai/settings/command.yaml": "permissions:\n  ask: [shell(git push:*)]\n  allow: [shell(git push:*)]\n",
	})
	v := buildView(t, root, Options{})

	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	if deploy.Declared != EffectDenied || deploy.Effect != EffectDenied {
		t.Fatalf("deploy = declared %s, effective %s; want denied for both", deploy.Declared, deploy.Effect)
	}
	if deploy.Decided == nil || deploy.Decided.List != "deny" || deploy.Decided.Rule != "mcp:railway" {
		t.Fatalf("deploy decided by %+v, want the server-wide deny", deploy.Decided)
	}
	if len(deploy.Grants) != 3 || !deploy.Grants[0].Wins || deploy.Grants[0].List != "deny" {
		t.Fatalf("grants = %+v, want deny first and winning, then the others", deploy.Grants)
	}
	for _, g := range deploy.Grants[1:] {
		if g.Wins {
			t.Errorf("grant %+v wins too", g)
		}
	}

	push := findOp(t, v, "shell-git-push:run")
	if push.Declared != EffectAsk {
		t.Errorf("git push = %s, want ask: ask beats a broader and an equal allow", push.Declared)
	}
}

func TestBuild_AgentWithoutTheServerInCanCannotReachIt(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:railway/deploy]\n",
		".agnostic-ai/agents/docs-writer.md":     "---\nname: docs-writer\ndescription: Writes docs.\ncan: [read, edit, mcp:github]\n---\n\nWrite docs.\n",
	})
	v := buildView(t, root, Options{AgentID: "docs-writer"})

	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	if deploy.Declared != EffectUnavailable || deploy.Effect != EffectUnavailable || deploy.Resolution != ResolutionAgentScope {
		t.Fatalf("deploy = %s/%s/%s, want unavailable through agent scope", deploy.Declared, deploy.Effect, deploy.Resolution)
	}
	if deploy.Decided == nil || deploy.Decided.List != "can" || deploy.Decided.Scope != "agent" {
		t.Fatalf("decided = %+v, want the agent's can list", deploy.Decided)
	}
	if !strings.Contains(deploy.Decided.Note, "does not include mcp:railway/deploy") {
		t.Errorf("note %q does not name the missing route", deploy.Decided.Note)
	}
	if len(deploy.Grants) != 2 || deploy.Grants[1].List != "allow" || deploy.Grants[1].Wins {
		t.Errorf("grants = %+v, want the losing project allow after the agent's list", deploy.Grants)
	}
	if v.Agent.ToolScope.Mode != "listed" || v.Agent.ToolScope.Field != "can" {
		t.Errorf("tool scope = %+v", v.Agent.ToolScope)
	}
}

func TestBuild_ReadonlyAgentCannotEditOrWrite(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [edit, write]\n",
		".agnostic-ai/agents/auditor.md":         "---\nname: auditor\ndescription: Audits.\nreadonly: true\n---\n\nAudit.\n",
	})
	v := buildView(t, root, Options{AgentID: "auditor"})

	for _, id := range []string{"files-all:edit", "files-all:write"} {
		op := findOp(t, v, id)
		if op.Declared != EffectUnavailable || op.Effect != EffectUnavailable {
			t.Errorf("%s = %s/%s, want unavailable on Claude Code", id, op.Declared, op.Effect)
		}
		if op.Decided == nil || op.Decided.List != "readonly" {
			t.Errorf("%s decided by %+v, want readonly", id, op.Decided)
		}
		if e := enforcementOn(t, op, "codex"); e.Effect != EffectUnresolved || !strings.Contains(e.Note, "Codex ignores sandbox_mode") {
			t.Errorf("%s on codex = %+v, want unresolved: Codex ignores readonly", id, e)
		}
	}
	if del := findOp(t, v, "files-all:delete"); del.Effect != EffectUnresolved {
		t.Errorf("delete = %s, want unresolved: Claude Code has no delete tool", del.Effect)
	}
	if read := findOp(t, v, "files-all:read"); read.Declared != EffectDefault {
		t.Errorf("read = %s, want default: readonly leaves reading alone", read.Declared)
	}
}

func TestBuild_SessionSubjectSeesEveryRuleWithoutNarrowing(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	if v.Agent.ID != SessionSubjectID || v.Agent.Kind != SubjectSession || v.Agent.ToolScope.Mode != "unrestricted" || v.Agent.MCPServers.Mode != "all" {
		t.Fatalf("subject = %+v, want the unrestricted session", v.Agent)
	}
	if v.Agent.Label != "Tool session (no agent)" {
		t.Errorf("label = %q", v.Agent.Label)
	}
	ids := make([]string, 0, len(v.Agents))
	for _, s := range v.Agents {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []string{"session", "coding-agent", "data-analyst", "reviewer"}) {
		t.Errorf("agents = %v", ids)
	}
	for id, want := range map[string]Effect{
		"mcp-analytics-db-scope:query":   EffectAllowed,
		"mcp-analytics-db-scope:execute": EffectDenied,
		"mcp-railway-scope:variables":    EffectAsk,
		"shell-rm:run":                   EffectDenied,
		"shell-any:run":                  EffectDefault,
	} {
		if op := findOp(t, v, id); op.Declared != want {
			t.Errorf("%s = %s, want %s", id, op.Declared, want)
		}
	}
	for _, r := range v.Resources {
		for _, op := range r.Operations {
			for _, g := range op.Grants {
				if g.Scope == "agent" {
					t.Fatalf("%s carries an agent grant %+v for the session", op.ID, g)
				}
			}
		}
	}
}

func TestBuild_ProtectedPathAsksBeforeEditOnEachTargetItsOwnWay(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                     "version: 1\ntargets: [claude, codex, cursor, gemini, copilot]\n",
		".agnostic-ai/settings/protected.yaml": "protected:\n  paths: [.github/**]\n  decision: ask\n  reason: CI changes need review.\n",
	})
	v := buildView(t, root, Options{})

	edit := findOp(t, v, "files-github-all:edit")
	if edit.Declared != EffectAsk || edit.Effect != EffectAsk || edit.Resolution != ResolutionTranslated {
		t.Fatalf("edit = %s/%s/%s, want ask translated on Claude Code", edit.Declared, edit.Effect, edit.Resolution)
	}
	if edit.Decided == nil || edit.Decided.List != "protected" || !strings.Contains(edit.Decided.Note, "decision: ask") {
		t.Fatalf("decided = %+v", edit.Decided)
	}
	claude := enforcementOn(t, edit, "claude")
	if claude.Status != "native-rule" || !slices.Equal(claude.Native, []string{"Edit(/.github/**)"}) {
		t.Errorf("claude = %+v", claude)
	}
	for _, target := range []string{"codex", "gemini"} {
		e := enforcementOn(t, edit, target)
		if e.Status != "hook" || e.Effect != EffectDenied || !slices.Equal(e.Native, []string{"." + target + "/hooks/agnostic-ai-protect.sh"}) {
			t.Errorf("%s = %+v, want a hook that blocks the edit", target, e)
		}
	}
	if e := enforcementOn(t, edit, "cursor"); e.Status != "not-expressed" || e.Effect != EffectUnresolved || !strings.Contains(e.Note, "Cursor does not enforce ask") {
		t.Errorf("cursor = %+v", e)
	}
	if e := enforcementOn(t, edit, "copilot"); e.Status != "not-expressed" || e.Effect != EffectUnresolved {
		t.Errorf("copilot = %+v", e)
	}
	if write := findOp(t, v, "files-github-all:write"); write.Declared != EffectAsk {
		t.Errorf("write = %s, want ask: protection covers writes too", write.Declared)
	}
	if all := findOp(t, v, "files-all:edit"); all.Declared != EffectDefault || len(all.Conditions) == 0 || !strings.Contains(all.Conditions[0], "protected .github/** (ask)") {
		t.Errorf("all files edit = %s, conditions %v; want default with the narrower protection named", all.Declared, all.Conditions)
	}
}

func TestBuild_DenyProtectionIsACursorWriteRule(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                     "version: 1\ntargets: [cursor, claude]\n",
		".agnostic-ai/settings/protected.yaml": "protected:\n  paths: [composer.lock]\n  decision: deny\n",
	})
	v := buildView(t, root, Options{})

	edit := findOp(t, v, "files-composer-lock:edit")
	if edit.Effect != EffectDenied || edit.Resolution != ResolutionTranslated {
		t.Fatalf("edit on cursor = %s/%s, want denied translated", edit.Effect, edit.Resolution)
	}
	want := []string{"Write(composer.lock) in .cursor/cli.json", "Write(composer.lock/**) in .cursor/cli.json"}
	if !slices.Equal(edit.Native, want) {
		t.Errorf("native = %v, want %v", edit.Native, want)
	}
}

func TestBuild_FileRootBecomesADeclaredRootWithItsCaveat(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	root := findResource(t, v, "mcp-workspace-files-root-1")
	if root.Kind != ResourceDeclaredRoot || root.Basis != "declared-root" || !root.LabelResolved || root.Label != "docs folder" {
		t.Fatalf("root = %+v", root)
	}
	if root.Identity.URI != "file:///workspace/demo-project/docs" || root.Identity.Server != "workspace-files" {
		t.Errorf("identity = %+v", root.Identity)
	}
	if root.Caveat != "informational filesystem guidance; not access control" {
		t.Errorf("caveat = %q", root.Caveat)
	}
	op := root.Operations[len(root.Operations)-1]
	if op.Action != anyToolAction || !slices.Contains(op.Conditions, "The rule names the server, not this root; the same decision applies to every root.") {
		t.Errorf("operation = %+v", op)
	}
	if !hasResourceKind(v, ResourceDeclaredRoot) {
		t.Error("no declared-root resource")
	}
}

func TestBuild_UnnamedFileRootFallsBackToItsURI(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":            "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/docs.yaml": "name: docs\ntype: stdio\ncommand: docs-server\nroots:\n  - uri: file:///srv/docs\n",
	})
	v := buildView(t, root, Options{})

	r := findResource(t, v, "mcp-docs-root-1")
	if r.Label != "file:///srv/docs" || r.LabelResolved {
		t.Errorf("label = %q resolved %v, want the URI, unresolved", r.Label, r.LabelResolved)
	}
}

func TestBuild_ServerWithoutRootsKeepsAnUndeclaredScope(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	scope := findResource(t, v, "mcp-railway-scope")
	if scope.Kind != ResourceUndeclaredScope || scope.Label != "Scope not declared" || scope.Basis != "undeclared" {
		t.Fatalf("scope = %+v", scope)
	}
	if scope.Description != "The server declares no roots. Credential reach is not inspected." {
		t.Errorf("description = %q", scope.Description)
	}
	for _, code := range []string{UncertaintyScopeUndeclared, UncertaintyCredentialReach} {
		found := false
		for _, u := range uncertaintiesWith(v, code) {
			found = found || slices.Contains(u.RelatedIDs, "mcp-railway-scope")
		}
		if !found {
			t.Errorf("no %s uncertainty for railway", code)
		}
	}
}

func TestBuild_ServerWideRuleOnANonFileRootNamesNoResourceForTheRoot(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP + "roots:\n  - uri: railway://acme/environments/staging/services/api\n    name: api (staging)\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:railway]\n",
	})
	v := buildView(t, root, Options{})

	var railway []Resource
	for _, r := range v.Resources {
		if r.ProviderID == "mcp-railway" {
			railway = append(railway, r)
		}
	}
	if len(railway) != 1 || railway[0].Kind != ResourceUndeclaredScope {
		t.Fatalf("railway resources = %+v, want one undeclared scope and nothing for the root", railway)
	}
	notFile := uncertaintiesWith(v, UncertaintyRootNotFilesystem)
	if len(notFile) != 1 || !strings.Contains(notFile[0].Message, "railway://acme/environments/staging/services/api") || !strings.Contains(notFile[0].Message, "railway") {
		t.Fatalf("root-not-filesystem = %+v, want one naming the root and its server", notFile)
	}
	anyTool := findOp(t, v, "mcp-railway-scope:*")
	if anyTool.Declared != EffectAllowed || anyTool.Effect != EffectAllowed || anyTool.Resolution != ResolutionTranslated {
		t.Errorf("any tool = %s/%s/%s, want the server-wide allow, documented on Claude Code", anyTool.Declared, anyTool.Effect, anyTool.Resolution)
	}
}

func TestBuild_ToolRulesBecomeOperationsBesideAnyOtherTool(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	github := findResource(t, v, "mcp-github-scope")
	var labels []string
	for _, op := range github.Operations {
		labels = append(labels, op.Label)
	}
	want := []string{"Get pull request", "List pull request files", "Create pull request", "Merge pull request", "Any other tool"}
	if !slices.Equal(labels, want) {
		t.Fatalf("operations = %v, want %v", labels, want)
	}
	create := findOp(t, v, "mcp-github-scope:create_pull_request")
	if create.Native == nil || create.Native[0] != "mcp__github__create_pull_request" {
		t.Errorf("native = %v", create.Native)
	}
	if other := findOp(t, v, "mcp-github-scope:*"); other.Declared != EffectDefault {
		t.Errorf("any other tool = %s, want default: no server-wide rule", other.Declared)
	}
	found := false
	for _, u := range uncertaintiesWith(v, UncertaintyToolListUnknown) {
		found = found || slices.Contains(u.RelatedIDs, "mcp-github-scope:*")
	}
	if !found {
		t.Error("no tool-list-unknown uncertainty for github")
	}
}

func TestBuild_DisabledServerIsUnavailableWhereTheToolHonorsIt(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude, cursor, gemini]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP + "disabled: true\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:railway/deploy]\n",
	})
	v := buildView(t, root, Options{})

	var status string
	for _, p := range v.Providers {
		if p.ID == "mcp-railway" {
			status = p.Status
		}
	}
	if status != "disabled" {
		t.Fatalf("provider status = %q", status)
	}
	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	if deploy.Declared != EffectUnavailable || deploy.Effect != EffectUnavailable || deploy.Resolution != ResolutionDisabled {
		t.Fatalf("deploy = %s/%s/%s", deploy.Declared, deploy.Effect, deploy.Resolution)
	}
	if e := enforcementOn(t, deploy, "cursor"); e.Effect != EffectUnavailable {
		t.Errorf("cursor = %+v, want unavailable: sync leaves the server out", e)
	}
	if e := enforcementOn(t, deploy, "gemini"); e.Effect != EffectUnresolved {
		t.Errorf("gemini = %+v, want unresolved: the disabled table does not list it", e)
	}
}

func TestBuild_UnknownAgentOrTargetIsNotFound(t *testing.T) {
	cfg, b := loadProject(t, demoRoot)
	for _, opts := range []Options{{AgentID: "nobody"}, {Target: "nowhere"}} {
		_, err := Build(cfg, b, opts)
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Build(%+v) error = %v, want NotFoundError", opts, err)
		}
		if len(notFound.Valid) == 0 || !strings.Contains(err.Error(), notFound.Valid[0]) {
			t.Errorf("error %q does not list the valid ids %v", err, notFound.Valid)
		}
	}
}

func TestBuild_FileMatchesWalkTheProjectWithinBounds(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [edit(src/**)]\n",
		"src/a.go":                               "package a\n",
		"src/b/c.go":                             "package b\n",
		"node_modules/x/src/d.js":                "x\n",
	})
	v := buildView(t, root, Options{FileMatches: true})

	m := findResource(t, v, "files-src-all").Matches
	if m == nil || m.Count != 2 || !slices.Equal(m.Samples, []string{"src/a.go", "src/b/c.go"}) || m.Truncated {
		t.Fatalf("matches = %+v", m)
	}
	if all := findResource(t, v, "files-all").Matches; all == nil || slices.Contains(all.Samples, "node_modules/x/src/d.js") {
		t.Errorf("all files = %+v, want node_modules skipped", all)
	}
	if without := buildView(t, root, Options{}); findResource(t, without, "files-src-all").Matches != nil {
		t.Error("matches resolved without FileMatches")
	}
}

func TestBuild_GlobalViewWritesNoPermissionRules(t *testing.T) {
	root := writeProject(t, map[string]string{
		"settings/permissions.yaml": "permissions:\n  allow: [shell(go test:*)]\n",
	})
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: "settings"}}
	b, err := spec.LoadBundle(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Build(cfg, b, Options{Root: root, Global: true, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if v.Snapshot.Project.Name != "global ("+root+")" || v.Harness.PermissionSupport != "none" {
		t.Errorf("project %q, support %q", v.Snapshot.Project.Name, v.Harness.PermissionSupport)
	}
	op := findOp(t, v, "shell-go-test:run")
	if op.Declared != EffectAllowed || op.Effect != EffectUnresolved || !strings.Contains(enforcementOn(t, op, "claude").Note, "sync --global writes no permission rules") {
		t.Errorf("go test = %+v", op)
	}
	if v.Snapshot.Evidence.Installed != InstalledNotChecked {
		t.Errorf("installed evidence = %q", v.Snapshot.Evidence.Installed)
	}
}

func TestBuild_GrantsPointAtTheLineThatDeclaresThem(t *testing.T) {
	v := buildView(t, demoRoot, Options{AgentID: "data-analyst", Target: "claude"})

	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	lines := map[string]int{}
	for _, g := range deploy.Grants {
		lines[g.List] = g.Source.Line
	}
	if want := map[string]int{"mcpServers": 5, "can": 4, "allow": 11}; !maps.Equal(lines, want) {
		t.Errorf("grant lines = %v, want %v", lines, want)
	}
	if op := findOp(t, v, "files-migrations-all:edit"); len(op.Grants) == 0 || op.Grants[len(op.Grants)-1].Source.Line != 4 {
		t.Errorf("migrations grants = %+v, want the protected path on line 4", op.Grants)
	}
}

func TestBuild_EveryOperationHasOneEdgeAndNoListIsNull(t *testing.T) {
	for _, agent := range []string{"", "coding-agent", "reviewer", "data-analyst"} {
		v := buildView(t, demoRoot, Options{AgentID: agent, Target: "codex", FileMatches: true})
		ops := 0
		for _, r := range v.Resources {
			ops += len(r.Operations)
		}
		if len(v.Edges) != ops {
			t.Errorf("%s: %d edges for %d operations", agent, len(v.Edges), ops)
		}
		for _, e := range v.Edges {
			if e.ID != e.SubjectID+"|"+e.OperationID || e.SubjectID != v.Agent.ID {
				t.Errorf("edge %+v", e)
			}
		}
		data := mustJSON(t, v)
		if strings.Contains(data, ":null") {
			t.Errorf("%s: the view holds a null list, which a client would have to guard against", agent)
		}
	}
}

const ruleToken = "ghp_0123456789abcdefghijklmnop"

func TestBuild_RedactsACredentialWrittenInAPermissionRule(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [\"shell(echo " + ruleToken + ")\"]\n  ask: [\"shell(curl -H 'X-Api-Key: " + ruleToken + "':*)\"]\n",
	})
	v := buildView(t, root, Options{})

	if data := mustJSON(t, v); strings.Contains(data, ruleToken) {
		t.Fatalf("the view carries the token written in a rule: %s", data)
	}
	redacted := 0
	for _, r := range v.Resources {
		if r.Kind != ResourceCommandPattern || r.Identity.Pattern == "*" {
			continue
		}
		redacted++
		if !strings.HasPrefix(r.ID, "shell-redacted") || !strings.HasPrefix(r.Label, "<redacted>") || r.Identity.Pattern != "<redacted>" {
			t.Errorf("resource = %+v, want a redacted id, label, and pattern", r)
		}
		if !strings.Contains(r.Description, "holds a credential") {
			t.Errorf("description = %q", r.Description)
		}
		for _, op := range r.Operations {
			if op.Decided == nil || op.Decided.Rule != "shell(<redacted>)" || !op.Decided.Redacted {
				t.Errorf("decided = %+v, want the redacted rule", op.Decided)
			}
			for _, e := range op.Enforcement {
				if e.Status == "native-rule" && !slices.Equal(e.Native, []string{"<redacted>"}) {
					t.Errorf("native on %s = %v", e.Target, e.Native)
				}
			}
		}
	}
	if redacted != 2 {
		t.Errorf("found %d command resources, want 2", redacted)
	}
}

func TestBuild_AgentToolListsAreRedactedToo(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                "version: 1\ntargets: [claude]\n",
		".agnostic-ai/agents/deployer.md": "---\nname: deployer\ndescription: Deploys.\ncan: [read, \"shell(echo " + ruleToken + ")\"]\n---\n\nDeploy.\n",
	})
	v := buildView(t, root, Options{AgentID: "deployer"})

	if data := mustJSON(t, v); strings.Contains(data, ruleToken) {
		t.Fatalf("the view carries the token written in a tool list: %s", data)
	}
	if !slices.Contains(v.Agent.ToolScope.Declared, "shell(<redacted>)") || !slices.Contains(v.Agent.ToolScope.Capabilities, "shell(<redacted>)") {
		t.Errorf("tool scope = %+v", v.Agent.ToolScope)
	}
}

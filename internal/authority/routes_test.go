package authority

import (
	"slices"
	"strings"
	"testing"
)

func routeEffect(op Operation, route string) (Effect, bool) {
	for _, r := range op.RouteCoverage.Checked {
		if r.Route == route {
			return r.Effect, true
		}
	}
	return "", false
}

func TestBuild_ShellRouteIsCheckedWithTheDefaultWhenNoBareShellRuleExists(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	effect, ok := routeEffect(deploy, "shell: any command")
	if !ok || effect != EffectDefault {
		t.Fatalf("shell route = %q (checked %v), want default: no rule names shell as a whole", effect, ok)
	}
	if effect, ok := routeEffect(deploy, "shell: go test …"); !ok || effect != EffectAllowed {
		t.Errorf("go test route = %q (checked %v), want allowed", effect, ok)
	}
	if effect, ok := routeEffect(deploy, "local files"); !ok || effect != EffectAllowed {
		t.Errorf("local files route = %q (checked %v), want allowed reads", effect, ok)
	}
	if !slices.ContainsFunc(deploy.AlternateRoutes, func(s string) bool { return strings.HasPrefix(s, "Shell access is not decided by any rule") }) {
		t.Errorf("alternate routes = %v, want the open shell route named", deploy.AlternateRoutes)
	}
	if len(uncertaintiesWith(v, UncertaintyAlternateRoute)) != 1 {
		t.Error("no alternate-route-not-modeled uncertainty")
	}
}

func TestBuild_OperationWithNoOpenRouteStillListsWhatWasCheckedAndNotModeled(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/railway.yaml":         railwayMCP,
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [mcp:railway/deploy]\n",
		".agnostic-ai/agents/deployer.md":        "---\nname: deployer\ndescription: Deploys.\ncan: [mcp:railway]\n---\n\nDeploy.\n",
	})
	v := buildView(t, root, Options{AgentID: "deployer"})

	deploy := findOp(t, v, "mcp-railway-scope:deploy")
	if len(deploy.AlternateRoutes) != 0 {
		t.Fatalf("alternate routes = %v, want none open: the agent has neither shell nor files", deploy.AlternateRoutes)
	}
	if len(deploy.RouteCoverage.Checked) == 0 || len(deploy.RouteCoverage.NotModeled) == 0 {
		t.Fatalf("route coverage = %+v, want checked and not-modeled routes even with none open", deploy.RouteCoverage)
	}
	if effect, _ := routeEffect(deploy, "shell: any command"); effect != EffectUnavailable {
		t.Errorf("shell route = %q, want unavailable", effect)
	}
	if !slices.Equal(deploy.RouteCoverage.NotModeled, notModeledRoutes) {
		t.Errorf("not modeled = %v", deploy.RouteCoverage.NotModeled)
	}
	if len(uncertaintiesWith(v, UncertaintyAlternateRoute)) != 1 {
		t.Error("the view does not say which routes it leaves unmodeled when none of the checked ones is open")
	}
}

func TestBuild_ShellOperationChecksLocalFileRoutes(t *testing.T) {
	v := buildView(t, demoRoot, Options{Target: "claude"})

	rm := findOp(t, v, "shell-rm:run")
	read, okRead := routeEffect(rm, "local files: read")
	edit, okEdit := routeEffect(rm, "local files: edit")
	if !okRead || !okEdit || read != EffectAllowed || edit != EffectDefault {
		t.Fatalf("file routes = read %q (%v), edit %q (%v)", read, okRead, edit, okEdit)
	}
	if _, ok := routeEffect(rm, "shell: any command"); ok {
		t.Error("a shell operation lists the shell route as its own alternate")
	}
}

func TestBuild_RouteWordingAndBadgeCarryTheRouteOwnInstalledState(t *testing.T) {
	for _, target := range []string{"claude", "codex"} {
		for _, agent := range []string{"", "coding-agent", "reviewer"} {
			v := buildView(t, demoRoot, Options{AgentID: agent, Target: target})
			byID := map[string]Operation{}
			for _, res := range v.Resources {
				for _, op := range res.Operations {
					byID[op.ID] = op
				}
			}
			for _, res := range v.Resources {
				for _, op := range res.Operations {
					for _, st := range op.RouteCoverage.Checked {
						src, ok := byID[routeOperationID(st.Route, v)]
						if !ok {
							t.Fatalf("%s/%s %s: route %q names no operation", target, agent, op.ID, st.Route)
						}
						if st.Effect != src.Effect || st.Hypothetical != src.Hypothetical || st.Installed != src.Installed {
							t.Errorf("%s/%s %s: route %q = %+v, want the state of %s (%s, hypothetical %v, installed %s)", target, agent, op.ID, st.Route, st, src.ID, src.Effect, src.Hypothetical, src.Installed)
						}
					}
					for _, sentence := range op.AlternateRoutes {
						st := routeForSentence(t, op, sentence)
						if got := strings.Contains(sentence, "once synced"); got != st.Hypothetical {
							t.Errorf("%s/%s %s: sentence %q says once synced = %v, but route %q is hypothetical = %v", target, agent, op.ID, sentence, got, st.Route, st.Hypothetical)
						}
					}
				}
			}
		}
	}
}

// routeOperationID maps a checked route back to the operation whose
// state it reports.
func routeOperationID(route string, v *View) string {
	switch route {
	case routeShellAny:
		return operationID("shell-any", "run")
	case routeLocalFiles, routeFilesRead:
		return operationID("files-all", capRead)
	case routeFilesEdit:
		return operationID("files-all", capEdit)
	}
	label := strings.TrimPrefix(route, "shell: ")
	for _, res := range v.Resources {
		if res.Kind == ResourceCommandPattern && res.Label == label {
			return operationID(res.ID, "run")
		}
	}
	return ""
}

// routeForSentence finds the checked route an open-route sentence
// describes, by the route's label inside the sentence.
func routeForSentence(t *testing.T, op Operation, sentence string) RouteStatus {
	t.Helper()
	switch {
	case strings.HasPrefix(sentence, "Shell access "):
		return findRoute(t, op, routeShellAny)
	case strings.HasPrefix(sentence, "Reading local files "):
		if r, ok := lookupRoute(op, routeLocalFiles); ok {
			return r
		}
		return findRoute(t, op, routeFilesRead)
	case strings.HasPrefix(sentence, "Editing local files "):
		return findRoute(t, op, routeFilesEdit)
	}
	rest := strings.TrimPrefix(sentence, "Shell commands matching ")
	for _, st := range op.RouteCoverage.Checked {
		if label, ok := strings.CutPrefix(st.Route, "shell: "); ok && strings.HasPrefix(rest, label+" ") {
			return st
		}
	}
	t.Fatalf("%s: no checked route for sentence %q", op.ID, sentence)
	return RouteStatus{}
}

func lookupRoute(op Operation, name string) (RouteStatus, bool) {
	for _, st := range op.RouteCoverage.Checked {
		if st.Route == name {
			return st, true
		}
	}
	return RouteStatus{}, false
}

func findRoute(t *testing.T, op Operation, name string) RouteStatus {
	t.Helper()
	st, ok := lookupRoute(op, name)
	if !ok {
		t.Fatalf("%s: no checked route %q", op.ID, name)
	}
	return st
}

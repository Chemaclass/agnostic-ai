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

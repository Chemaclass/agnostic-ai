package spec

import (
	"strings"
	"testing"
)

func TestNormalizeScope_RejectsScopeInsideNodeModules(t *testing.T) {
	for _, scope := range []string{"node_modules", "node_modules/pkg", "apps/web/node_modules/pkg"} {
		_, err := NormalizeScope(scope)
		if err == nil || !strings.Contains(err.Error(), "node_modules") {
			t.Errorf("NormalizeScope(%q) error = %v, want a node_modules refusal", scope, err)
		}
	}
}

func TestNormalizeScope_KeepsOrdinaryScopes(t *testing.T) {
	for scope, want := range map[string]string{
		"services/api":          "services/api",
		"services/api/":         "services/api",
		"packages/node_mods":    "packages/node_mods",
		"tools/my-node_modules": "tools/my-node_modules",
	} {
		got, err := NormalizeScope(scope)
		if err != nil || got != want {
			t.Errorf("NormalizeScope(%q) = %q, %v; want %q", scope, got, err, want)
		}
	}
}

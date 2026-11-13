package opencode

import "testing"

func TestTranslateAgentCapability_MatchesWriter(t *testing.T) {
	for _, tool := range []string{"Read", "Bash", "Write", "mcp__github"} {
		native, ok := (Adapter{}).TranslateAgentCapability(tool)
		perms, _ := agentPermission([]string{tool}, nil)
		if !ok || len(native) != 1 {
			t.Fatalf("%s: native=%v ok=%v", tool, native, ok)
		}
		if _, written := perms[native[0]]; !written {
			t.Errorf("%s: translator says %q, writer wrote %v", tool, native[0], perms)
		}
	}
}

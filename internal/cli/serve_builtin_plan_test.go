package cli

import (
	"strings"
	"testing"
)

func TestPlanBuiltinChange_RefusesARequestThatChangesNothing(t *testing.T) {
	root := builtinChangeProject(t)
	before, err := snapshotBuiltinProject(root, []string{"agnostic-ai.yaml", stateFilePath(".")})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := planBuiltinChange(root, builtinRequest{name: "caveman", enabled: false}, audienceShared)

	if plan != nil || err == nil || !strings.Contains(err.Error(), "caveman already has the requested value") {
		t.Fatalf("plan = %v, err = %v; want the already-has-value refusal", plan, err)
	}
	if err := before.check(root); err != nil {
		t.Fatalf("refused preview changed the project: %v", err)
	}
}

package cli

import (
	"reflect"
	"testing"
)

func TestWatchTargets_FollowsConfigWithoutTargetFlags(t *testing.T) {
	effective := []string{"claude", "codex"}

	if got := watchTargets(nil, nil, nil, effective); got != nil {
		t.Errorf("no target flags: got %v, want nil so each pass reads the config", got)
	}

	cases := map[string]struct{ targets, only, except []string }{
		"--target": {targets: []string{"claude"}},
		"--only":   {only: []string{"claude"}},
		"--except": {except: []string{"cursor"}},
	}
	for name, c := range cases {
		if got := watchTargets(c.targets, c.only, c.except, effective); !reflect.DeepEqual(got, effective) {
			t.Errorf("%s: got %v, want %v", name, got, effective)
		}
	}
}

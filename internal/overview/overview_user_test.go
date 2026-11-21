package overview

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestBuild_MarksUserConfigurationAsUser(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	if Build(Inputs{Cfg: cfg}).UserLevel {
		t.Error("a project view is marked global")
	}
	if !Build(Inputs{Cfg: cfg, UserLevel: true}).UserLevel {
		t.Error("a user configuration view is not marked global")
	}
}

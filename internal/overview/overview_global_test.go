package overview

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestBuild_MarksUserConfigurationAsGlobal(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}}
	if Build(Inputs{Cfg: cfg}).Global {
		t.Error("a project view is marked global")
	}
	if !Build(Inputs{Cfg: cfg, Global: true}).Global {
		t.Error("a user configuration view is not marked global")
	}
}

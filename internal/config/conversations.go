package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

const stuckShellAfterKey = "conversations.stuckShellAfter"

type ConversationsConfig struct {
	StuckShellAfter *ShellDuration `yaml:"stuckShellAfter,omitempty" json:"stuckShellAfter,omitempty" jsonschema_description:"How long a Claude Code background shell can run before Conversations offers to stop it. Use a duration like 45m or 1h30m. Default 30m. Use 0 to disable. Home config only."`
}

type ShellDuration string

func (d *ShellDuration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode || (n.Tag != "!!str" && n.Tag != "!!int") {
		return fmt.Errorf("%s must be a duration such as 30m, or 0 to disable", stuckShellAfterKey)
	}
	*d = ShellDuration(n.Value)
	return nil
}

func (c ConversationsConfig) ShellThreshold(source string) (*time.Duration, error) {
	if c.StuckShellAfter == nil {
		return nil, nil
	}
	duration, err := time.ParseDuration(string(*c.StuckShellAfter))
	if err != nil || duration < 0 {
		return nil, errs.Coded(errs.CodeConfigDecode, "%s: %s must be a nonnegative duration such as 30m, or 0 to disable", source, stuckShellAfterKey)
	}
	return &duration, nil
}

package cli

import (
	"fmt"
	"strings"
)

// switchValue is a bool flag that also reads on and off, so init and
// sync take the same words for --gitignore.
type switchValue bool

func (v *switchValue) Set(s string) error {
	on, err := parseSwitch(s)
	if err != nil {
		return err
	}
	*v = switchValue(on)
	return nil
}

func (v *switchValue) String() string {
	if *v {
		return "on"
	}
	return "off"
}

func (v *switchValue) Type() string     { return "on|off" }
func (v *switchValue) IsBoolFlag() bool { return true }

func parseSwitch(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true":
		return true, nil
	case "off", "false":
		return false, nil
	}
	return false, fmt.Errorf("expected on or off, got %q", s)
}

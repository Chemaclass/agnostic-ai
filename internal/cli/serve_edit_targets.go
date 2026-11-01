package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

const editTargetsPath = "/api/edit/targets"

var errLastTarget = errors.New("keep at least one coding tool on")

// supportedTargetNames are the tools a switch may name: every built-in
// adapter in a project, the user-level ones with --global.
func (h *editHandler) supportedTargetNames() []string {
	if h.global {
		return globalTargetNames()
	}
	return adapters.Names()
}

// defaultTargets is the list that applies when no config sets targets.
func (h *editHandler) defaultTargets() []string {
	if h.global {
		return globalTargetNames()
	}
	return config.DefaultTargets()
}

// teamTargets is the list the committed config selects, or the default
// list when it sets none; listed says which.
func (h *editHandler) teamTargets() (list []string, listed bool, err error) {
	team, _ := h.builtinsConfigFiles()
	data, err := os.ReadFile(team)
	if os.IsNotExist(err) {
		return h.defaultTargets(), false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", team, err)
	}
	set, err := configList(data, "targets")
	if err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", team, err)
	}
	if set == nil {
		return h.defaultTargets(), false, nil
	}
	return *set, true, nil
}

// setTarget switches one name in the targets list and refuses to leave
// it empty.
func setTarget(data []byte, name string, enabled bool) ([]byte, error) {
	out, err := setListMember(data, "targets", name, enabled)
	if err != nil {
		return nil, err
	}
	list, err := configList(out, "targets")
	if err != nil {
		return nil, err
	}
	if list != nil && len(*list) == 0 {
		return nil, errLastTarget
	}
	return out, nil
}

type targetsRequest struct {
	name    string
	enabled bool
	reset   bool
}

// targetsEdit applies req to the shared or the personal targets list.
// A shared edit on a config with no targets key writes the default list
// first, since a one-name list would drop every other default. A
// personal edit copies the shared list into the personal config first,
// since that list replaces the shared one whole, and drops it again once
// it matches.
func (h *editHandler) targetsEdit(where editAudience, req targetsRequest) (string, func([]byte) ([]byte, error), error) {
	team, local := h.builtinsConfigFiles()
	localData, err := os.ReadFile(local)
	localMissing := os.IsNotExist(err)
	if err != nil && !localMissing {
		return "", nil, fmt.Errorf("%s: %w", local, err)
	}
	teamList, _, err := h.teamTargets()
	if err != nil {
		return "", nil, err
	}
	defaults := h.defaultTargets()
	if where == audienceShared {
		if req.reset {
			return "", nil, fmt.Errorf("%s: reset applies to your personal list; send audience personal", overviewDisplay(h.root, local))
		}
		if personal, err := configList(localData, "targets"); err != nil || personal != nil {
			return "", nil, cmp.Or(err, fmt.Errorf("%s sets targets for you; reset it before changing the shared list", overviewDisplay(h.root, local)))
		}
		return team, func(data []byte) ([]byte, error) {
			listed, err := configList(data, "targets")
			if err != nil {
				return nil, err
			}
			if listed == nil {
				if slices.Contains(defaults, req.name) == req.enabled {
					return data, nil
				}
				_, mapping, err := yamlDocument(data)
				if err != nil {
					return nil, err
				}
				if data, err = editYAMLPair(data, mapping, "targets", slices.Clone(defaults)); err != nil {
					return nil, err
				}
			}
			return setTarget(data, req.name, req.enabled)
		}, nil
	}
	if err := h.checkPersonalTarget(local); err != nil {
		return "", nil, err
	}
	inherited := ""
	if !h.global {
		if inherited, err = inheritedPersonalMemory(h.root); err != nil {
			return "", nil, err
		}
	}
	display := overviewDisplay(h.root, local)
	return local, func(data []byte) ([]byte, error) {
		personal, err := configList(data, "targets")
		if err != nil {
			return nil, err
		}
		if personal != nil && !yamlHasKey(data, "targets") {
			return nil, fmt.Errorf("%s sets targets through a YAML merge; edit that file by hand", display)
		}
		var out []byte
		switch {
		case req.reset && personal == nil:
			return nil, fmt.Errorf("%s: there is no personal targets list to reset", display)
		case req.reset:
			out = data
		case personal == nil && slices.Contains(teamList, req.name) == req.enabled:
			return data, nil
		default:
			if personal == nil {
				seed := ""
				if localMissing {
					seed = inherited
				}
				if data, err = seedPersonalList(data, "targets", teamList, seed); err != nil {
					return nil, err
				}
			}
			if out, err = setTarget(data, req.name, req.enabled); err != nil {
				return nil, err
			}
			if personal, err = configList(out, "targets"); err != nil {
				return nil, err
			}
		}
		if !req.reset && !sameBuiltins(*personal, teamList) {
			return out, nil
		}
		if out, err = removeYAMLPath(out, "targets"); err != nil {
			return nil, err
		}
		return personalConfigRest(out, inherited), nil
	}, nil
}

// checkTargetsExpected refuses a switch whose page showed a stale state.
func (h *editHandler) checkTargetsExpected(name string, reset bool, raw json.RawMessage) error {
	list, _, err := h.teamTargets()
	if err != nil {
		return err
	}
	_, local := h.builtinsConfigFiles()
	data, err := os.ReadFile(local)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", local, err)
	}
	if err == nil {
		personal, err := configList(data, "targets")
		if err != nil {
			return err
		}
		if personal != nil {
			list = *personal
		}
	}
	return checkListExpected(list, "coding tools", name, reset, raw)
}

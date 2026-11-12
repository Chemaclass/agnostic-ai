package hookrun

import (
	"slices"
	"strings"
	"testing"
)

// docs.augmentcode.com/cli/hooks.md, "Environment variables in
// commands": with args, Auggie expands ${VAR} and ${VAR:-default} in
// command and every args item, leaves an unset variable without a
// default as written, and reads \${ as a literal ${.
func TestAugmentExpand_FollowsTheHooksReference(t *testing.T) {
	env := []string{"SET=value", "EMPTY=", "BIN=/usr/bin/env"}
	for _, tc := range []struct {
		name string
		h    Handler
		want Handler
	}{
		{"a set variable", Handler{Command: "node", Args: []string{"${SET}/x"}}, Handler{Command: "node", Args: []string{"value/x"}}},
		{"a set variable with a default", Handler{Command: "node", Args: []string{"${SET:-other}"}}, Handler{Command: "node", Args: []string{"value"}}},
		{"an unset variable with a default", Handler{Command: "node", Args: []string{"${UNSET:-block}"}}, Handler{Command: "node", Args: []string{"block"}}},
		{"an unset variable without a default", Handler{Command: "node", Args: []string{"${UNSET}"}}, Handler{Command: "node", Args: []string{"${UNSET}"}}},
		{"an escaped reference", Handler{Command: "node", Args: []string{`\${SET}`}}, Handler{Command: "node", Args: []string{"${SET}"}}},
		{"a reference without braces", Handler{Command: "node", Args: []string{"$SET"}}, Handler{Command: "node", Args: []string{"$SET"}}},
		{"the command", Handler{Command: "${BIN}", Args: []string{"true"}}, Handler{Command: "/usr/bin/env", Args: []string{"true"}}},
		{"a set empty variable", Handler{Command: "node", Args: []string{"a${EMPTY}b"}}, Handler{Command: "node", Args: []string{"ab"}}},
		{"a command without args goes to a shell", Handler{Command: "echo ${SET}"}, Handler{Command: "echo ${SET}"}},
	} {
		got, unmodeled := AugmentExpand(tc.h, env)
		if unmodeled != "" || got.Command != tc.want.Command || !slices.Equal(got.Args, tc.want.Args) {
			t.Errorf("%s: %q %q, %q; want %q %q", tc.name, got.Command, got.Args, unmodeled, tc.want.Command, tc.want.Args)
		}
	}
}

func TestAugmentExpand_FlagsFormsTheReferenceLeavesOpen(t *testing.T) {
	env := []string{"SET=value", "EMPTY="}
	for _, arg := range []string{
		"${EMPTY:-block}",
		"${SET-block}",
		"${SET:+block}",
		"${SET:=block}",
		"${UNSET:-${SET}}",
		"${#SET}",
		"${unclosed",
		"${AUGMENT_PROJECT_DIR}/hooks/check.sh",
	} {
		if _, unmodeled := AugmentExpand(Handler{Command: "node", Args: []string{arg}}, env); !strings.Contains(unmodeled, "Augment's hooks reference") {
			t.Errorf("%s: unmodeled = %q", arg, unmodeled)
		}
	}
}

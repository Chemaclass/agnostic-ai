package approvals_test

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

const preset = "approve-for-me"

func TestKiroPolicy_AddThenRemoveRestoresBytes(t *testing.T) {
	cases := map[string]struct{ before, added string }{
		"no file": {
			before: "",
			added:  "policies:\n  - approve-for-me\n",
		},
		"comments only": {
			before: "# my rules\n",
			added:  "# my rules\npolicies:\n  - approve-for-me\n",
		},
		"rules without policies": {
			before: "# keep me\nrules:\n  - capability: shell # inline\n    match: [\"git *\"]\n    effect: allow\n",
			added:  "# keep me\nrules:\n  - capability: shell # inline\n    match: [\"git *\"]\n    effect: allow\npolicies:\n  - approve-for-me\n",
		},
		"no final newline": {
			before: "rules: []",
			added:  "rules: []\npolicies:\n  - approve-for-me\n",
		},
		"block list": {
			before: "policies:\n  # mine\n  - dev-shell\nrules: []\n",
			added:  "policies:\n  - approve-for-me\n  # mine\n  - dev-shell\nrules: []\n",
		},
		"block list at key indent": {
			before: "policies:\n- dev-shell\n",
			added:  "policies:\n- approve-for-me\n- dev-shell\n",
		},
		"flow list": {
			before: "policies: [dev-shell, read-all] # two\n",
			added:  "policies: [approve-for-me, dev-shell, read-all] # two\n",
		},
		"empty flow list": {
			before: "policies: []\n",
			added:  "policies: [approve-for-me]\n",
		},
		"empty key": {
			before: "policies: # none yet\nrules: []\n",
			added:  "policies: # none yet\n  - approve-for-me\nrules: []\n",
		},
		"empty key at end without newline": {
			before: "policies:",
			added:  "policies:\n  - approve-for-me\n",
		},
		"windows line ends": {
			before: "rules: []\r\n",
			added:  "rules: []\r\npolicies:\r\n  - approve-for-me\r\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			add, err := approvals.AddKiroPolicy("p.yaml", []byte(tc.before), preset)
			if err != nil {
				t.Fatal(err)
			}
			if string(add.Data) != tc.added || !add.Changed {
				t.Fatalf("added = %q (changed %v), want %q", add.Data, add.Changed, tc.added)
			}
			listed, err := approvals.KiroListsPolicy("p.yaml", add.Data, preset)
			if err != nil || !listed {
				t.Fatalf("listed = %v, %v after add", listed, err)
			}
			again, err := approvals.AddKiroPolicy("p.yaml", add.Data, preset)
			if err != nil || again.Changed || string(again.Data) != tc.added {
				t.Fatalf("second add = %q (changed %v), %v; want no change", again.Data, again.Changed, err)
			}
			removed, err := approvals.RemoveKiroPolicy("p.yaml", add.Data, preset, add.Owned)
			if err != nil {
				t.Fatal(err)
			}
			if string(removed) != tc.before {
				t.Fatalf("removed = %q, want %q", removed, tc.before)
			}
		})
	}
}

func TestKiroPolicy_RemoveKeepsWhatTheUserAddedSince(t *testing.T) {
	add, err := approvals.AddKiroPolicy("p.yaml", []byte("rules: []\n"), preset)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(add.Data), "  - approve-for-me\n", "  - approve-for-me\n  - dev-shell\n", 1) + "# later\n"
	removed, err := approvals.RemoveKiroPolicy("p.yaml", []byte(edited), preset, add.Owned)
	if err != nil {
		t.Fatal(err)
	}
	if want := "rules: []\npolicies:\n  - dev-shell\n# later\n"; string(removed) != want {
		t.Fatalf("removed = %q, want %q", removed, want)
	}
}

func TestKiroPolicy_RemoveWithoutTheItemChangesNothing(t *testing.T) {
	data := []byte("policies:\n  - dev-shell\n")
	removed, err := approvals.RemoveKiroPolicy("p.yaml", data, preset, approvals.KiroPolicyOwned{CreatedKey: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(removed) != string(data) {
		t.Fatalf("removed = %q, want it unchanged", removed)
	}
}

func TestKiroPolicy_AlreadyListedIsNoChange(t *testing.T) {
	data := []byte("policies:\n  - approve-for-me\n")
	add, err := approvals.AddKiroPolicy("p.yaml", data, preset)
	if err != nil || add.Changed || string(add.Data) != string(data) {
		t.Fatalf("add = %q (changed %v), %v; want no change", add.Data, add.Changed, err)
	}
}

func TestKiroPolicy_RefusesShapesItCannotEditInPlace(t *testing.T) {
	for name, data := range map[string]string{
		"not yaml":            "rules: [\n",
		"list at top":         "- approve-for-me\n",
		"flow mapping at top": "{rules: []}\n",
		"policies is text":    "policies: approve-for-me\n",
		"policies is a map":   "policies:\n  a: b\n",
		"two documents":       "rules: []\n---\nrules: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := approvals.AddKiroPolicy("p.yaml", []byte(data), preset); err == nil || !strings.Contains(err.Error(), "p.yaml") {
				t.Fatalf("err = %v, want one naming p.yaml", err)
			}
		})
	}
}

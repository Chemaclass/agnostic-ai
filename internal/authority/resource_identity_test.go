package authority

import (
	"regexp"
	"strings"
	"testing"
)

func TestImpact_CredentialCommandIdentitySurvivesRemovalAndAddition(t *testing.T) {
	const first = "shell(echo ghp_0123456789abcdefghijklmnop)"
	const second = "shell(echo ghp_9876543210abcdefghijklmnop)"
	for _, removing := range []bool{true, false} {
		name := "addition"
		allow := ""
		change := Change{Op: "add", List: "allow", Rule: first}
		wantBefore, wantAfter := EffectDefault, EffectAllowed
		if removing {
			name = "removal"
			allow = "  allow: ['" + first + "']\n"
			change.Op = "remove"
			wantBefore, wantAfter = EffectAllowed, EffectDefault
		}
		t.Run(name, func(t *testing.T) {
			root := writeProject(t, map[string]string{
				"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
				".agnostic-ai/settings/permissions.yaml": "permissions:\n" + allow + "  deny: ['" + second + "']\n",
			})
			before := buildView(t, root, Options{})
			var secondID string
			for _, r := range before.Resources {
				if r.Kind == ResourceCommandPattern && r.Operations[0].Declared == EffectDenied {
					secondID = r.ID
				}
			}
			if secondID == "" {
				t.Fatal("denied command missing before proposal")
			}
			res := impactOf(t, root, Options{}, Proposal{Changes: []Change{change}})
			if !res.Valid {
				t.Fatalf("proposal invalid: %v", res.Problems)
			}
			if len(res.Changes) != 1 {
				t.Errorf("changes = %+v, want only the first command", res.Changes)
			}
			for _, c := range res.Changes {
				if c.ResourceID == secondID || c.DeclaredBefore != wantBefore || c.DeclaredAfter != wantAfter {
					t.Errorf("incorrect command change: %+v", c)
				}
			}
			var found bool
			for _, r := range res.After.Resources {
				if r.ID == secondID {
					found = true
					if r.Operations[0].Declared != EffectDenied {
						t.Errorf("unchanged second command became %s", r.Operations[0].Declared)
					}
				}
			}
			if !found {
				t.Error("unchanged second command lost its identity")
			}
			data := mustJSON(t, res)
			for _, secret := range []string{"ghp_0123456789abcdefghijklmnop", "ghp_9876543210abcdefghijklmnop"} {
				if strings.Contains(data, secret) || strings.Contains(data, strings.ReplaceAll(secret, "_", "-")) {
					t.Error("proposal exposed a credential")
				}
			}
		})
	}
}

func TestShellResources_CredentialIDsIgnoreDiscoveryOrderAndIncludeMatchingSemantics(t *testing.T) {
	const exact = "shell(echo ghp_0123456789abcdefghijklmnop)"
	const prefix = "shell(echo ghp_0123456789abcdefghijklmnop:*)"
	const other = "shell(echo ghp_9876543210abcdefghijklmnop)"
	idsFor := func(rules []string) map[string]string {
		t.Helper()
		d := discovery{}
		for _, rule := range rules {
			d.caps = append(d.caps, parseCapability(rule)...)
		}
		out := map[string]string{}
		for _, r := range shellResources(d, map[string]bool{}, nil) {
			if r.shell == nil {
				continue
			}
			key := r.shell.prefix
			if r.shell.exact {
				key += " exact"
			} else {
				key += " prefix"
			}
			out[key] = r.res.ID
			if !regexp.MustCompile(`^shell-redacted_[0-9a-f]{64}$`).MatchString(r.res.ID) {
				t.Errorf("credential ID is not opaque: %s", r.res.ID)
			}
		}
		return out
	}
	a := idsFor([]string{exact, prefix, other})
	b := idsFor([]string{other, prefix, exact})
	seen := map[string]bool{}
	for key, id := range a {
		if b[key] != id {
			t.Errorf("ID changed on reorder: %s -> %s", id, b[key])
		}
		if seen[id] {
			t.Errorf("exact and prefix commands share ID: %s", id)
		}
		seen[id] = true
	}
	if len(a) != 3 {
		t.Errorf("resources = %d, want distinct exact, prefix, and other commands", len(a))
	}
}

func TestShellResources_CredentialIdentityCannotCollideWithASafeSlug(t *testing.T) {
	const command = "echo ghp_0123456789abcdefghijklmnop"
	fingerprint := opaqueID("true\x00" + command)
	d := discovery{caps: parseCapability("shell(" + command + ")")}
	d.caps = append(d.caps, parseCapability("shell(redacted-"+fingerprint+":*)")...)
	ids := map[string]bool{}
	for _, r := range shellResources(d, map[string]bool{}, nil) {
		if ids[r.res.ID] {
			t.Errorf("credential identity collides with safe command slug: %s", r.res.ID)
		}
		ids[r.res.ID] = true
	}
}

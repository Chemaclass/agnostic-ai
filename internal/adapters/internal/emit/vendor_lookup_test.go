package emit

import (
	"sort"
	"testing"
)

// vendorLookupOrders holds, per target, the project instruction files
// that target's CLI checks in order, first match wins with no merge.
// Only vendors that document such an order belong here.
//
// zed: "Zed uses the first matching file in this list"
// (https://zed.dev/docs/ai/instructions, target-audit 2026-08-27, #624).
var vendorLookupOrders = map[string][]string{
	"zed": {
		".rules",
		".cursorrules",
		".windsurfrules",
		".clinerules",
		".github/copilot-instructions.md",
		"AGENT.md",
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
	},
}

// TestVendorLookupOrder_EntryPointIsRead guards the assumption behind
// every first-match lookup: the file we write for that target has to be
// on the vendor's list at all. A path the vendor never opens delivers
// nothing, however complete its content.
func TestVendorLookupOrder_EntryPointIsRead(t *testing.T) {
	t.Parallel()
	for _, vendor := range sortedVendors() {
		path := EntryPointPath(nil, vendor)
		if rankIn(vendorLookupOrders[vendor], path) < 0 {
			t.Errorf("%s entry-point %q is absent from its documented lookup order %v",
				vendor, path, vendorLookupOrders[vendor])
		}
	}
}

// TestVendorLookupOrder_WinningFileCarriesRules is the regression guard
// for #624. entryPointPaths and inlineRulesTargets are independent maps,
// so a target that is in the first but not the second writes a
// pointer-only file, and that file can outrank the file carrying the
// rule bodies in a vendor's lookup order. The vendor then reads the
// pointer and stops, and every rule silently stops applying.
//
// Each vendor is paired with every other target because the hazard needs
// only two enabled targets to bite: whichever pair a user enables, the
// file the vendor opens first must carry the rules. Passing pairwise
// implies passing for any larger target set, since a bigger set only
// adds consumers that could deliver the rules.
func TestVendorLookupOrder_WinningFileCarriesRules(t *testing.T) {
	t.Parallel()
	for _, vendor := range sortedVendors() {
		order := vendorLookupOrders[vendor]
		for _, other := range sortedTargets() {
			if other == vendor {
				continue
			}
			winner, consumers := firstMatch(order, vendor, other)
			if winner == "" {
				t.Errorf("%s + %s: no emitted file matches the %s lookup order", vendor, other, vendor)
				continue
			}
			if !anyInlinesRules(consumers) {
				t.Errorf("%s + %s: %s reads %q first (rank %d) and it carries no rule bodies; "+
					"consumers %v all write a pointer-only body",
					vendor, other, vendor, winner, rankIn(order, winner)+1, consumers)
			}
		}
	}
}

// firstMatch returns the entry-point path the vendor opens first when
// only these targets are enabled, plus every target writing to it.
func firstMatch(order []string, targets ...string) (string, []string) {
	consumers := map[string][]string{}
	for _, t := range targets {
		if p := EntryPointPath(nil, t); p != "" {
			consumers[p] = append(consumers[p], t)
		}
	}
	for _, p := range order {
		if c, ok := consumers[p]; ok {
			return p, c
		}
	}
	return "", nil
}

func sortedVendors() []string {
	out := make([]string, 0, len(vendorLookupOrders))
	for v := range vendorLookupOrders {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func sortedTargets() []string {
	out := make([]string, 0, len(entryPointPaths))
	for t := range entryPointPaths {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func anyInlinesRules(targets []string) bool {
	for _, t := range targets {
		if InlinesRulesIntoEntryPoint(t) {
			return true
		}
	}
	return false
}

func rankIn(order []string, path string) int {
	for i, p := range order {
		if p == path {
			return i
		}
	}
	return -1
}

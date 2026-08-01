package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// sourcesPath is the upstream doc/changelog list the target-audit skill
// reads instead of searching. Its accuracy is what keeps an audit run
// cheap, so the invariants below are enforced rather than trusted.
const sourcesPath = "../../.agnostic-ai/skills/target-audit/references/sources.md"

// sectionRE matches a per-target heading: "## <target>".
var sectionRE = regexp.MustCompile(`(?m)^## ([a-z0-9-]+)$`)

func readSources(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(sourcesPath))
	if err != nil {
		t.Fatalf("read %s: %v", sourcesPath, err)
	}
	return string(data)
}

func sourceSections(t *testing.T) map[string]string {
	t.Helper()
	body := readSources(t)
	idx := sectionRE.FindAllStringSubmatchIndex(body, -1)
	out := make(map[string]string, len(idx))
	for i, m := range idx {
		name := body[m[2]:m[3]]
		end := len(body)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		out[name] = body[m[1]:end]
	}
	return out
}

// TestAuditSources_CoverEveryRegisteredTarget is the invariant that keeps
// the target-audit skill honest as adapters are added. A new target with
// no upstream sources entry would sync, emit, and pass every other test
// while silently never being audited: the exact failure mode the skill
// exists to catch, reproduced inside the skill's own tooling. Adding an
// adapter must add its docs here in the same PR.
func TestAuditSources_CoverEveryRegisteredTarget(t *testing.T) {
	sections := sourceSections(t)
	for _, name := range adapters.Names() {
		if _, ok := sections[name]; !ok {
			t.Errorf("target %q has no `## %s` section in %s; add its vendor docs and changelog before merging the adapter",
				name, name, sourcesPath)
		}
	}
}

// TestAuditSources_NameOnlyRegisteredTargets catches the reverse drift: a
// section left behind after a target is renamed or dropped sends an
// auditor to fetch docs for a target that no longer exists.
func TestAuditSources_NameOnlyRegisteredTargets(t *testing.T) {
	registered := make(map[string]bool, len(adapters.Names()))
	for _, name := range adapters.Names() {
		registered[name] = true
	}
	for name := range sourceSections(t) {
		if !registered[name] {
			t.Errorf("%s documents %q, which is not a registered target", sourcesPath, name)
		}
	}
}

// TestAuditSources_EverySectionCarriesDocsAndWatch asserts each entry is
// usable: a `docs:` line to fetch and a `watch:` line naming what churns.
// A section with neither is a placeholder that costs an auditor a full
// search to work around.
func TestAuditSources_EverySectionCarriesDocsAndWatch(t *testing.T) {
	for name, body := range sourceSections(t) {
		if !strings.Contains(body, "- docs: http") {
			t.Errorf("section %q has no `- docs: http...` line", name)
		}
		if !strings.Contains(body, "- watch:") {
			t.Errorf("section %q has no `- watch:` line naming what churns", name)
		}
	}
}

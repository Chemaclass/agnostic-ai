package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const keysSkill = ".agnostic-ai/skills/review/SKILL.md"

func syncedKeysProject(t *testing.T) []string {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	mustWriteFile(t, keysSkill, "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	runSyncOK(t)
	return readStateFile(".").Outputs
}

func assertConfigRejected(t *testing.T, outputs []string, want ...string) {
	t.Helper()
	before := snapshotFiles(t, outputs...)
	for _, args := range [][]string{{"sync"}, {"validate"}, {"doctor", "config"}} {
		out, err := runCLI(t, args...)
		if err == nil {
			t.Errorf("%s passed:\n%s", args[0], out)
			continue
		}
		for _, w := range want {
			if !strings.Contains(err.Error()+out, w) {
				t.Errorf("%s does not say %q: %v\n%s", strings.Join(args, " "), w, err, out)
			}
		}
	}
	if after := snapshotFiles(t, outputs...); !reflect.DeepEqual(after, before) {
		t.Error("a rejected config changed the outputs")
	}
}

// A misspelled top-level key used to be dropped, so the target list fell
// back to the defaults and sync removed the other targets' files (#1589).
func TestConfig_UnknownTopLevelKeyFailsBeforeAnyWrite(t *testing.T) {
	outputs := syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargts: [claude, cursor]\n")

	assertConfigRejected(t, outputs, `agnostic-ai.yaml:2: unknown key "targts" (did you mean targets?)`)
}

func TestConfig_UnknownNestedKeyNamesItsPath(t *testing.T) {
	outputs := syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\nsync:\n  collsion-policy: prefer-spec\n")

	assertConfigRejected(t, outputs, `agnostic-ai.yaml:4: unknown key "sync.collsion-policy" (did you mean collision-policy?)`)
}

func TestConfig_UnknownKeyInTheLocalOverrideNamesThatFile(t *testing.T) {
	outputs := syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.local.yaml", "outputs:\n  cursor:\n    rules-extt: .mdc\n")

	assertConfigRejected(t, outputs, `agnostic-ai.local.yaml:3: unknown key "outputs.cursor.rules-extt"`)
}

func TestConfig_MisspelledTargetFailsBeforeAnyWrite(t *testing.T) {
	outputs := syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursr]\n")

	assertConfigRejected(t, outputs, `unknown target "cursr" (did you mean cursor?)`)
}

func TestConfig_UnknownOnUnsupportedValueFails(t *testing.T) {
	outputs := syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\non-unsupported: wrn\n")

	assertConfigRejected(t, outputs, `on-unsupported: "wrn"`, "warn, error, silent")
}

// A model tier maps target names inline, so its keys are open.
func TestConfig_ModelTierTargetKeysAreNotUnknownKeys(t *testing.T) {
	syncedKeysProject(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\nmodels:\n  fast:\n    claude: haiku\n    default: small\n    effort: low\n")

	runSyncOK(t)
}

// A newer config can carry a key an older binary does not know; its
// requires line explains that better than the unknown key does.
func TestConfig_RequiresMessageComesBeforeAnUnknownKey(t *testing.T) {
	syncedKeysProject(t)
	setRunningVersion(t, "v0.70.0")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\nrequires: \">=0.99.0\"\ntargets: [claude, cursor]\nfuture-key: on\n")

	_, err := runCLI(t, "sync")

	if err == nil || !strings.Contains(err.Error(), "0.99.0") || strings.Contains(err.Error(), "future-key") {
		t.Errorf("sync error = %v, want the requires message only", err)
	}
}

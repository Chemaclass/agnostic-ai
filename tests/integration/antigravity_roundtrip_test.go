package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestAntigravityRoundTrip_SyncImportSyncIsByteEqual is the
// antigravity audit's byte-stability gate from #343:
//
//	sync antigravity -> snapshot .agents/* (rules, agents, mcp, AGENTS.md)
//	                 -> wipe source specs
//	                 -> import antigravity
//	                 -> wipe emit
//	                 -> sync antigravity
//	                 -> assert byte-for-byte identical
//
// Fixture: 3 rules + 3 agents + 2 MCP servers (one stdio, one remote,
// covering the `serverUrl` <-> `url` rename import must reverse so a
// re-emit lands on the same bytes). Skills stay covered at the
// adapter-test level (internal/adapters/antigravity) only: `import
// antigravity` does not read them back yet, so adding them to this
// fixture would only fail the path-set comparison below. MCP servers
// carried the same carve-out until #589 added importAntigravityMCP.
func TestAntigravityRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedAntigravityRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "antigravity")
	first := snapshotAntigravityEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no antigravity output")
	}

	for _, sub := range []string{"agents", "rules", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "antigravity")

	// Wipe both the current default tree and the legacy singular one so
	// the second sync writes from scratch and the comparison reflects
	// emit output, not stale bytes left by the first sync.
	for _, sub := range []string{".agent", ".agents"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "antigravity")
	second := snapshotAntigravityEmit(t, dir)

	firstPaths := sortedKeys(first)
	secondPaths := sortedKeys(second)
	if !equalStringSlice(firstPaths, secondPaths) {
		t.Fatalf("emit path set changed across round-trip\nfirst:  %v\nsecond: %v",
			firstPaths, secondPaths)
	}
	for _, p := range firstPaths {
		if first[p] != second[p] {
			t.Errorf("byte mismatch at %s (first=%d bytes, second=%d bytes)\n%s",
				p, len(first[p]), len(second[p]), unifiedDiffLines(first[p], second[p]))
		}
	}
}

// TestAntigravityRoundTrip_ManualTriggerWithDescriptionStaysManual is
// the regression test for #1117's review: a hand-authored
// `trigger: manual` rule that also carries a `description` (the vendor
// recommends one on every rule) must not silently upgrade to
// `model_decision` after an `import` + `sync` cycle. `manual` means
// "load only on an @-mention"; `model_decision` means "load
// automatically when relevant" -- a real broadening of activation.
func TestAntigravityRoundTrip_ManualTriggerWithDescriptionStaysManual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents", "rules", "checklist.md"),
		[]byte("---\ntrigger: manual\ndescription: Release checklist, read on demand.\n---\n\nConfirm the changelog and version bump.\n"), 0o644))

	runCmd(t, "import", "antigravity")
	must(t, os.RemoveAll(filepath.Join(dir, ".agents")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("a manual rule with a description must round-trip as manual, not get promoted to model_decision:\n%s", got)
	}
	if strings.Contains(got, "trigger: model_decision") {
		t.Errorf("must not silently broaden manual to model_decision:\n%s", got)
	}
	if !strings.Contains(got, "description: Release checklist, read on demand.") {
		t.Errorf("expected the description to still carry through:\n%s", got)
	}
}

// TestAntigravityRoundTrip_UnknownTriggerDoesNotBroadenToAlwaysOn is
// the second regression test for #1117's review: an unrecognized
// `trigger` value must not round-trip into `always_on`, Antigravity's
// most active mode, just because agnostic-ai could not map it onto a
// generic field.
func TestAntigravityRoundTrip_UnknownTriggerDoesNotBroadenToAlwaysOn(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents", "rules", "typo.md"),
		[]byte("---\ntrigger: alwaysOn\n---\n\nUse tabs.\n"), 0o644))

	runCmd(t, "import", "antigravity")
	must(t, os.RemoveAll(filepath.Join(dir, ".agents")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "typo.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, "trigger: always_on") {
		t.Errorf("an unrecognized trigger must not round-trip into always_on:\n%s", got)
	}
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("an unrecognized trigger must fall to the narrowest mode, manual:\n%s", got)
	}
}

// TestAntigravityScopedRoundTrip_ManualStaysManual is the scoped
// counterpart of TestAntigravityRoundTrip_ManualTriggerWithDescriptionStaysManual
// (second review of #1118): a scoped rule's `PrepareScopedRules` pass
// always forces `alwaysApply: false` and a non-empty `globs` from the
// scope pattern, so the generic mapping alone would always re-derive
// `glob`, never `manual`, on the next sync. Only the preserved
// `x-antigravity.trigger` override (`import`'s `NativeKeys`) stops that
// broadening, and only when scope prep does not also strip the trigger
// key back out.
func TestAntigravityScopedRoundTrip_ManualStaysManual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", ".agents", "rules", "manual-only.md"),
		[]byte("---\ntrigger: manual\n---\n\nOnly load this on an @-mention.\n"), 0o644))

	runCmd(t, "import", "antigravity")
	must(t, os.RemoveAll(filepath.Join(dir, "backend")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, "backend", ".agents", "rules", "manual-only.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("a scoped manual rule must round-trip as manual, not get promoted to glob by the scope's own forced globs:\n%s", got)
	}
	if strings.Contains(got, "trigger: glob") {
		t.Errorf("must not silently broaden manual to glob:\n%s", got)
	}
}

// TestAntigravityScopedRoundTrip_ManualWithDescriptionStaysManual is the
// scoped variant with a description attached, so a naive re-derivation
// from the generic fields alone would land on `model_decision` instead
// of `glob`, an equally real broadening of activation (second review of
// #1118).
func TestAntigravityScopedRoundTrip_ManualWithDescriptionStaysManual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", ".agents", "rules", "checklist.md"),
		[]byte("---\ntrigger: manual\ndescription: Release checklist, read on demand.\n---\n\nConfirm the changelog and version bump.\n"), 0o644))

	runCmd(t, "import", "antigravity")
	must(t, os.RemoveAll(filepath.Join(dir, "backend")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, "backend", ".agents", "rules", "checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("a scoped manual rule with a description must round-trip as manual, not model_decision or glob:\n%s", got)
	}
	if !strings.Contains(got, "description: Release checklist, read on demand.") {
		t.Errorf("expected the description to still carry through:\n%s", got)
	}
}

// TestAntigravityScopedRoundTrip_UnknownTriggerStaysManual is the scoped
// variant of TestAntigravityRoundTrip_UnknownTriggerDoesNotBroadenToAlwaysOn:
// an unrecognized trigger on a scoped rule must fall to `manual`, not
// silently broaden to `glob` from the scope's own forced globs (second
// review of #1118).
func TestAntigravityScopedRoundTrip_UnknownTriggerStaysManual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", ".agents", "rules", "typo.md"),
		[]byte("---\ntrigger: alwaysOn\n---\n\nUse tabs.\n"), 0o644))

	runCmd(t, "import", "antigravity")
	must(t, os.RemoveAll(filepath.Join(dir, "backend")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, "backend", ".agents", "rules", "typo.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, "trigger: glob") || strings.Contains(got, "trigger: always_on") {
		t.Errorf("an unrecognized trigger on a scoped rule must fall to manual, not glob or always_on:\n%s", got)
	}
	if !strings.Contains(got, "trigger: manual\n") {
		t.Errorf("an unrecognized trigger must fall to the narrowest mode, manual:\n%s", got)
	}
}

// TestAntigravityRoundTrip_LegacyBackupSurvivesASecondFullSync is the
// third-review regression for #1118: writing `.agent/AGENTS.md.bak`
// through the same tracked path a generated output uses recorded it in
// the sync ledger. The next full sync no longer touches
// `.agent/AGENTS.md` (it is already gone), so the backup never
// re-appears in that sync's written set, and the orphan sweep deleted
// it -- its own copied provenance marker "proved" ownership. Two
// consecutive full syncs must leave the backup byte-identical.
func TestAntigravityRoundTrip_LegacyBackupSurvivesASecondFullSync(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agent"), 0o755))
	const legacy = "# AI Project Conventions\n\nGenerated by agnostic-ai. Do not edit by hand.\n\nstale.\n"
	must(t, os.WriteFile(filepath.Join(dir, ".agent", "AGENTS.md"), []byte(legacy), 0o644))

	runCmd(t, "sync", "-t", "antigravity")

	bakPath := filepath.Join(dir, ".agent", "AGENTS.md.bak")
	first, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("missing backup after first sync: %v", err)
	}
	if string(first) != legacy {
		t.Fatalf("backup does not match the original legacy content:\n%s", first)
	}

	runCmd(t, "sync", "-t", "antigravity")

	second, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup did not survive a second full sync (orphan-swept?): %v", err)
	}
	if string(second) != string(first) {
		t.Errorf("backup changed across a second sync\nfirst:  %q\nsecond: %q", first, second)
	}
}

// TestAntigravityRoundTrip_LegacyContentSurvivesAnExistingBackup is the
// fourth-review regression: a legacy path already recorded in an
// older, already-persisted sync ledger (simulated here directly in
// `.agnostic-ai/.sync-state`, the way a project upgraded straight from
// a pre-#1114 release would carry it) reads as an orphan the moment
// this run's migration no longer declares it a managed output. Until
// this fix, an already-taken `.bak` name made MigrateLegacyPath leave
// the legacy file in place "to be safe", which was the opposite: the
// orphan sweep then deleted it on the strength of its own copied
// provenance marker, and its current bytes -- different from the
// stale `.bak` -- were gone for good. One sync must now end with both
// byte sets on disk.
func TestAntigravityRoundTrip_LegacyContentSurvivesAnExistingBackup(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", ".sync-state"),
		[]byte(`{"version":3,"synced_at":"2026-01-01T00:00:00Z","outputs":[".agent/AGENTS.md"]}`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agent"), 0o755))
	const older = "Generated by agnostic-ai. Do not edit by hand.\n\nolder backup content.\n"
	must(t, os.WriteFile(filepath.Join(dir, ".agent", "AGENTS.md.bak"), []byte(older), 0o644))
	const current = "Generated by agnostic-ai. Do not edit by hand.\n\ncurrent legacy content, never backed up.\n"
	must(t, os.WriteFile(filepath.Join(dir, ".agent", "AGENTS.md"), []byte(current), 0o644))

	runCmd(t, "sync", "-t", "antigravity")

	if _, err := os.Stat(filepath.Join(dir, ".agent", "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("legacy file should be removed once its bytes are backed up, err=%v", err)
	}
	gotOlder, err := os.ReadFile(filepath.Join(dir, ".agent", "AGENTS.md.bak"))
	if err != nil {
		t.Fatalf("existing backup must survive: %v", err)
	}
	if string(gotOlder) != older {
		t.Errorf("existing backup must not be overwritten\nwant: %q\ngot:  %q", older, gotOlder)
	}
	gotCurrent, err := os.ReadFile(filepath.Join(dir, ".agent", "AGENTS.md.bak.1"))
	if err != nil {
		t.Fatalf("current legacy content must land in a numbered backup: %v", err)
	}
	if string(gotCurrent) != current {
		t.Errorf("numbered backup should carry the legacy file's current bytes\nwant: %q\ngot:  %q", current, gotCurrent)
	}
}

// TestAntigravityRoundTrip_NestedRuleFileStaysDormant is the
// import -> sync regression for the third review's second finding:
// Antigravity "scans only immediate `.md` children inside
// `.agents/rules/` ... ignores files nested in subdirectories", so a
// file one level deeper is dormant there. Importing it anyway, and
// preserving its subdirectory as a portable scope, reconstructed a
// spec that the next sync placed at a fresh `<dir>/.agents/rules/`
// root -- a real directory Antigravity does scan -- silently promoting
// a dormant file to active. Covers both the root rules dir and a
// scoped one, since both import paths share the same walker.
func TestAntigravityRoundTrip_NestedRuleFileStaysDormant(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents", "rules", "root.md"), []byte("# root\n\nroot body\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents", "rules", "archive"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents", "rules", "archive", "old.md"), []byte("# old\n\ndormant root-level body\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", ".agents", "rules", "auth.md"), []byte("# auth\n\nauth body\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", ".agents", "rules", "archive"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", ".agents", "rules", "archive", "old.md"), []byte("# old\n\ndormant scoped body\n"), 0o644))

	runCmd(t, "import", "antigravity")

	for _, p := range []string{
		filepath.Join(".agnostic-ai", "rules", "root.md"),
		filepath.Join(".agnostic-ai", "rules", "backend", "auth.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}
	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai", "rules"), "dormant root-level body")
	assertNoFileContains(t, filepath.Join(dir, ".agnostic-ai", "rules"), "dormant scoped body")

	must(t, os.RemoveAll(filepath.Join(dir, ".agents")))
	must(t, os.RemoveAll(filepath.Join(dir, "backend")))
	runCmd(t, "sync", "-t", "antigravity")

	for _, p := range []string{
		filepath.Join(".agents", "rules", "archive", "old.md"),
		filepath.Join("archive", ".agents", "rules", "old.md"),
		filepath.Join("backend", "archive", ".agents", "rules", "old.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("a dormant nested rule must not be activated by sync: %s exists (err=%v)", p, err)
		}
	}
}

// TestAntigravityRoundTrip_ScopeNamedLikeOwnOutputRoot is the
// fourth-review import->sync regression: a rule scoped to a directory
// literally named `.agents` emits to `.agents/.agents/rules/<name>.md`
// (CheckScopePath accepts that name the same as `.github` or
// `vendor`), but import pruned the root `.agents` directory as
// Antigravity's own output root before ever checking whether it also
// held a scoped rules dir, so that one scope name could emit but never
// import back.
func TestAntigravityRoundTrip_ScopeNamedLikeOwnOutputRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agents", ".agents", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agents", ".agents", "rules", "scoped.md"),
		[]byte("# scoped\n\nscoped-in-dot-agents body\n"), 0o644))

	runCmd(t, "import", "antigravity")

	importedSpec := filepath.Join(dir, ".agnostic-ai", "rules", ".agents", "scoped.md")
	if _, err := os.Stat(importedSpec); err != nil {
		t.Fatalf("missing imported spec %s: %v", importedSpec, err)
	}

	must(t, os.RemoveAll(filepath.Join(dir, ".agents")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, ".agents", ".agents", "rules", "scoped.md"))
	if err != nil {
		t.Fatalf("scope .agents did not re-emit at .agents/.agents/rules/scoped.md: %v", err)
	}
	if !strings.Contains(string(raw), "scoped-in-dot-agents body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", raw)
	}
}

// TestAntigravityRoundTrip_LegacyScopeNamedLikeOwnOutputRoot is the
// same fourth-review regression for the legacy singular default:
// `.agent/.agent/rules/<name>.md` must round-trip once
// `outputs.antigravity.rules-dir` points both emission and import at
// `.agent/rules`.
func TestAntigravityRoundTrip_LegacyScopeNamedLikeOwnOutputRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - antigravity
outputs:
  antigravity:
    rules-dir: .agent/rules
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agent", ".agent", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agent", ".agent", "rules", "scoped.md"),
		[]byte("# scoped\n\nscoped-in-dot-agent body\n"), 0o644))

	runCmd(t, "import", "antigravity")

	importedSpec := filepath.Join(dir, ".agnostic-ai", "rules", ".agent", "scoped.md")
	if _, err := os.Stat(importedSpec); err != nil {
		t.Fatalf("missing imported spec %s: %v", importedSpec, err)
	}

	must(t, os.RemoveAll(filepath.Join(dir, ".agent")))
	runCmd(t, "sync", "-t", "antigravity")

	raw, err := os.ReadFile(filepath.Join(dir, ".agent", ".agent", "rules", "scoped.md"))
	if err != nil {
		t.Fatalf("scope .agent did not re-emit at .agent/.agent/rules/scoped.md: %v", err)
	}
	if !strings.Contains(string(raw), "scoped-in-dot-agent body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", raw)
	}
}

// assertNoFileContains fails the test if any file under root contains
// substr.
func assertNoFileContains(t *testing.T, root, substr string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), substr) {
			t.Errorf("%s unexpectedly contains %q (a dormant nested file must not import)", path, substr)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func seedAntigravityRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  rules: .agnostic-ai/rules
  mcps: .agnostic-ai/mcps
targets:
  - antigravity
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/fs.yaml"),
		[]byte("name: fs\ncommand: npx\nargs:\n  - \"-y\"\n  - \"@modelcontextprotocol/server-filesystem\"\ncwd: /workspace\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/github.yaml"),
		[]byte("name: github\ntype: http\nurl: https://api.githubcopilot.com/mcp/\nheaders:\n  Authorization: Bearer x\n"), 0o644))
}

// snapshotAntigravityEmit reads every file under .agent/ (the
// entry-point pointer `sync` owns) and .agents/ (the adapter's own
// rules/agents/skills/MCP output) and returns a relative-path -> bytes
// map. Both roots are required: walking .agent/ alone would silently
// miss the entire rules/agents payload, which defaults to the plural
// .agents/ tree.
func snapshotAntigravityEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, sub := range []string{".agent", ".agents"} {
		full := filepath.Join(root, sub)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", full, err)
		}
	}
	return out
}

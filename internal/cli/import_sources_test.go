package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// advertisedImportSources returns the names `importSources()` prints,
// which is what the help text and the unknown-source error both show.
func advertisedImportSources() []string {
	var out []string
	for _, name := range strings.Split(importSources(), ", ") {
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists pins the
// invariant that broke in #905: `importSources()` feeds both the help
// text and the "unknown source" error, while `isKnownImportSource`
// gated multi-source runs from a separate hardcoded list. The two
// drifted, so `import antigravity codex` failed with a message that
// listed `antigravity` as supported.
func TestIsKnownImportSource_AcceptsEverySourceTheHelpTextLists(t *testing.T) {
	for _, name := range advertisedImportSources() {
		if !isKnownImportSource(name) {
			t.Errorf("importSources() advertises %q but isKnownImportSource rejects it", name)
		}
	}
}

// TestRunImport_DispatchesEverySourceTheHelpTextLists closes the same
// drift one layer down. Passing validation only means the name is in a
// list; `runImport`'s switch is what actually has to handle it, and a
// name missing there reaches the unknown-source error at run time,
// after validation has already waved it through.
//
// Import failures from an empty project are expected and ignored. Only
// the unknown-source code is a failure here.
func TestRunImport_DispatchesEverySourceTheHelpTextLists(t *testing.T) {
	for _, name := range advertisedImportSources() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)

			err := runImport(dir, name, &config.Config{Sources: config.Sources{}})
			if err != nil && strings.Contains(err.Error(), "unknown source") {
				t.Errorf("importSources() advertises %q but runImport does not dispatch it: %v", name, err)
			}
		})
	}
}

// unimportableDetectedTarget returns a target `init` detects but
// `import` has no importer for. It skips the test when every detected
// target has gained an importer, since there is then nothing to skip.
func unimportableDetectedTarget(t *testing.T) (string, []string) {
	t.Helper()
	names := make([]string, 0, len(targetMarkers))
	for name := range targetMarkers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !isKnownImportSource(name) {
			return name, targetMarkers[name]
		}
	}
	t.Skip("every detected target has an importer")
	return "", nil
}

// TestImportAll_NeverDispatchesADetectedTargetWithoutAnImporter pins
// #1052: detection reads `targetMarkers`, which covers every target
// because `init` and the sync picker use it too, while dispatch reads
// `importSourceNames`. `import all` detected openhands and factory,
// then failed on each with the unknown-source error.
//
// Import failures from bare marker directories are expected and
// ignored. Only the unknown-source error is a failure here.
func TestImportAll_NeverDispatchesADetectedTargetWithoutAnImporter(t *testing.T) {
	for name, markers := range targetMarkers {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			for _, marker := range markers {
				if err := os.MkdirAll(filepath.Join(dir, marker), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			stderr := captureStderr(t, func() {
				_ = captureStdout(t, func() {
					_ = importAll(dir, &config.Config{Sources: config.Sources{}})
				})
			})
			if strings.Contains(stderr, "unknown source") {
				t.Errorf("import all detected %q but cannot dispatch it:\n%s", name, stderr)
			}
		})
	}
}

func TestImportAll_ReportsDetectedTargetWithoutAnImporterAsSkipped(t *testing.T) {
	name, markers := unimportableDetectedTarget(t)
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, markers[0]), 0o755); err != nil {
		t.Fatal(err)
	}

	var err error
	out := captureStdout(t, func() {
		err = importAll(dir, &config.Config{Sources: config.Sources{}})
	})
	if err != nil {
		t.Errorf("a skipped target must not fail import all: %v", err)
	}
	want := "skipping " + name + ": detected, but there is no importer for it"
	if !strings.Contains(out, want) {
		t.Errorf("want %q in output:\n%s", want, out)
	}
	if strings.Contains(out, "importing from "+name) {
		t.Errorf("import all must not try to import %q:\n%s", name, out)
	}
}

func TestPrintImportNextSteps_OmitsDetectedTargetWithoutAnImporter(t *testing.T) {
	name, markers := unimportableDetectedTarget(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, markers[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := captureSummary(t)
	printImportNextSteps(dir, "claude")
	if strings.Contains(buf.String(), "import "+name) {
		t.Errorf("hint suggests `import %s`, which has no importer:\n%s", name, buf.String())
	}
}

func TestScaffold_OmitsImportHintForDetectedTargetWithoutAnImporter(t *testing.T) {
	name, markers := unimportableDetectedTarget(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, markers[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	buf := captureSummary(t)
	if err := scaffold(scaffoldOptions{Root: dir, Base: "", Targets: allTargetNames()}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "agnostic-ai import "+name) {
		t.Errorf("init suggests `import %s`, which has no importer:\n%s", name, buf.String())
	}
}

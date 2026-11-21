package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testDownloadableApp(t *testing.T, parent string) string {
	t.Helper()
	app := filepath.Join(parent, companionAppName+".app")
	for path, data := range map[string][]byte{
		filepath.Join(app, "Contents", "Info.plist"):            companionInfoPlist("1.2.3"),
		filepath.Join(app, "Contents", "Resources", binaryName): []byte("cli"),
		filepath.Join(app, "Contents", "MacOS", companionName):  []byte("companion"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(app)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestCompanionInstallCommand_LeavesExistingCommandAlone(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	folder := t.TempDir()
	existing := filepath.Join(folder, binaryName)
	if err := os.WriteFile(existing, []byte("from npm"), 0755); err != nil {
		t.Fatal(err)
	}
	err := installCompanionCommand(io.Discard, app, []string{folder}, func(string) (string, error) { return existing, nil })
	raw, readErr := os.ReadFile(existing)
	if err == nil || readErr != nil || string(raw) != "from npm" {
		t.Errorf("existing command changed: %q, %v, %v", raw, err, readErr)
	}
}

func TestCompanionInstallCommand_LinksBundledCLIAndRepeats(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	folder := filepath.Join(t.TempDir(), "bin")
	missing := func(string) (string, error) { return "", os.ErrNotExist }
	for range 2 {
		if err := installCompanionCommand(io.Discard, app, []string{folder}, missing); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.Readlink(filepath.Join(folder, binaryName))
	want := filepath.Join(app, "Contents", "Resources", binaryName)
	if err != nil || got != want {
		t.Errorf("link = %q, %v; want %q", got, err, want)
	}
}

func TestCompanionUpdateApp_VerifiesBeforeStoppingAndReplacesTogether(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	stopped := false
	tools.quit = func(context.Context, string) (bool, error) { stopped = true; return false, nil }
	tools.rename = os.Rename
	started := false
	deps := companionAppDeps{
		prepare: func(_ context.Context, _ io.Writer, version, staging string) (string, error) {
			if stopped {
				t.Error("stopped before verification")
			}
			if version != "2.0.0" {
				t.Errorf("version %q", version)
			}
			built := testDownloadableApp(t, staging)
			if err := os.WriteFile(filepath.Join(built, "Contents", "Resources", binaryName), []byte("updated cli"), 0755); err != nil {
				t.Fatal(err)
			}
			return built, nil
		},
		start: func(path string) error {
			started = true
			if path != app {
				t.Errorf("start %q", path)
			}
			return nil
		},
	}
	deps.sameSigner = acceptSigner
	deps.runBuilt = acceptBuilt
	if err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", tools, deps); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", binaryName))
	if err != nil || string(raw) != "updated cli" || !stopped || !started {
		t.Errorf("update = %q, %v, stopped %v, started %v", raw, err, stopped, started)
	}
}

func TestCompanionUpdateApp_NamesTheVersionItRefusesAndTheFormat(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	deps := companionAppDeps{prepare: func(context.Context, io.Writer, string, string) (string, error) {
		t.Error("downloaded a refused version")
		return "", nil
	}}
	for _, version := range []string{"v1.2", "", "1.2.3-rc1", "latest"} {
		err := updateCompanionApp(context.Background(), io.Discard, app, version, builtTools(t), deps)
		want := fmt.Sprintf("update %s: version %q is not a stable release; use X.Y.Z, such as 1.4.0", app, version)
		if err == nil || err.Error() != want {
			t.Errorf("version %q: error %v; want %q", version, err, want)
		}
	}
}

func TestCompanionAppCommands_HelpShowsAnExampleAndTheRestart(t *testing.T) {
	for _, command := range companionAppCommands(companionTools{}) {
		if !strings.Contains(command.Example, "agnostic-ai companion "+command.Name()+" --app") {
			t.Errorf("%s example = %q", command.Name(), command.Example)
		}
	}
	update := companionAppCommands(companionTools{})[1]
	if !strings.Contains(update.Long, "quits the running companion") || !strings.Contains(update.Long, "starts it again") {
		t.Errorf("update-app long = %q", update.Long)
	}
}

func TestCompanionUpdateApp_FailedVerificationKeepsRunningApp(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("stopped on failed verification")
		return false, nil
	}
	deps := companionAppDeps{prepare: func(context.Context, io.Writer, string, string) (string, error) {
		return "", errors.New("checksum mismatch")
	}}
	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", tools, deps)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", binaryName))
	if string(raw) != "cli" {
		t.Errorf("changed CLI: %q", raw)
	}
}

func TestCompanionUpdateApp_RestoresOldAppWhenReplacementFails(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) { return false, nil }
	tools.rename = func(from, to string) error {
		if to == app && filepath.Base(from) != companionAsideName {
			return errors.New("replace failed")
		}
		return os.Rename(from, to)
	}
	restarted := false
	deps := companionAppDeps{prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
		return testDownloadableApp(t, staging), nil
	}, sameSigner: acceptSigner, runBuilt: acceptBuilt, start: func(string) error { restarted = true; return nil }}
	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", tools, deps)
	if err == nil || !restarted {
		t.Errorf("update = %v, restarted %v", err, restarted)
	}
	raw, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", binaryName))
	if string(raw) != "cli" {
		t.Errorf("old app not restored: %q", raw)
	}
}

func TestCompanionAppUpgrade_LeavesSignedBundleIntact(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	path := filepath.Join(app, "Contents", "Resources", binaryName)
	err := installVerifiedRelease(io.Discard, path, "2.0.0", "http://127.0.0.1:1", http.DefaultClient, nil)
	if err == nil || !strings.Contains(err.Error(), "Check for updates") {
		t.Errorf("bundled upgrade error = %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "cli" {
		t.Errorf("bundled CLI changed: %q", raw)
	}
}

func TestCompanionUpdateApp_NamesEachStepForTheCompanion(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.rename = os.Rename
	var out bytes.Buffer
	tools.quit = func(context.Context, string) (bool, error) {
		if !strings.HasSuffix(out.String(), "Restarting the companion.\n") {
			t.Errorf("stopped the companion after %q", out.String())
		}
		return false, nil
	}
	deps := companionAppDeps{prepare: func(_ context.Context, w io.Writer, _ string, staging string) (string, error) {
		if w.(*bytes.Buffer).String() != "Downloading agnostic-ai 2.0.0.\n" {
			t.Errorf("prepared after %q", out.String())
		}
		return testDownloadableApp(t, staging), nil
	}, sameSigner: acceptSigner, runBuilt: acceptBuilt, start: func(string) error {
		if !strings.HasSuffix(out.String(), "Replaced the companion app.\n") {
			t.Errorf("started after %q", out.String())
		}
		return nil
	}}

	if err := updateCompanionApp(context.Background(), &out, app, "2.0.0", tools, deps); err != nil {
		t.Fatal(err)
	}
}

func acceptSigner(context.Context, string, string) error { return nil }

func acceptBuilt(context.Context, string, string) error { return nil }

func TestCompanionUpdateApp_RunsNoDownloadedBinaryBeforeTheSignerCheck(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	for _, signer := range []error{errors.New("codesign: test-requirement: failed"), fmt.Errorf("%s: %w", app, errInstalledUnpinned)} {
		deps := companionAppDeps{prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
			return testDownloadableApp(t, staging), nil
		}, sameSigner: func(context.Context, string, string) error { return signer }, runBuilt: func(context.Context, string, string) error {
			t.Errorf("ran a binary from the downloaded app before refusing its signer %v", signer)
			return nil
		}}

		if err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", builtTools(t), deps); err == nil {
			t.Errorf("signer %v: updated anyway", signer)
		}
	}
}

func TestCompanionUpdateApp_RefusesAnAppWhoseBinariesFailTheirCheck(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("stopped the companion for an app whose binaries failed")
		return false, nil
	}
	signed := false
	deps := companionAppDeps{prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
		return testDownloadableApp(t, staging), nil
	}, sameSigner: func(context.Context, string, string) error { signed = true; return nil }, runBuilt: func(_ context.Context, built, version string) error {
		if !signed || filepath.Base(built) != companionAppName+".app" || version != "2.0.0" {
			t.Errorf("ran %q for %q, signer checked %v", built, version, signed)
		}
		return errors.New("companion version \"1.0.0\", expected 2.0.0")
	}}

	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", tools, deps)

	if err == nil || !strings.Contains(err.Error(), "expected 2.0.0") {
		t.Errorf("error %v", err)
	}
}

func TestCompanionUpdateApp_RefusesAnAppSignedByAnotherCertificate(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	tools := builtTools(t)
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("stopped the companion for an app signed by someone else")
		return false, nil
	}
	deps := companionAppDeps{prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
		return testDownloadableApp(t, staging), nil
	}, sameSigner: func(_ context.Context, installed, built string) error {
		if installed != app || filepath.Base(built) != companionAppName+".app" {
			t.Errorf("compared %q with %q", installed, built)
		}
		return errors.New("codesign: test-requirement: failed")
	}}

	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", tools, deps)

	if err == nil || !strings.Contains(err.Error(), "is not signed by this project") {
		t.Errorf("error %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", binaryName)); string(raw) != "cli" {
		t.Errorf("changed CLI: %q", raw)
	}
}

func TestCompanionUpdateApp_RefusesAFolderItCannotWriteBeforeDownloading(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a folder this user cannot write")
	}
	parent := t.TempDir()
	app := testDownloadableApp(t, parent)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	deps := companionAppDeps{prepare: func(context.Context, io.Writer, string, string) (string, error) {
		t.Error("downloaded for an app it cannot replace")
		return "", nil
	}}

	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", builtTools(t), deps)

	if err == nil || !strings.Contains(err.Error(), "Move the app to Applications, open it from there, then update") {
		t.Errorf("error %v", err)
	}
}

func TestPinnedRequirement_ReadsOnlyARequirementThatPinsACertificate(t *testing.T) {
	pinned := `identifier "dev.agnostic-ai.companion" and certificate leaf = H"0e01a50348aa6d59f2ccc808b8ca966bdcd25585"`
	for _, tc := range []struct {
		name, output, want string
		ok                 bool
	}{
		{"pinned", "Executable=/A.app/Contents/MacOS/agnostic-ai-companion\ndesignated => " + pinned + "\n", pinned, true},
		{"ad hoc", "Executable=/A.app/Contents/MacOS/agnostic-ai-companion\n# designated => cdhash H\"8b8f\"\n", "", false},
		{"identifier only", "designated => identifier \"dev.agnostic-ai.companion\"\n", "", false},
		{"empty", "", "", false},
	} {
		if got, ok := pinnedRequirement(tc.output); got != tc.want || ok != tc.ok {
			t.Errorf("%s: pinnedRequirement = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCompanionUpdateApp_BlamesAnInstalledAppWithoutAProjectSignature(t *testing.T) {
	app := testDownloadableApp(t, t.TempDir())
	deps := companionAppDeps{prepare: func(_ context.Context, _ io.Writer, _ string, staging string) (string, error) {
		return testDownloadableApp(t, staging), nil
	}, sameSigner: func(context.Context, string, string) error { return fmt.Errorf("%s: %w", app, errInstalledUnpinned) }}

	err := updateCompanionApp(context.Background(), io.Discard, app, "2.0.0", builtTools(t), deps)

	if err == nil || strings.Contains(err.Error(), "is not signed by this project") || !strings.Contains(err.Error(), "Download the new disk image by hand once") {
		t.Errorf("error %v", err)
	}
}

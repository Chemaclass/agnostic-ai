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
	}, start: func(string) error { restarted = true; return nil }}
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
	}, start: func(string) error { return nil }}

	if err := updateCompanionApp(context.Background(), &out, app, "2.0.0", tools, deps); err != nil {
		t.Fatal(err)
	}
}

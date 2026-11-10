package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func macBuild() companionBuild {
	return companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"}
}

// legacyLoginEntry writes the launch agent an earlier macOS install left
// when start at login was on, and returns the entries to look in.
func legacyLoginEntry(t *testing.T, program string) []string {
	t.Helper()
	agents := t.TempDir()
	entry := filepath.Join(agents, companionLaunchAgent)
	if err := os.WriteFile(entry, []byte("<plist><dict><key>ProgramArguments</key><array><string>"+program+"</string></array></dict></plist>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{entry, filepath.Join(agents, companionBundleID+".plist")}
}

// bareCompanion writes the program an install before the app bundle left on macOS.
func bareCompanion(t *testing.T, dir string) string {
	t.Helper()
	bare := filepath.Join(dir, "companion", companionName)
	if err := os.MkdirAll(filepath.Dir(bare), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bare, []byte("the bare one"), 0o700); err != nil {
		t.Fatal(err)
	}
	return bare
}

func TestCompanionFile_IsInsideTheAppOnMacOS(t *testing.T) {
	for goos, want := range map[string]string{
		"darwin":  filepath.Join("/cfg", "companion", "agnostic-ai companion.app", "Contents", "MacOS", "agnostic-ai-companion"),
		"linux":   filepath.Join("/cfg", "companion", "agnostic-ai-companion"),
		"windows": filepath.Join("/cfg", "companion", "agnostic-ai-companion.exe"),
	} {
		if got := companionFile("/cfg", goos); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

func TestCompanionIcon_IsTheProjectIcon(t *testing.T) {
	project, err := os.ReadFile(filepath.Join("..", "..", "docs", "site", "static", "icon-512.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(companionIconPNG, project) {
		t.Error("the companion's icon differs from docs/site/static/icon-512.png; copy it again")
	}
}

func TestCompanionICNS_WrapsThePNGInOneEntry(t *testing.T) {
	png := []byte("\x89PNG fake")

	icns := companionICNS(png)

	if string(icns[:4]) != "icns" || binary.BigEndian.Uint32(icns[4:8]) != uint32(len(icns)) {
		t.Fatalf("header = %q %d, file is %d bytes", icns[:4], binary.BigEndian.Uint32(icns[4:8]), len(icns))
	}
	if string(icns[8:12]) != "ic09" || binary.BigEndian.Uint32(icns[12:16]) != uint32(8+len(png)) {
		t.Errorf("entry = %q %d", icns[8:12], binary.BigEndian.Uint32(icns[12:16]))
	}
	if !bytes.Equal(icns[16:], png) {
		t.Error("the entry does not hold the PNG")
	}
}

func TestCompanionInfoPlist_EscapesTheVersion(t *testing.T) {
	plist := string(companionInfoPlist("1.2.3-4-g<abc>&"))

	if !strings.Contains(plist, "<string>1.2.3-4-g&lt;abc&gt;&amp;</string>") {
		t.Errorf("Info.plist:\n%s", plist)
	}
}

func TestInstallCompanion_BuildsASignedAppOnMacOS(t *testing.T) {
	dir := t.TempDir()
	tools := builtTools(t)
	var signed []string
	tools.sign = func(_ context.Context, app string) error {
		if _, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
			t.Errorf("signed before the app was complete: %v", err)
		}
		signed = append(signed, app)
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, macBuild(), nil)

	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "companion", "agnostic-ai companion.app")
	if raw, _ := os.ReadFile(companionFile(dir, "darwin")); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	for _, want := range []string{
		"<key>CFBundleIdentifier</key>\n\t<string>dev.agnostic-ai.companion</string>",
		"<key>CFBundleExecutable</key>\n\t<string>agnostic-ai-companion</string>",
		"<key>CFBundleIconFile</key>\n\t<string>companion</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3</string>",
		"<key>LSUIElement</key>\n\t<true/>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	if icns, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", "companion.icns")); !bytes.Equal(icns, companionICNS(companionIconPNG)) {
		t.Error("the app does not hold the agnostic-ai icon")
	}
	if len(signed) != 1 || filepath.Base(signed[0]) != "agnostic-ai companion.app" {
		t.Errorf("signed %v, want the app once", signed)
	}
	if !strings.HasSuffix(out.String(), "✓ Installed "+app+"\nStart it with: agnostic-ai companion\n") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestInstallCompanion_KeepsTheInstalledAppWhenSigningFails(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	tools := builtTools(t)
	tools.sign = func(context.Context, string) error { return errors.New("codesign: no identity") }
	tools.quit = func(context.Context, string) (bool, error) {
		t.Error("the running companion was stopped for an install that failed")
		return false, nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), nil)

	if err == nil || err.Error() != "sign the companion: codesign: no identity" {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_ReplacesTheInstalledAppAndStartsTheNewOne(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}
	tools.loginOn = func(context.Context, string) error {
		t.Error("start at login was changed for an app that keeps its place")
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), nil)

	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	if started != installed {
		t.Errorf("started %q, want %q", started, installed)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "companion"))
	if len(entries) != 1 {
		t.Errorf("the companion folder holds %d entries, want only the app", len(entries))
	}
}

func TestInstallCompanion_StartsTheOldAppAgainWhenItCannotBeReplaced(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a folder this user cannot write to")
	}
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	folder := filepath.Join(dir, "companion")
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
	tools := builtTools(t)
	var asked int
	tools.quit = runsUntilAsked(&asked)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), nil)

	if err == nil || !strings.HasPrefix(err.Error(), "install the companion: ") {
		t.Errorf("err = %v", err)
	}
	if started != installed {
		t.Errorf("started %q: the companion that ran was left stopped", started)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_MovesABareCompanionAndItsStartAtLoginIntoTheApp(t *testing.T) {
	dir := t.TempDir()
	bare := bareCompanion(t, dir)
	entries := legacyLoginEntry(t, bare)
	tools := builtTools(t)
	var asked int
	var askedWith []string
	tools.quit = func(_ context.Context, path string) (bool, error) {
		askedWith = append(askedWith, path)
		asked++
		return asked == 1, nil
	}
	var steps []string
	tools.loginOff = func(_ context.Context, path string) error {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("start at login was turned off after the bare companion was deleted: %v", err)
		}
		steps = append(steps, "login off "+path)
		return nil
	}
	tools.loginOn = func(_ context.Context, path string) error {
		if _, err := os.Stat(bare); err == nil {
			t.Error("start at login moved while the bare companion was still there")
		}
		steps = append(steps, "login on "+path)
		return nil
	}
	tools.start = func(path string, _ []string) error {
		steps = append(steps, "start "+path)
		return nil
	}
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, macBuild(), entries)

	if err != nil {
		t.Fatal(err)
	}
	inner := companionFile(dir, "darwin")
	if askedWith[0] != bare {
		t.Errorf("asked %q to quit, want the bare companion that runs", askedWith[0])
	}
	want := []string{"login off " + bare, "login on " + inner, "start " + inner}
	if strings.Join(steps, "\n") != strings.Join(want, "\n") {
		t.Errorf("steps:\n%s\nwant:\n%s", strings.Join(steps, "\n"), strings.Join(want, "\n"))
	}
	for _, gone := range append([]string{bare}, entries...) {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
	app := filepath.Join(dir, "companion", "agnostic-ai companion.app")
	wantOut := "✓ Stopped the running companion\n" +
		"✓ Installed " + app + "\n" +
		"✓ Removed the earlier companion at " + bare + "\n" +
		"✓ Start at login stays on\n" +
		"✓ Started it again\n"
	if !strings.HasSuffix(out.String(), wantOut) {
		t.Errorf("output:\n%s\nwant it to end with:\n%s", out.String(), wantOut)
	}
}

func TestInstallCompanion_LeavesStartAtLoginOffWhenTheBareCompanionHadItOff(t *testing.T) {
	dir := t.TempDir()
	bare := bareCompanion(t, dir)
	tools := builtTools(t)
	tools.loginOn = func(context.Context, string) error {
		t.Error("start at login was turned on")
		return nil
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, macBuild(), []string{filepath.Join(t.TempDir(), companionLaunchAgent)})

	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bare); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the bare companion is still there (%v)", err)
	}
}

func TestInstallCompanion_AsksToTurnStartAtLoginOnAgainWhenItCannotMoveIt(t *testing.T) {
	for name, tc := range map[string]struct {
		quit    func(context.Context, string) (bool, error)
		loginOn error
		want    string
	}{
		// A companion that may still run would take the request itself,
		// from the bare program's path.
		"it cannot tell whether the old one runs": {
			quit: func(context.Context, string) (bool, error) { return false, errors.New("no answer") },
			want: "! Start at login was on. Turn it on again in the companion's settings.\n",
		},
		"the system refuses": {
			quit:    func(context.Context, string) (bool, error) { return false, nil },
			loginOn: errors.New("Operation not permitted"),
			want:    "! Start at login was on and could not be moved to the app (Operation not permitted). Turn it on again in the companion's settings.\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			bare := bareCompanion(t, dir)
			tools := builtTools(t)
			tools.quit = tc.quit
			calls := 0
			tools.loginOn = func(context.Context, string) error {
				calls++
				return tc.loginOn
			}
			var out bytes.Buffer

			err := installCompanion(context.Background(), &out, false, tools, dir, macBuild(), legacyLoginEntry(t, bare))

			if err != nil {
				t.Fatal(err)
			}
			if tc.loginOn == nil && calls != 0 {
				t.Error("start at login was turned on while the old companion may still run")
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("output:\n%s\nwant it to hold:\n%s", out.String(), tc.want)
			}
		})
	}
}

func TestRunCompanion_StartsABareCompanionNotYetMovedIntoTheApp(t *testing.T) {
	dir := t.TempDir()
	bare := bareCompanion(t, dir)
	tools := builtTools(t)
	var started string
	tools.start = func(path string, _ []string) error {
		started = path
		return nil
	}

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, dir, "darwin", "1.2.3")

	if err != nil || started != bare {
		t.Errorf("err %v, started %q, want %q", err, started, bare)
	}
}

func TestUninstallCompanion_RemovesTheAppOnMacOS(t *testing.T) {
	dir := t.TempDir()
	installed := installedCompanion(t, dir, "darwin")
	entries := legacyLoginEntry(t, "/elsewhere/agnostic-ai-companion")
	if err := os.WriteFile(entries[1], []byte("<plist/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var loginOff string
	tools.loginOff = func(_ context.Context, path string) error {
		loginOff = path
		return nil
	}
	var out bytes.Buffer

	err := uninstallCompanion(context.Background(), &out, tools, dir, "darwin", entries)

	if err != nil {
		t.Fatal(err)
	}
	if loginOff != installed {
		t.Errorf("start at login turned off through %q, want %q", loginOff, installed)
	}
	for _, gone := range append([]string{filepath.Join(dir, "companion")}, entries...) {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s is still there (%v)", gone, err)
		}
	}
	want := "✓ Start at login is off\n" +
		"✓ Removed " + filepath.Join(dir, "companion") + "\n" +
		"Your read and archive marks stay: agnostic-ai serve uses them too.\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

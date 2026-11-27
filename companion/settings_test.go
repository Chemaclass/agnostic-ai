package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSettings_KeepsTheExistingConfigurationPath(t *testing.T) {
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(config, "agnostic-ai", "companion.json"); path != want {
		t.Errorf("settings path = %q, want %q", path, want)
	}
	if got, want := webviewDataPath(path), filepath.Join(config, "agnostic-ai", "companion", "webview"); got != want {
		t.Errorf("web view data path = %q, want %q", got, want)
	}
}

func TestSettings_AnimateIsOnWhenTheFileHasNoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Cmd+Shift+K"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if !saved.animate() {
		t.Error("a file without the key turned animation off")
	}
}

func TestSettings_KeepsAnimateOffAcrossAWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	off := false
	written := settings{Bubble: &point{X: 10, Y: 20}, Shortcut: "Cmd+Shift+K", Animate: &off}

	if err := writeSettings(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if read.animate() {
		t.Error("animation came back on")
	}
	if read.Bubble == nil || *read.Bubble != (point{X: 10, Y: 20}) || read.Shortcut != "Cmd+Shift+K" {
		t.Errorf("the other settings changed: %+v", read)
	}
}

func TestSettings_DimIsOffWhenTheFileHasNoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Cmd+Shift+K"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if saved.DimWhenIdle {
		t.Error("a file without the key turned dimming on")
	}
	if saved.SmallCompanion {
		t.Error("a file without the key made the circle small")
	}
}

func TestSettings_KeepsDimOnAcrossAWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	written := settings{Bubble: &point{X: 10, Y: 20}, Shortcut: "Cmd+Shift+K", DimWhenIdle: true, SmallCompanion: true}

	if err := writeSettings(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if !read.DimWhenIdle {
		t.Error("dimming came back off")
	}
	if !read.SmallCompanion {
		t.Error("the small circle came back regular")
	}
	if read.Bubble == nil || *read.Bubble != (point{X: 10, Y: 20}) || read.Shortcut != "Cmd+Shift+K" || !read.animate() {
		t.Errorf("the other settings changed: %+v", read)
	}
}

func TestLoadSettings_MovesAFileThatIsNotJSONAsideAndStartsWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	for _, broken := range []string{`{"shortcut":`, ""} {
		if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(path + ".bad")

		saved, err := loadSettings(path)

		if err != nil || saved.Shortcut != "" || saved.Bubble != nil {
			t.Fatalf("loadSettings(%q) = %+v, %v; want the defaults and no error", broken, saved, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the broken file is still at %s: %v", path, err)
		}
		if aside, err := os.ReadFile(path + ".bad"); err != nil || string(aside) != broken {
			t.Errorf("companion.json.bad = %q, %v; want %q kept", aside, err, broken)
		}
	}
}

func TestLoadSettings_KeepsAnEarlierBadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "companion.json")
	if err := os.WriteFile(path+".bad", []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadSettings(path); err != nil {
		t.Fatal(err)
	}

	if first, _ := os.ReadFile(path + ".bad"); string(first) != "first" {
		t.Errorf("companion.json.bad = %q; want the first bad file kept", first)
	}
	later, _ := filepath.Glob(path + ".bad-*")
	if len(later) != 1 {
		t.Fatalf("bad files with a time = %v; want one", later)
	}
	if second, _ := os.ReadFile(later[0]); string(second) != "second" {
		t.Errorf("%s = %q", later[0], second)
	}
}

func TestReadSettings_KeepsTheFieldsBesideOneOfTheWrongType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	raw := `{"shortcut":"Cmd+Shift+K","smallCompanion":"yes","bubble":{"x":10,"y":20}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := loadSettings(path)

	if err != nil || saved.Shortcut != "Cmd+Shift+K" || saved.SmallCompanion || saved.Bubble == nil || *saved.Bubble != (point{X: 10, Y: 20}) {
		t.Errorf("loadSettings = %+v, %v; want the fields that parsed", saved, err)
	}
	if kept, _ := os.ReadFile(path); string(kept) != raw {
		t.Errorf("the file changed to %q; want it left alone", kept)
	}
}

func TestReadSettings_LeavesAFileThatIsNotJSONWhereItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := readSettings(path); err == nil {
		t.Error("readSettings took a file that is not JSON")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("readSettings moved the file: %v", err)
	}
}

func TestReadSettings_LeavesAWrongTypedValueAtItsDefault(t *testing.T) {
	for name, tc := range map[string]struct {
		raw   string
		check func(settings) bool
	}{
		"animate":            {`{"animate":"yes","shortcut":"Cmd+Shift+K"}`, func(s settings) bool { return s.Animate == nil && s.animate() }},
		"whole bubble":       {`{"bubble":"x","shortcut":"Cmd+Shift+K"}`, func(s settings) bool { return s.Bubble == nil }},
		"one bubble field":   {`{"bubble":{"x":"9","y":300},"shortcut":"Cmd+Shift+K"}`, func(s settings) bool { return s.Bubble == nil }},
		"a field after them": {`{"smallCompanion":"yes","dimWhenIdle":true,"shortcut":"Cmd+Shift+K"}`, func(s settings) bool { return !s.SmallCompanion && s.DimWhenIdle }},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "companion.json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}

			saved, err := readSettings(path)

			if err != nil || saved.Shortcut != "Cmd+Shift+K" || !tc.check(saved) {
				t.Errorf("readSettings(%s) = %+v, %v", tc.raw, saved, err)
			}
		})
	}
}

// companion uninstall removes the companion folder beside the settings,
// so the web view data must live inside it.
func TestWebviewDataPath_IsInsideTheCompanionFolder(t *testing.T) {
	config := filepath.Join("C:", "Users", "ada", "AppData", "Roaming")

	got := webviewDataPath(filepath.Join(config, "agnostic-ai", "companion.json"))

	if want := filepath.Join(config, "agnostic-ai", "companion", "webview"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A drag of the circle saves its place after a web page turned the small
// circle on: the save must not write the companion's older copy back.
func TestChangeSettings_KeepsAValueAnotherProcessWrote(t *testing.T) {
	s := shellWith(t, &fakeShortcuts{})
	if err := s.changeSettings(func(saved *settings) { saved.Shortcut = "Cmd+Shift+K" }); err != nil {
		t.Fatal(err)
	}
	outside, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	outside.SmallCompanion = true
	if err := writeSettings(s.settingsPath, outside); err != nil {
		t.Fatal(err)
	}

	if err := s.changeSettings(func(saved *settings) { saved.Bubble = &point{X: 3, Y: 4} }); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(s.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.SmallCompanion || saved.Bubble == nil || *saved.Bubble != (point{X: 3, Y: 4}) || saved.Shortcut != "Cmd+Shift+K" {
		t.Errorf("saved %+v; want the outside value, the new place and the shortcut", saved)
	}
}

func TestSaveSettingsChange_WaitsForTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	release, err := lockSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- saveSettingsChange(path, settings{Shortcut: "Cmd+Shift+K"}, func(saved *settings) { saved.DimWhenIdle = true })
	}()
	select {
	case err := <-done:
		t.Fatalf("the save did not wait for the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if saved, err := readSettings(path); err != nil || !saved.DimWhenIdle || saved.Shortcut != "Cmd+Shift+K" {
		t.Errorf("saved %+v, %v", saved, err)
	}
}

// TestLoadSettings_WaitsForASaveInProgress holds the settings lock while
// a broken file is replaced with a good one: recovery waits, then reads
// the good file and moves nothing aside.
func TestLoadSettings_WaitsForASaveInProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte("{cut short"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := lockSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan settings)
	go func() {
		saved, _ := loadSettings(path)
		done <- saved
	}()
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"animate": false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	release()
	saved := <-done
	if saved.Animate == nil || *saved.Animate {
		t.Errorf("loadSettings = %+v; want the file saved while it waited", saved)
	}
	if _, err := os.Stat(path + ".bad"); !os.IsNotExist(err) {
		t.Errorf("a good file was moved aside: %v", err)
	}
}

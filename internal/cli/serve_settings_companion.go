package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/atomicfile"
)

// Companion app switches POST /api/settings takes. They change how the
// companion looks and need no proof, except turning off update checks.
const (
	settingCompanionAnimate         = "companion.animate"
	settingCompanionDimWhenIdle     = "companion.dimWhenIdle"
	settingCompanionSmall           = "companion.smallCompanion"
	settingCompanionCheckForUpdates = "companion.checkForUpdates"
)

// companionSwitchFields maps each switch to its key in companion.json.
var companionSwitchFields = map[string]string{
	settingCompanionAnimate:         "animate",
	settingCompanionDimWhenIdle:     "dimWhenIdle",
	settingCompanionSmall:           "smallCompanion",
	settingCompanionCheckForUpdates: "checkForUpdates",
}

// companionStateFile is what a running companion writes beside its
// settings and removes as it quits.
const companionStateFile = "companion-state.json"

// companionLockTries bounds the wait for the companion's settings lock to
// about 2 seconds, as the companion waits for it.
const (
	companionLockTries = 50
	companionLockPause = 40 * time.Millisecond
)

// settingsCompanionApp is the companion app's own settings. The
// companion reads its file again within 5 seconds of a change.
type settingsCompanionApp struct {
	File            string `json:"file"`
	Animate         bool   `json:"animate"`
	DimWhenIdle     bool   `json:"dimWhenIdle"`
	SmallCompanion  bool   `json:"smallCompanion"`
	CheckForUpdates bool   `json:"checkForUpdates"`
	// Running is set while a companion runs; Live then holds what only it knows.
	Running bool                   `json:"running"`
	Live    *settingsCompanionLive `json:"live,omitempty"`
	Problem string                 `json:"problem,omitempty"`
}

// settingsCompanionLive is read only: the system owns start at login,
// and a shortcut works only once the system takes its keys.
type settingsCompanionLive struct {
	Shortcut            string `json:"shortcut"`
	ShortcutProblem     string `json:"shortcutProblem,omitempty"`
	SendShortcut        string `json:"sendShortcut"`
	SendShortcutProblem string `json:"sendShortcutProblem,omitempty"`
	StartAtLogin        bool   `json:"startAtLogin"`
	StartAtLoginProblem string `json:"startAtLoginProblem,omitempty"`
}

// companionApp reads the companion's settings and state files in dir.
func companionApp(dir string) *settingsCompanionApp {
	app := &settingsCompanionApp{File: filepath.Join(dir, companionSettingsFile), Animate: true, CheckForUpdates: true}
	fields, err := readCompanionSettings(app.File)
	if err != nil {
		app.Problem = err.Error()
	}
	for key, field := range companionSwitchFields {
		var on bool
		if raw, ok := fields[field]; ok && json.Unmarshal(raw, &on) == nil {
			*app.switchFor(key) = on
		}
	}
	app.Live, app.Running = runningCompanion(filepath.Join(dir, companionStateFile))
	return app
}

func (a *settingsCompanionApp) switchFor(key string) *bool {
	switch key {
	case settingCompanionAnimate:
		return &a.Animate
	case settingCompanionDimWhenIdle:
		return &a.DimWhenIdle
	case settingCompanionSmall:
		return &a.SmallCompanion
	}
	return &a.CheckForUpdates
}

// runningCompanion reads the state file a running companion keeps. A
// file left by a companion that crashed names a process that is gone.
func runningCompanion(path string) (*settingsCompanionLive, bool) {
	raw, err := readShared(path)
	if err != nil {
		return nil, false
	}
	var state struct {
		PID int `json:"pid"`
		settingsCompanionLive
	}
	if json.Unmarshal(raw, &state) != nil {
		return nil, false
	}
	if !companionProcessRuns(state.PID) || !companionStateFresh(path) {
		return nil, false
	}
	return &state.settingsCompanionLive, true
}

// companionStateStale is how long a state file may go untouched. A
// running companion touches it every 5 seconds, so an older one was left
// by a companion that is gone, even when another process took its id.
const companionStateStale = 30 * time.Second

func companionStateFresh(path string) bool {
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) < companionStateStale
}

// readCompanionSettings returns the file's top-level fields; a missing
// file has none.
func readCompanionSettings(path string) (map[string]json.RawMessage, error) {
	raw, err := readShared(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if fields == nil {
		// A file holding null is an empty one, as the companion reads it.
		return map[string]json.RawMessage{}, nil
	}
	return fields, nil
}

// readShared reads a file the companion replaces, without holding it
// in a way that makes the companion's replace fail on Windows.
func readShared(path string) ([]byte, error) {
	file, err := atomicfile.OpenShared(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, 1<<20))
}

// lockCompanionSettings takes the lock file every writer of the
// companion's settings holds: the companion and agnostic-ai serve.
func lockCompanionSettings(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s.lock: %w", path, err)
	}
	release := func() { _ = file.Close() }
	for try := 1; ; try++ {
		err := tryLockProjectFile(file)
		if err == nil {
			return release, nil
		}
		// The companion refuses to save without the lock, so this side does too.
		if !projectLockContended(err) {
			release()
			return nil, fmt.Errorf("lock %s.lock: %w", path, err)
		}
		if try == companionLockTries {
			release()
			return nil, fmt.Errorf("lock %s.lock: the companion is saving its settings; try again: %w", path, err)
		}
		time.Sleep(companionLockPause)
	}
}

// writeCompanionSwitch sets one switch in the companion's settings and
// keeps every other field, including ones this version does not know.
// The caller holds the settings lock. A file that is not JSON is left for
// the companion, which moves it aside at its next start.
func writeCompanionSwitch(path, key string, on bool) error {
	fields, err := readCompanionSettings(path)
	if err != nil {
		return err
	}
	value, _ := json.Marshal(on)
	fields[companionSwitchFields[key]] = value
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := atomicfile.Replace(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

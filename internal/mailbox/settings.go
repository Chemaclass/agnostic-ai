package mailbox

import (
	"context"
	"path/filepath"
)

type Settings struct {
	ApproveAgentSends bool `json:"approveAgentSends"`
}

type storedSettings struct {
	ApproveAgentSends *bool `json:"approveAgentSends"`
}

func (s Store) settingsPath() string { return filepath.Join(s.Dir, "settings.json") }

// SettingsPath is the file Settings reads.
func (s Store) SettingsPath() string { return s.settingsPath() }

// Settings reads without the lock because writes replace the file atomically.
// Anything but a readable, valid file that turns approval off keeps it on.
func (s Store) Settings() Settings {
	var stored storedSettings
	if err := readJSON(s.settingsPath(), &stored); err != nil || stored.ApproveAgentSends == nil {
		return Settings{ApproveAgentSends: true}
	}
	return Settings{ApproveAgentSends: *stored.ApproveAgentSends}
}

func (s Store) SaveSettings(ctx context.Context, settings Settings) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return writeJSON(s.settingsPath(), settings)
}

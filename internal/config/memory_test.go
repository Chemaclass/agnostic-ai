package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFiles(t *testing.T, base, local string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if local != "" {
		if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName), []byte(local), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad_ReadsPersonalMemoryFromTheLocalFile(t *testing.T) {
	cfg, err := Load(writeConfigFiles(t, "version: 1\n", "memory:\n  personal: repo\n"))
	if err != nil || !cfg.RepoPersonalMemory() {
		t.Errorf("cfg = %+v, err %v", cfg, err)
	}
}

func TestLoad_RejectsPersonalMemoryInTheSharedConfig(t *testing.T) {
	_, err := Load(writeConfigFiles(t, "version: 1\nmemory:\n  personal: checkout\n", ""))
	if err == nil || !strings.Contains(err.Error(), LocalOverrideFileName) {
		t.Errorf("err = %v, want a pointer to %s", err, LocalOverrideFileName)
	}
}

func TestLoad_RejectsAnUnknownPersonalMemoryMode(t *testing.T) {
	_, err := Load(writeConfigFiles(t, "version: 1\n", "memory:\n  personal: home\n"))
	if err == nil || !strings.Contains(err.Error(), "memory.personal") {
		t.Errorf("err = %v", err)
	}
}

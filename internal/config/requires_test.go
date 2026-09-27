package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestParseRequirement_RejectsAnythingButAMinimumRelease(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "0.69.0", "0.69", ">=0.69", ">= 0.69.x", "<0.70.0", "=0.69.0", ">=0.69.0-rc.1", ">=0.69.0, <0.70.0", "latest"} {
		if _, err := ParseRequirement(in); err == nil {
			t.Errorf("ParseRequirement(%q) accepted a constraint it does not support", in)
		}
	}
}

func TestRequirement_AllowsReleasesAtOrAboveTheMinimum(t *testing.T) {
	t.Parallel()
	req, err := ParseRequirement(" >= v0.69.0 ")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		version string
		allowed bool
	}{
		{"0.68.9", false},
		{"0.69.0", true},
		{"v0.69.0", true},
		{"0.69.1", true},
		{"0.70.0", true},
		{"1.0.0", true},
		{"0.9.100", false},
	}
	for _, c := range cases {
		allowed, release := req.Allows(c.version)
		if !release || allowed != c.allowed {
			t.Errorf("Allows(%q) = %v, %v; want %v, true", c.version, allowed, release, c.allowed)
		}
	}
	if got := req.String(); got != ">=0.69.0" {
		t.Errorf("String() = %q, want >=0.69.0", got)
	}
}

func TestRequirement_DevBuildsAreNotReleases(t *testing.T) {
	t.Parallel()
	req, err := ParseRequirement(">=0.69.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"dev", "test", "", "0.70.0-rc.1", "0.69.1-next"} {
		if _, release := req.Allows(version); release {
			t.Errorf("Allows(%q) treated a non-release build as a release", version)
		}
	}
}

func TestLoad_InvalidRequiresNamesFileAndKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte("version: 1\nrequires: \"0.69\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an invalid requires to fail the load")
	}
	if errs.CodeOf(err) != errs.CodeConfigDecode || !strings.Contains(err.Error(), path+": requires:") || !strings.Contains(err.Error(), `"0.69"`) {
		t.Errorf("want AAI-004 naming %s, the key, and the value, got %v", path, err)
	}
}

func TestLoad_LocalRequiresReplacesBase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte("requires: \">=0.60.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LocalOverrideFileName), []byte("requires: \">=0.70.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Requires != ">=0.70.0" {
		t.Errorf("Requires = %q, want the local >=0.70.0", cfg.Requires)
	}
}

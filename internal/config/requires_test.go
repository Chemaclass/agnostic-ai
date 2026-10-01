package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestParseRequirement_RejectsWhatIsNotAVersionConstraint(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "  ", "0.69", ">=0.69", ">= 0.69.x", ">=0.69.0-rc.1", "0.69.0.1", ">=0.69.0, <0.70.0", ">=0.69.0<0.70.0", ">0.69.0", "~0.69.0", "latest", ">=", "0.69.0 latest"} {
		if _, err := ParseRequirement(in); err == nil {
			t.Errorf("ParseRequirement(%q) accepted a constraint it does not support", in)
		}
	}
}

func TestRequirement_ExactReleaseAllowsOnlyThatRelease(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"0.73.0", "=0.73.0", " = v0.73.0 ", "v0.73.0"} {
		req, err := ParseRequirement(in)
		if err != nil {
			t.Fatalf("ParseRequirement(%q): %v", in, err)
		}
		for version, want := range map[string]bool{"0.72.0": false, "0.72.9": false, "0.73.0": true, "v0.73.0": true, "0.73.1": false, "0.74.0": false, "1.0.0": false} {
			if allowed, release := req.Allows(version); !release || allowed != want {
				t.Errorf("%q Allows(%q) = %v, %v; want %v, true", in, version, allowed, release, want)
			}
		}
		if got := req.String(); got != "0.73.0" {
			t.Errorf("%q String() = %q, want 0.73.0", in, got)
		}
	}
}

func TestRequirement_RangeAllowsReleasesInsideIt(t *testing.T) {
	t.Parallel()
	req, err := ParseRequirement(">=0.73.0  <0.74.0")
	if err != nil {
		t.Fatal(err)
	}
	for version, want := range map[string]bool{"0.72.9": false, "0.73.0": true, "0.73.9": true, "0.74.0": false, "1.0.0": false} {
		if allowed, release := req.Allows(version); !release || allowed != want {
			t.Errorf("Allows(%q) = %v, %v; want %v, true", version, allowed, release, want)
		}
	}
	if got := req.String(); got != ">=0.73.0 <0.74.0" {
		t.Errorf("String() = %q, want >=0.73.0 <0.74.0", got)
	}

	atMost, err := ParseRequirement("<=0.73.2")
	if err != nil {
		t.Fatal(err)
	}
	if allowed, _ := atMost.Allows("0.73.2"); !allowed {
		t.Error("<=0.73.2 must allow 0.73.2")
	}
	if allowed, _ := atMost.Allows("0.73.3"); allowed {
		t.Error("<=0.73.2 must refuse 0.73.3")
	}
}

func TestRequirement_InstallTarget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		version string
		latest  bool
	}{
		{">=0.70.0", "", true},
		{"0.73.0", "0.73.0", false},
		{">=0.73.0 <0.74.0", "0.73.0", false},
		{"<0.74.0", "", false},
	}
	for _, c := range cases {
		req, err := ParseRequirement(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if version, latest := req.InstallTarget(); version != c.version || latest != c.latest {
			t.Errorf("%q InstallTarget() = %q, %v; want %q, %v", c.in, version, latest, c.version, c.latest)
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

func TestRequiresSchemaPatternMatchesTheParser(t *testing.T) {
	field, _ := reflect.TypeFor[Config]().FieldByName("Requires")
	pattern := strings.TrimPrefix(field.Tag.Get("jsonschema"), "pattern=")
	re := regexp.MustCompile(pattern)
	for _, value := range []string{
		">=0.71.0", "0.73.0", "=0.73.0", "v0.73.0", ">= 0.71.0", ">=0.73.0 <0.74.0", "  <=1.0.0  ",
		"latest", ">0.71.0", "0.73", ">=0.73.0,<0.74.0", "0.73.0x", "~0.73.0", ">=", ">=0.69.0<0.70.0", ">=0.69.0-rc.1", "0.69.0.1",
	} {
		_, err := ParseRequirement(value)
		if got, want := re.MatchString(value), err == nil; got != want {
			t.Errorf("%q: schema pattern matches=%v, parser accepts=%v", value, got, want)
		}
	}
}

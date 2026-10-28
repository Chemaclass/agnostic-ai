package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestLoad_WrongValueShapeNamesFileLineAndExpectedShape(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{"map", "version: 1\ngitignore: yes-please\n", `:2: gitignore: expected a map, got the string "yes-please"`},
		{"list", "version: 1\ntargets: claude\n", `:2: targets: expected a list, got the string "claude"`},
		{"number", "version: \"x\"\n", `:1: version: expected a whole number, got the string "x"`},
		{"nested", "version: 1\nsync:\n  target-overview: [a]\n", `:3: sync.target-overview: expected true or false, got a list`},
		{"document", "hello\n", `:1: expected a map, got the string "hello"`},
		{"alias", "version: 1\nname: &n claude\ntargets: *n\n", `:3: targets: expected a list, got the string "claude"`},
		{"list item", "version: 1\ntargets:\n  - claude\n  - {a: b}\n", `:4: targets[1]: expected a string, got a map`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeConfigFiles(t, tc.yaml, "")
			_, err := Load(dir)
			want := filepath.Join(dir, ConfigFileName) + tc.want
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
			if strings.Contains(err.Error(), "unmarshal") || strings.Contains(err.Error(), "config.") || strings.Contains(err.Error(), "!!") {
				t.Errorf("message leaks decoder internals: %v", err)
			}
			if errs.CodeOf(err) != errs.CodeConfigDecode {
				t.Errorf("code = %q, want %s", errs.CodeOf(err), errs.CodeConfigDecode)
			}
		})
	}
}

func TestLoad_WrongValueShapeInLocalFileNamesThatFile(t *testing.T) {
	dir := writeConfigFiles(t, "version: 1\ntargets: [claude]\n", "# personal\ngitignore: yes-please\n")
	_, err := Load(dir)
	want := filepath.Join(dir, LocalOverrideFileName) + `:2: gitignore: expected a map, got the string "yes-please"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), "merged") {
		t.Errorf("message points at the internal merged document: %v", err)
	}
}

func TestLoad_RejectsAnUnsupportedVersion(t *testing.T) {
	for _, local := range []string{"", "version: 7\n", "version: 7.0\n", "version: 1.5\n"} {
		base := "version: 7\n"
		if local != "" {
			base = "version: 1\n"
		}
		_, err := Load(writeConfigFiles(t, base, local))
		if err == nil || !strings.Contains(err.Error(), "is not supported") || !strings.Contains(err.Error(), "version: 1") {
			t.Errorf("local %q: err = %v, want a refusal naming version 1", local, err)
		}
		if errs.CodeOf(err) != errs.CodeConfigDecode {
			t.Errorf("code = %q, want %s", errs.CodeOf(err), errs.CodeConfigDecode)
		}
	}
	if _, err := Load(writeConfigFiles(t, "version: 1\n", "")); err != nil {
		t.Errorf("version 1 refused: %v", err)
	}
	if _, err := Load(writeConfigFiles(t, "targets: [claude]\n", "")); err != nil {
		t.Errorf("config without version refused: %v", err)
	}
}

func TestLoad_NonNumericVersionReportsOnlyTheShape(t *testing.T) {
	_, err := Load(writeConfigFiles(t, "version: \"x\"\n", ""))
	if err == nil || strings.Count(err.Error(), "version:") != 1 {
		t.Errorf("err = %v, want one finding for version", err)
	}
}

func TestLoad_MergedConfigTypeErrorNamesTheFileThatHoldsIt(t *testing.T) {
	for _, tc := range []struct {
		name, base, local, file, want string
	}{
		{"local", "version: 1\n", "# local\non-unsupported: [warn]\n", LocalOverrideFileName, ":2: on-unsupported: expected a string, got a list"},
		{"base", "version: 1\n# shared\non-unsupported: [warn]\n", "targets: [claude]\n", ConfigFileName, ":3: on-unsupported: expected a string, got a list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeConfigFiles(t, tc.base, tc.local)
			_, err := Load(dir)
			want := filepath.Join(dir, tc.file) + tc.want
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want %q", err, want)
			}
			if errs.CodeOf(err) != errs.CodeConfigDecode {
				t.Errorf("code = %q, want %s", errs.CodeOf(err), errs.CodeConfigDecode)
			}
		})
	}
}

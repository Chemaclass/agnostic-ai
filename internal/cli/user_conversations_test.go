package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoadUserConversations_UsesTheSharedSettingUntilTheLocalFileOverridesIt(t *testing.T) {
	source := t.TempDir()
	after, err := loadUserConversations(source)
	if err != nil || after != nil {
		t.Errorf("default threshold = %v, %v; want the scanner default", after, err)
	}
	writeFile(t, filepath.Join(source, config.ConfigFileName), "version: 1\nconversations:\n  stuckShellAfter: 45m\n")
	after, err = loadUserConversations(source)
	if err != nil || after == nil || *after != 45*time.Minute {
		t.Errorf("shared threshold = %v, %v; want 45m", after, err)
	}
	local := filepath.Join(source, "local", config.ConfigFileName)
	writeFile(t, local, "version: 1\nconversations: {}\n")
	after, err = loadUserConversations(source)
	if err != nil || after == nil || *after != 45*time.Minute {
		t.Errorf("empty local layer threshold = %v, %v; want 45m", after, err)
	}
	writeFile(t, local, "version: 1\nconversations:\n  stuckShellAfter: 0\n")
	after, err = loadUserConversations(source)
	if err != nil || after == nil || *after != 0 {
		t.Errorf("override threshold = %v, %v; want explicit zero", after, err)
	}
}

func TestLoadUserConversations_NamesTheInvalidFileAndSetting(t *testing.T) {
	for _, value := range []string{"-1m", "later", "10", "[1h]"} {
		t.Run(value, func(t *testing.T) {
			source := t.TempDir()
			file := filepath.Join(source, "local", config.ConfigFileName)
			writeFile(t, file, "version: 1\nconversations:\n  stuckShellAfter: "+value+"\n")
			_, err := loadUserConversations(source)
			if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "stuckShellAfter") {
				t.Errorf("load with %s = %v", value, err)
			}
		})
	}
}

func TestServe_UsesHomeShellSettingsInBothScopes(t *testing.T) {
	for _, scope := range []string{"project", "global"} {
		for _, value := range []string{"0", "broken"} {
			t.Run(scope+"/"+value, func(t *testing.T) {
				source := t.TempDir()
				t.Setenv(envUserRoot, source)
				settings := filepath.Join(source, config.ConfigFileName)
				writeFile(t, settings, "version: 1\nconversations:\n  stuckShellAfter: "+value+"\n")
				args := []string{"--api-only", "--addr", "127.0.0.1:0", "--stop-with-stdin"}
				if scope == "global" {
					args = append(args, "--scope", "user")
				} else {
					args = append(args, "--dir", t.TempDir())
				}
				cmd := newServeCmd()
				var out, warnings bytes.Buffer
				cmd.SetArgs(args)
				cmd.SetIn(strings.NewReader(""))
				cmd.SetOut(&out)
				cmd.SetErr(&warnings)
				if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), "Serving agnostic-ai at http://127.0.0.1:") {
					t.Errorf("serve startup = %q, %v", out.String(), err)
				}
				if text := warnings.String(); value == "broken" {
					if !strings.Contains(text, settings) || !strings.Contains(text, "disabling the stuck shell warning and stop action") {
						t.Errorf("invalid threshold warning = %q", text)
					}
				} else if text != "" {
					t.Errorf("valid threshold warning = %q", text)
				}
			})
		}
	}
}

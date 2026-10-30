package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEditField_GlobalVoiceSavesAndClearsNestedMembers(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	original := "# Keep these native keys.\nx-claude:\n  verbose: false\n"
	mustWriteGlobalTest(t, file, original)
	h := newEditHandler(source, true)
	for _, tc := range []struct {
		key, raw string
		want     map[string]any
	}{
		{"enabled", "true", map[string]any{"enabled": true}},
		{"mode", `"hold"`, map[string]any{"enabled": true, "mode": "hold"}},
		{"language", `"es"`, map[string]any{"enabled": true, "mode": "hold", "language": "es"}},
		{"enabled", "false", map[string]any{"enabled": false, "mode": "hold", "language": "es"}},
		{"mode", `"tap"`, map[string]any{"enabled": false, "mode": "tap", "language": "es"}},
		{"enabled", "null", map[string]any{"mode": "tap", "language": "es"}},
		{"mode", "null", map[string]any{"language": "es"}},
		{"language", "null", nil},
	} {
		body := fmt.Sprintf(`{"audience":"shared","key":"voice.%s","target":"all","value":%s}`, tc.key, tc.raw)
		rec := postEdit(t, h, editFieldPath, body, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", body, rec.Code, rec.Body)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		voice, present := doc["voice"]
		if tc.want == nil {
			if present {
				t.Errorf("last clear left voice: %v", voice)
			}
		} else if !reflect.DeepEqual(voice, tc.want) {
			t.Errorf("after %s: voice = %#v, want %#v", body, voice, tc.want)
		}
		for key := range doc {
			if strings.HasPrefix(key, "voice.") {
				t.Errorf("saved a flat key %q instead of a nested voice member", key)
			}
		}
		if !strings.Contains(string(data), original) {
			t.Errorf("edit changed unrelated bytes: %s", data)
		}
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != original {
		t.Errorf("clearing voice changed original bytes: %q, %v", data, err)
	}
}

func TestEditField_GlobalVoiceRemovesEmptiedServerScaffold(t *testing.T) {
	silence(t)
	source := editHome(t)
	if err := os.Remove(filepath.Join(source, "settings", "native.yaml")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "settings", "defaults.yaml")
	for _, tc := range []struct{ key, raw string }{
		{"enabled", "true"}, {"mode", `"tap"`}, {"language", `"es"`},
		{"enabled", "null"}, {"mode", "null"}, {"language", "null"},
	} {
		body := fmt.Sprintf(`{"audience":"shared","key":"voice.%s","target":"all","value":%s}`, tc.key, tc.raw)
		if rec := postEdit(t, newEditHandler(source, true), editFieldPath, body, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", body, rec.Code, rec.Body)
		}
		if tc.raw != "null" {
			data, err := os.ReadFile(file)
			if err != nil || !strings.HasPrefix(string(data), fieldCreatedMarker) {
				t.Fatalf("missing marked scaffold: %q, %v", data, err)
			}
		}
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("empty server scaffold remains: %v", err)
	}
}

func TestEditField_GlobalVoiceRejectsInvalidValuesWithoutWriting(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	h := newEditHandler(source, true)
	for _, tc := range []struct{ key, raw string }{
		{"enabled", `"true"`}, {"enabled", "1"}, {"enabled", "{}"},
		{"mode", `"toggle"`}, {"mode", `""`}, {"mode", "true"},
		{"language", `""`}, {"language", `"   "`}, {"language", "2"}, {"language", "false"},
	} {
		body := fmt.Sprintf(`{"audience":"shared","key":"voice.%s","target":"all","value":%s}`, tc.key, tc.raw)
		if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != string(original) {
		t.Errorf("invalid values changed settings: %q, %v", data, err)
	}
}

func TestEditField_GlobalVoiceRejectsPerToolTargets(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	for _, tc := range []struct{ key, raw string }{
		{"enabled", "true"}, {"mode", `"tap"`}, {"language", `"es"`},
	} {
		for _, target := range []string{"claude", "codex"} {
			for _, value := range []string{tc.raw, "null"} {
				body := fmt.Sprintf(`{"audience":"shared","key":"voice.%s","target":%q,"value":%s}`, tc.key, target, value)
				if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code != http.StatusBadRequest {
					t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
				}
			}
		}
	}
}

func TestEditField_ProjectVoiceIsReadOnly(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWriteGlobalTest(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	h := newEditHandler(project, false)
	for _, tc := range []struct{ key, raw string }{
		{"enabled", "true"}, {"mode", `"tap"`}, {"language", `"es"`},
	} {
		for _, value := range []string{tc.raw, "null"} {
			body := fmt.Sprintf(`{"audience":"shared","key":"voice.%s","target":"all","value":%s}`, tc.key, value)
			if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code != http.StatusBadRequest {
				t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".agnostic-ai", "settings", "defaults.yaml")); !os.IsNotExist(err) {
		t.Errorf("project voice edit created a settings file: %v", err)
	}
}

func TestEditField_VoicePersonalMembersAndStaleValuesStaySeparate(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	original := "name: native\nvoice:\n  enabled: true\n  mode: tap\n  language: es\n"
	mustWriteGlobalTest(t, file, original)
	h := newEditHandler(source, true)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"audience":"shared","key":"voice.mode","value":"hold","expected":"tap"}`, 200},
		{`{"audience":"shared","key":"voice.mode","value":"tap","expected":"tap"}`, 409},
		{`{"audience":"personal","key":"voice.enabled","value":false,"expected":null}`, 200},
		{`{"audience":"shared","key":"voice.mode","value":"tap","expected":"hold"}`, 200},
		{`{"audience":"shared","key":"voice.enabled","value":false,"expected":"true"}`, 409},
		{`{"audience":"personal","key":"voice.mode","value":null,"expected":null}`, 200},
		{`{"audience":"personal","key":"voice.enabled","value":null,"expected":"true"}`, 409},
		{`{"audience":"personal","key":"voice.enabled","value":null,"expected":"false"}`, 200},
	} {
		if rec := postEdit(t, h, editFieldPath, tc.body, nil); rec.Code != tc.code {
			t.Fatalf("%s = %d %s, want %d", tc.body, rec.Code, rec.Body, tc.code)
		}
	}
	if got := readGlobalTest(t, file); got != original {
		t.Errorf("personal edits changed shared voice: %s", got)
	}
	if _, err := os.Stat(filepath.Join(source, "local", "settings", "native.yaml")); !os.IsNotExist(err) {
		t.Errorf("reset left the server-created personal scaffold: %v", err)
	}
}

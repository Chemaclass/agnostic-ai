package cli

import (
	"bytes"
	"testing"
)

func TestGrokTOML_ModelAddRemoveRestoresBytes(t *testing.T) {
	for _, base := range []string{"", "[models]", "# keep\n[plugins]\nenabled = true\n", "# keep\n[models]\n# keep model notes\nother = 'keep'\n[plugins]\nx = 1\n", "# no trailing newline\n[plugins]\nx = 1"} {
		t.Run(base, func(t *testing.T) {
			want := []globalSetting{{target: "grok", key: "models.default", value: "grok-code", field: "model"}}
			added, err := mergeGlobalSettings("config.toml", grokTOMLFormat, []byte(base), want, nil)
			if err != nil {
				t.Fatal(err)
			}
			removed, err := mergeGlobalSettings("config.toml", grokTOMLFormat, added.data, nil, added.owned)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(removed.data, []byte(base)) {
				t.Errorf("restored %q want %q", removed.data, base)
			}
		})
	}
}
func TestGrokTOML_DistinguishesLiteralDottedKey(t *testing.T) {
	base := []byte("\"models.default\" = \"literal\"\n[models]\ndefault = \"native\" # keep\nother = 1\n")
	want := []globalSetting{{target: "grok", key: "models.default", value: "changed", field: "model"}}
	result, err := mergeGlobalSettings("config.toml", grokTOMLFormat, base, want, map[string]any{"models.default": "native"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.data, []byte("\"models.default\" = \"literal\"")) || !bytes.Contains(result.data, []byte("default = \"changed\" # keep")) {
		t.Fatalf("wrong key edited: %s", result.data)
	}
}

func TestGrokTOML_VoiceAddRemoveRestoresBytes(t *testing.T) {
	for _, base := range []string{"", "[ui]", "[ui]\n", "# keep\n[plugins]\nenabled = true\n", "# keep\n[ui]\n# keep voice notes\ntheme = 'dark'\n[plugins]\nx = 1\n", "# no trailing newline\n[plugins]\nx = 1"} {
		t.Run(base, func(t *testing.T) {
			want := []globalSetting{
				{target: "grok", key: "ui.voice_capture_mode", value: "toggle", field: "voice.mode"},
				{target: "grok", key: "ui.voice_stt_language", value: "es", field: "voice.language"},
			}
			added, err := mergeGlobalSettings("config.toml", grokTOMLFormat, []byte(base), want, nil)
			if err != nil {
				t.Fatal(err)
			}
			values, err := settingsValues("config.toml", grokTOMLFormat, added.data)
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := settingsLookup(values, "ui.voice_capture_mode", grokTOMLFormat); got != "toggle" {
				t.Errorf("voice mode = %v", got)
			}
			removed, err := mergeGlobalSettings("config.toml", grokTOMLFormat, added.data, nil, added.owned)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(removed.data, []byte(base)) {
				t.Errorf("restored %q want %q", removed.data, base)
			}
		})
	}
}

func TestGrokTOML_ModelAndVoiceContainersRestoreIndependently(t *testing.T) {
	base := []byte("# keep\n[plugins]\nenabled = true\n")
	want := []globalSetting{
		{target: "grok", key: "models.default", value: "grok-code", field: "model"},
		{target: "grok", key: "ui.voice_capture_mode", value: "toggle", field: "voice.mode"},
	}
	added, err := mergeGlobalSettings("config.toml", grokTOMLFormat, base, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	voiceOnly, err := mergeGlobalSettings("config.toml", grokTOMLFormat, added.data, want[1:], added.owned)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(voiceOnly.data, []byte("[models]")) || !bytes.Contains(voiceOnly.data, []byte("[ui]")) {
		t.Errorf("independent model removal = %s", voiceOnly.data)
	}
	removed, err := mergeGlobalSettings("config.toml", grokTOMLFormat, voiceOnly.data, nil, voiceOnly.owned)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(removed.data, base) {
		t.Errorf("restored %q want %q", removed.data, base)
	}
}

func TestGrokTOML_EmptyUIOverrideWinsOverExistingValues(t *testing.T) {
	base := []byte("# keep\n[ui]\nvoice_capture_mode = 'hold'\nvoice_stt_language = 'en'\n[plugins]\nenabled = true\n")
	want := []globalSetting{{target: "grok", key: "ui", value: map[string]any{}, field: "x-grok.ui"}}
	result, err := mergeGlobalSettings("config.toml", grokTOMLFormat, base, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	values, err := settingsValues("config.toml", grokTOMLFormat, result.data)
	if err != nil {
		t.Fatal(err)
	}
	if got, found := settingsLookup(values, "ui", grokTOMLFormat); !found || !sameSetting(got, map[string]any{}) {
		t.Errorf("ui = %v, want empty table", got)
	}
	if !bytes.Contains(result.data, []byte("# keep\n")) || !bytes.Contains(result.data, []byte("[plugins]\nenabled = true\n")) {
		t.Errorf("unrelated content changed: %s", result.data)
	}
}

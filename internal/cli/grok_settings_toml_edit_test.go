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

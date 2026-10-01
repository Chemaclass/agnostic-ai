package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func keysIn(t *testing.T, body string) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	keys, err := unknownKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, k := range keys {
		keys[i] = k[len(path):]
	}
	return keys
}

func TestUnknownKeys_MergeKeysAreCheckedInPlace(t *testing.T) {
	body := "version: 1\nbase: &b\n  provenance-header: false\noutputs:\n  claude: *b\n  cursor:\n    <<: *b\n    rules-dir: x\n"
	if got := keysIn(t, body); !slices.Equal(got, []string{`:2: unknown key "base"`}) {
		t.Errorf("unknown keys = %q", got)
	}

	body = "version: 1\nx: &bad\n  rules-dirr: x\noutputs:\n  cursor:\n    <<: [*bad]\n"
	got := keysIn(t, body)
	if !slices.Contains(got, `:3: unknown key "outputs.cursor.rules-dirr" (did you mean rules-dir?)`) {
		t.Errorf("unknown keys = %q", got)
	}
}

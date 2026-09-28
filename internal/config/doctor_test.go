package config

import "testing"

func TestConfig_IgnoresReference(t *testing.T) {
	cfg := &Config{Doctor: DoctorConfig{CheckReferences: CheckReferencesConfig{Ignore: []string{"url", "*-url"}}}}
	cases := []struct {
		name string
		dest string
		want bool
	}{
		{"exact placeholder", "url", true},
		{"glob placeholder", "gcp-url", true},
		{"real link stays flagged", "references/setup.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.IgnoresReference(tc.dest); got != tc.want {
				t.Errorf("IgnoresReference(%q) = %v, want %v", tc.dest, got, tc.want)
			}
		})
	}
}

func TestConfig_IgnoresReference_NilSafe(t *testing.T) {
	var cfg *Config
	if cfg.IgnoresReference("url") {
		t.Error("nil config must ignore nothing")
	}
}

package overview

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestSettingsFields_ExposeEditContract(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude"}, Sources: config.Sources{Settings: ".agnostic-ai/settings"}}
	fields := buildFields(SectionSettings, nil, nil, cfg, false)
	byKey := map[string]SettingField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if got := byKey["effort"].Editable; got == nil || got.Path != ".agnostic-ai/settings/defaults.yaml" || !slices.Contains(got.Allowed, "high") {
		t.Fatalf("effort metadata = %+v", got)
	}
	if got := byKey["session-retention-days"].Editable; got == nil || got.Kind != "number" || got.DisabledReason == "" {
		t.Fatalf("project retention = %+v", got)
	}
	fields = buildFields(SectionSettings, nil, nil, cfg, true)
	for _, f := range fields {
		if f.Key == "session-retention-days" && (f.Editable == nil || f.Editable.Path != "settings/defaults.yaml" || f.Editable.DisabledReason != "") {
			t.Fatalf("global retention = %+v", f.Editable)
		}
	}
}

package zed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ValidatesNativeSkillNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		valid bool
	}{
		{"a", true},
		{"deploy-v2", true},
		{strings.Repeat("a", 64), true},
		{"Deploy", false},
		{"my_skill", false},
		{"my--skill", false},
		{"-deploy", false},
		{"deploy-", false},
		{strings.Repeat("a", 65), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.TempCwd(t)
			b := spec.NewBundle([]spec.Entry{{Kind: spec.KindSkill, Name: tt.name, Body: "Deploy safely."}})
			err := New().Emit(emit.NewSession(), b, &config.Config{}, false)
			path := filepath.Join(".agents", "skills", tt.name, "SKILL.md")
			if tt.valid {
				if err != nil {
					t.Fatalf("valid name: %v", err)
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil || !strings.Contains(string(data), "Deploy safely.") {
					t.Errorf("skill missing: %s (%v)", data, readErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "1-64") || !strings.Contains(err.Error(), tt.name) {
				t.Errorf("invalid name needs an actionable error: %v", err)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("invalid skill was emitted: %v", statErr)
			}
		})
	}
}

package grok

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) ValidateSkill(skill spec.Entry) error {
	return validateSkillIdentity(skill, emit.ResolveMeta(skill.Meta, target)["name"])
}

func validateSkillIdentity(entry spec.Entry, native any) error {
	var declared string
	switch value := native.(type) {
	case string:
		declared = value
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		declared = fmt.Sprint(value)
	}
	for _, candidate := range []string{declared, entry.Name} {
		if name := normalizedSkillName(candidate); name != "" && len(name) <= 64 {
			return nil
		}
	}
	return fmt.Errorf("grok %s %q: native name and filename fallback must normalize to 1-64 ASCII letters, digits or hyphens", entry.Kind, entry.Name)
}

func normalizedSkillName(name string) string {
	var out strings.Builder
	hyphen := false
	for _, char := range strings.TrimSpace(name) {
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			hyphen = false
		} else if !hyphen {
			out.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(out.String(), "-")
}

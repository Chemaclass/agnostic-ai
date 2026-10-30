package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const sessionRetentionField = "session-retention-days"

func sessionRetentionIssues(settings []spec.Entry) []validationIssue {
	var issues []validationIssue
	for _, entry := range settings {
		if value, present := entry.Meta[sessionRetentionField]; present {
			if _, valid := spec.SessionRetentionDays(value); !valid {
				issues = append(issues, validationIssue{Path: entry.Path, Field: sessionRetentionField, Message: "session-retention-days must be an integer of 1 or more", entry: entry})
			}
		}
	}
	return issues
}

func globalRetentionSetting(target string, f globalSettingsFile, settings []spec.Entry) []globalSetting {
	var out []globalSetting
	for _, entry := range settings {
		value, present := entry.Meta[sessionRetentionField]
		if !present || !entry.EmitsTo(target) {
			continue
		}
		days, valid := spec.SessionRetentionDays(value)
		if !valid {
			continue
		}
		if f.retention == "" {
			reason := "no documented retention setting for session files"
			switch target {
			case "kilo":
				reason = "session retention needs a separate design for Kilo's deletion switch"
			case "factory":
				reason = "session retention is controlled by the organization"
			}
			adapters.NoteSettingsFieldNoOp(target, sessionRetentionField, 1, reason)
			continue
		}
		var native any = days
		if target != "claude" && target != "codebuddy" {
			native = strconv.FormatUint(days, 10) + "d"
		}
		out = []globalSetting{{target: target, key: f.retention, field: sessionRetentionField, source: entry.Path, value: native}}
	}
	return out
}

func warnShorterRetention(path, format, key string, before map[string]any, after []byte, warn io.Writer) error {
	if enabled, present := settingsLookup(before, "general.sessionRetention.enabled", format); present && enabled == false {
		return nil
	}
	values, err := settingsValues(path, format, after)
	if err != nil {
		return err
	}
	if enabled, present := settingsLookup(values, "general.sessionRetention.enabled", format); present && enabled == false {
		return nil
	}
	age := func(settings map[string]any) (float64, bool) {
		if value, present := settingsLookup(settings, key, format); present {
			return nativeRetentionDays(value)
		}
		return 30, true
	}
	oldDays, oldOK := age(before)
	newDays, newOK := age(values)
	if oldOK && newOK && newDays < oldDays {
		if _, err := fmt.Fprintf(warn, "warning: %s: lowering session retention from %g to %g days deletes older sessions\n", path, oldDays, newDays); err != nil {
			return fmt.Errorf("write session retention warning: %w", err)
		}
	}
	return nil
}

func nativeRetentionDays(value any) (float64, bool) {
	if days, valid := spec.SessionRetentionDays(value); valid {
		return float64(days), true
	}
	if number, ok := value.(float64); ok {
		return number, number > 0
	}
	text, ok := value.(string)
	if !ok || len(text) < 2 {
		return 0, false
	}
	count, err := strconv.ParseFloat(text[:len(text)-1], 64)
	if err != nil || count <= 0 {
		return 0, false
	}
	units := map[byte]float64{'s': 1.0 / 86400, 'm': 1.0 / 1440, 'h': 1.0 / 24, 'd': 1, 'w': 7}
	unit, valid := units[text[len(text)-1]]
	return count * unit, valid
}

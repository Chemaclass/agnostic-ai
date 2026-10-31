package cli

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func voiceTargetNoOpReason(target string) string {
	switch target {
	case "codex":
		return "no voice dictation setting sync --global can write; Codex realtime settings are experimental"
	case "codebuddy":
		return "no documented voice key; language sets the response language"
	case "qoder":
		return "no documented voice key; QODER_ASR_URL selects a speech recognition service"
	case "copilot":
		return "Copilot CLI voice settings keys are not published"
	case "warp":
		return "Warp voice settings need a separate mapping; see #2836"
	case "kiro":
		return "Kiro CLI voice settings need a separate mapping; see #2837; no IDE voice setting was found"
	case "goose":
		return "Goose dictation selects a provider and model; no portable on/off mapping"
	case "kilo":
		return "Kilo speech recognition selects a model and service; no dictation on/off key"
	case "factory":
		return "Factory dictation is controlled by the organization"
	case "aider":
		return "Aider starts dictation with /voice; file settings tune recording"
	case "antigravity":
		return "Antigravity starts dictation with /voice, /record, or F5; no documented settings key"
	case "amp":
		return "Amp native apps provide dictation; no CLI settings key"
	case "cline":
		return "Cline Desktop voice has no documented file key; VS Code dictation was removed"
	case "cursor":
		return "Cursor Voice Mode has no documented file key"
	case "junie":
		return "Junie starts dictation with /voice; no documented configuration key"
	case "openhands":
		return "OpenHands voice settings live in browser storage"
	case "opencode":
		return "opencode voice is experimental desktop state; no personal settings key"
	case "trae":
		return "Trae voice file keys remain unverified"
	case "windsurf":
		return "Windsurf Cascade voice was removed; no personal settings key"
	case "augment", "continue", "crush", "kimi", "pi":
		return "no voice setting found in the tool's documented configuration or source"
	case "ellipsis":
		return "Ellipsis is a repository review service with no voice setting"
	case "jules":
		return "Jules audio changelogs are output; no voice input setting"
	case "zed":
		return "Zed microphone settings control calls; no dictation setting"
	default:
		return "no voice dictation setting sync --global can write"
	}
}

func noteQwenVoiceModel(values map[string]any, want []globalSetting) {
	if !slices.ContainsFunc(want, func(setting globalSetting) bool {
		return setting.target == "qwen" && strings.HasPrefix(setting.field, "voice.")
	}) {
		return
	}
	model, _ := values["voiceModel"].(string)
	if model == "" {
		adapters.NoteSettingsFieldNoOp("qwen", "voice", 1, "Qwen keeps dictation off until a voice model is chosen; run /model --voice or set x-qwen.voiceModel")
	}
}

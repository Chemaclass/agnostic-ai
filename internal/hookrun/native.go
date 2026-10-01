package hookrun

import (
	"encoding/json"
	"slices"
)

// nativeHandler is one handler in the hooks block Claude Code, Codex,
// and Gemini CLI share: per event, matcher groups that list handlers.
type nativeHandler struct {
	Type           string   `json:"type"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	CommandWindows string   `json:"commandWindows"`
}

// runs compares what the target starts on goos: Codex runs
// commandWindows on Windows and command elsewhere. Each target needs
// type command to run a command at all.
func (n nativeHandler) runs(h Handler, goos string) bool {
	if n.Type != "command" {
		return false
	}
	if goos == "windows" && n.CommandWindows != h.CommandWindows {
		return false
	}
	return n.Command == h.Command && slices.Equal(n.Args, h.Args)
}

// Unsynced returns the handlers that no handler under event in the
// synced native file body runs on goos: the file is stale or edited by
// hand. The matcher is not compared, since sync can merge matchers.
func Unsynced(body []byte, event, goos string, handlers []Handler) ([]Handler, error) {
	var doc struct {
		Hooks map[string][]struct {
			Hooks []nativeHandler `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var native []nativeHandler
	for _, group := range doc.Hooks[event] {
		native = append(native, group.Hooks...)
	}
	var missing []Handler
	for _, h := range handlers {
		if !slices.ContainsFunc(native, func(n nativeHandler) bool { return n.runs(h, goos) }) {
			missing = append(missing, h)
		}
	}
	return missing, nil
}

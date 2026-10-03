package hookrun

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// nativeGroup is one matcher group in the hooks block Claude Code,
// Codex, and Gemini CLI share: per event, groups that list handlers.
type nativeGroup struct {
	Matcher string          `json:"matcher"`
	Hooks   []nativeHandler `json:"hooks"`
}

type nativeHandler struct {
	Type           string            `json:"type"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	CommandWindows string            `json:"commandWindows"`
	Timeout        float64           `json:"timeout"`
	Env            map[string]string `json:"env"`
	OnFailure      string            `json:"on_failure"`
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

// timeout reads the native timeout: milliseconds on Gemini CLI and
// Augment, seconds elsewhere.
func (n nativeHandler) timeout(target string) time.Duration {
	if target == "gemini" || target == "augment" {
		return time.Duration(n.Timeout * float64(time.Millisecond))
	}
	return time.Duration(n.Timeout * float64(time.Second))
}

// HandlerDrift is a handler the synced native file does not run as the
// spec says, and why.
type HandlerDrift struct {
	Handler Handler
	Reason  string
}

// Drift compares each handler with the handlers under event in the
// synced native file body: the command (with args), the group matcher,
// the timeout, and, on Gemini CLI, the handler env. covers reports
// whether a native matcher can be what sync wrote for the spec's, since
// Codex joins the matchers of specs that share a command.
func Drift(target string, body []byte, event, matcher, goos string, handlers []Handler, covers func(native, spec string) bool) ([]HandlerDrift, error) {
	if target == "cursor" {
		return cursorDrift(body, event, matcher, handlers)
	}
	if target == "crush" {
		return crushDrift(body, matcher, handlers)
	}
	if target == "copilot" {
		return copilotDrift(body, event, matcher, handlers)
	}
	var doc struct {
		Hooks map[string][]nativeGroup `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var drift []HandlerDrift
	for _, h := range handlers {
		reason := fmt.Sprintf("has no %s command %q", event, shownCommand(h, goos))
		for _, group := range doc.Hooks[event] {
			for _, n := range group.Hooks {
				if !n.runs(h, goos) {
					continue
				}
				reason = fieldDrift(target, group.Matcher, matcher, n, h, covers)
				if reason == "" {
					break
				}
			}
			if reason == "" {
				break
			}
		}
		if reason != "" {
			drift = append(drift, HandlerDrift{Handler: h, Reason: reason})
		}
	}
	return drift, nil
}

func fieldDrift(target, nativeMatcher, matcher string, n nativeHandler, h Handler, covers func(native, spec string) bool) string {
	if !covers(nativeMatcher, matcher) {
		return fmt.Sprintf("runs %q with matcher %q, not %q", h.Command, nativeMatcher, matcher)
	}
	// Codex keeps the first timeout among specs that share a command, so
	// a spec without one cannot tell a stale value from a sibling's.
	if got := n.timeout(target); got != h.Timeout && (target != "codex" || h.Timeout != 0) {
		return fmt.Sprintf("runs %q with timeout %s, not %s", h.Command, got, h.Timeout)
	}
	if target == "gemini" && !maps.Equal(n.Env, h.Env) {
		return fmt.Sprintf("runs %q with different env %s", h.Command, strings.Join(envKeysDiffering(n.Env, h.Env), ", "))
	}
	return ""
}

// envKeysDiffering names the keys whose value differs or that only one
// side sets. Values stay out of the warning: an env can hold secrets.
func envKeysDiffering(a, b map[string]string) []string {
	var keys []string
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			keys = append(keys, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func shownCommand(h Handler, goos string) string {
	command := h.Command
	if goos == "windows" && h.CommandWindows != "" {
		command = h.CommandWindows
	}
	for _, a := range h.Args {
		command += " " + a
	}
	return command
}

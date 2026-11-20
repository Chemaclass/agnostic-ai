package cli

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
)

func TestServeMux_StopShellRejectsInvalidAndUnlistedTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	h := newServeMux(nil, authority.HandlerOptions{Root: home}, serveMuxOptions{})
	valid := `{"tool":"claude","id":"` + inProject + `","pid":54321,"procStart":"2026-11-07T12:00:00Z"}`
	for _, tc := range []struct {
		name, body, origin string
		status             int
	}{
		{"unlisted", valid, "", http.StatusConflict},
		{"missing start", `{"tool":"claude","id":"x","pid":42}`, "", http.StatusBadRequest},
		{"negative PID", `{"tool":"claude","id":"x","pid":-42,"procStart":"time"}`, "", http.StatusBadRequest},
		{"extra field", strings.TrimSuffix(valid, "}") + `,"command":"anything"}`, "", http.StatusBadRequest},
		{"foreign origin", valid, "https://foreign.example", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postEdit(t, h, "/api/conversations/stop-shell", tc.body, func(r *http.Request) {
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
			})
			var reply editError
			if err := json.Unmarshal(rec.Body.Bytes(), &reply); rec.Code != tc.status || err != nil || reply.Error == "" {
				t.Errorf("stop shell = %d %s (%v), want %d with a JSON error", rec.Code, rec.Body, err, tc.status)
			}
		})
	}
}

package cli

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
)

func TestServeMux_StopSessionRejectsInvalidAndUnlistedTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	h := newServeMux(nil, authority.HandlerOptions{Root: home}, serveMuxOptions{})
	valid := `{"tool":"claude","id":"` + inProject + `"}`
	for _, tc := range []struct {
		name, body, origin string
		status             int
	}{
		{"not running", valid, "", http.StatusConflict},
		{"missing id", `{"tool":"claude"}`, "", http.StatusBadRequest},
		{"extra field", `{"tool":"claude","id":"` + inProject + `","pid":42}`, "", http.StatusBadRequest},
		{"foreign origin", valid, "https://foreign.example", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postEdit(t, h, "/api/conversations/stop-session", tc.body, func(r *http.Request) {
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
			})
			var reply editError
			if err := json.Unmarshal(rec.Body.Bytes(), &reply); rec.Code != tc.status || err != nil || reply.Error == "" {
				t.Errorf("stop session = %d %s (%v), want %d with a JSON error", rec.Code, rec.Body, err, tc.status)
			}
		})
	}
}

package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func TestServeMux_StopSessionRejectsInvalidAndUnlistedTargets(t *testing.T) {
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
		{"not running", valid, "", http.StatusConflict},
		{"missing id", `{"tool":"claude","pid":54321,"procStart":"time"}`, "", http.StatusBadRequest},
		{"missing process", `{"tool":"claude","id":"` + inProject + `"}`, "", http.StatusBadRequest},
		{"extra field", strings.TrimSuffix(valid, "}") + `,"command":"anything"}`, "", http.StatusBadRequest},
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

type unreadableStopProcesses struct{ stillProcesses }

func (unreadableStopProcesses) AliveAmong([]conversations.ProcessRef) (map[int]bool, error) {
	return nil, errors.New("failed to read processes")
}

func TestStopSessionRoute_ReportsProcessReadFailure(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("requires support for stopping sessions")
	}
	h, _, _ := conversationsFixture(t, true)
	h.scanner.Processes = unreadableStopProcesses{}
	rec := postEdit(t, h, "/api/conversations/stop-session", `{"tool":"claude","id":"`+inProject+`","pid":4242,"procStart":"Sat Nov  7 11:00:00 2026"}`, nil)
	var reply editError
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); rec.Code != http.StatusInternalServerError || err != nil || !strings.Contains(reply.Error, "failed to read processes") {
		t.Errorf("stop session = %d %s (%v), want 500 with the read error", rec.Code, rec.Body, err)
	}
}

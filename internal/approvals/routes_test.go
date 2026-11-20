package approvals_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestRoutes_OneRowPerTarget(t *testing.T) {
	var targets []string
	for _, r := range approvals.Routes() {
		if slices.Contains(targets, r.Target) {
			t.Errorf("%s has more than one row", r.Target)
		}
		targets = append(targets, r.Target)
	}
	if want := adapters.Names(); !slices.Equal(targets, want) {
		t.Errorf("route targets = %v, want every adapter %v", targets, want)
	}
}

func TestRoutes_RowsNameMechanismOrReason(t *testing.T) {
	for _, r := range approvals.Routes() {
		for mode, m := range map[string]*approvals.Mechanism{"yolo": r.Yolo, "review": r.Review} {
			if m == nil {
				if r.Reason == "" {
					t.Errorf("%s %s: no route and no fallback reason", r.Target, mode)
				}
				continue
			}
			if m.Name == "" || !strings.HasPrefix(m.File, "~/") || !strings.HasPrefix(m.Evidence, "https://") {
				t.Errorf("%s %s: route %+v needs a mechanism, a user-level file and an evidence URL", r.Target, mode, *m)
			}
		}
	}
}

func TestRoutes_ClaudeAndCodexHaveTheHook(t *testing.T) {
	for _, target := range []string{"claude", "codex"} {
		r := approvals.RouteFor(target)
		if m := r.Mechanism("yolo", "darwin"); m == nil || m.Name != approvals.MechanismPermissionHook {
			t.Errorf("%s yolo = %+v, want the permission hook", target, m)
		}
		if r.Mechanism("review", "linux") != nil {
			t.Errorf("%s review has a route before its slice lands", target)
		}
	}
}

func TestRoutes_WindowsFallsBack(t *testing.T) {
	for _, r := range approvals.Routes() {
		for _, mode := range []string{"yolo", "review"} {
			if r.Mechanism(mode, "windows") != nil || r.FallbackReason(mode, "windows") != approvals.WindowsReason {
				t.Errorf("%s %s on Windows = %q, want %q", r.Target, mode, r.FallbackReason(mode, "windows"), approvals.WindowsReason)
			}
		}
	}
}

func TestRoutes_AskHasNoReason(t *testing.T) {
	if got := approvals.RouteFor("copilot").FallbackReason("ask", "linux"); got != "" {
		t.Errorf("ask fallback reason = %q, want none", got)
	}
	if got := approvals.RouteFor("nope"); got.Mechanism("yolo", "linux") != nil || got.Reason == "" {
		t.Errorf("unknown target route = %+v, want a fallback", got)
	}
}

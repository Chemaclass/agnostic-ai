package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// doctor config said "config valid" for configs sync rejects, which sent
// users to the wrong place (#2660).
func TestDoctorConfig_FailsWithTheErrorSyncPrints(t *testing.T) {
	for name, c := range map[string]struct{ config, names string }{
		"unknown builtin":         {"version: 1\ntargets: [claude]\nbuiltins: [handof]\n", "handof"},
		"unmet requires":          {"version: 1\ntargets: [claude]\nrequires: \">=99.0.0\"\n", ">=99.0.0"},
		"bad agents output":       {"version: 1\ntargets: [claude]\noutputs:\n  claude:\n    agents: skill\n", "outputs.claude.agents"},
		"unknown tier target":     {"version: 1\ntargets: [claude]\nmodels:\n  fast:\n    claudee: haiku\n", "claudee"},
		"unknown coverage target": {"version: 1\ntargets: [claude]\ncoverage:\n  accept:\n    - {target: claudee, kind: rules, field: model, reason: x}\n", "coverage.accept"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(c.config), 0o644); err != nil {
				t.Fatal(err)
			}
			silence(t)

			// A release build, since requires is not checked on a dev build.
			_, _, syncErr := runAsVersion(t, "v0.89.0", "sync")
			out, _, doctorErr := runAsVersion(t, "v0.89.0", "doctor", "config")

			if errs.CodeOf(syncErr) == "" || !strings.Contains(syncErr.Error(), c.names) {
				t.Fatalf("sync must reject this config with a coded error naming %q, got %v", c.names, syncErr)
			}
			if doctorErr == nil {
				t.Fatalf("doctor config passed a config sync rejects:\n%s", out)
			}
			if doctorErr.Error() != syncErr.Error() || errs.CodeOf(doctorErr) != errs.CodeOf(syncErr) {
				t.Errorf("doctor config must fail as sync does\n doctor: [%s] %v\n   sync: [%s] %v",
					errs.CodeOf(doctorErr), doctorErr, errs.CodeOf(syncErr), syncErr)
			}
		})
	}
}

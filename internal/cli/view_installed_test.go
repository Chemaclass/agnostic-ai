package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestInstalledState_NeverRunsAnExternalAdapterFromPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	marker := filepath.Join(dir, "executed")
	executable := filepath.Join(dir, "agnostic-ai-adapter-foreign")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n: > \"$AUTHORITY_ADAPTER_MARKER\"\nprintf '%s' '{\"protocol_version\":1,\"files\":[]}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTHORITY_ADAPTER_MARKER", marker)
	state, err := installedState("foreign", &config.Config{Targets: []string{"foreign"}}, spec.Bundle{}, dir)
	if err == nil || state.State == authority.InstalledInSync {
		t.Errorf("external adapter was considered for comparison: %+v, %v", state, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("external adapter executed: marker state %v", err)
	}
}

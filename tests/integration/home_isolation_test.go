package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain points HOME at a temp dir and unsets CODEX_HOME, so no test
// reads the user's own tool config, such as ~/.codex/config.toml. The Go
// caches and `go env -w` settings stay where they were: they can default
// under HOME, and the tests that build the binary would start cold or
// lose a private proxy.
func TestMain(m *testing.M) {
	code, err := runIsolated(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func runIsolated(m *testing.M) (int, error) {
	if err := pinGoEnv("GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"); err != nil {
		return 1, err
	}
	home, err := os.MkdirTemp("", "agnostic-integration-tests-")
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(home)
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, home); err != nil {
			return 1, err
		}
	}
	if err := os.Unsetenv("CODEX_HOME"); err != nil {
		return 1, err
	}
	return m.Run(), nil
}

func pinGoEnv(keys ...string) error {
	out, err := exec.Command("go", append([]string{"env"}, keys...)...).Output()
	if err != nil {
		return fmt.Errorf("go env: %w", err)
	}
	values := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(values) != len(keys) {
		return fmt.Errorf("go env: got %d values for %d keys", len(values), len(keys))
	}
	for i, key := range keys {
		if err := os.Setenv(key, strings.TrimRight(values[i], "\r")); err != nil {
			return err
		}
	}
	return nil
}

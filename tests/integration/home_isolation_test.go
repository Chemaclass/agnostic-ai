package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

var isolatedEnvKeys = []string{"AGNOSTIC_AI_HOME", "AGNOSTIC_AI_TARGET", "CODEX_HOME", "QWEN_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_DISABLE_AUTO_MEMORY", "CLAUDE_PROJECT_DIR", "GEMINI_PROJECT_DIR", "CURSOR_PROJECT_DIR", "QODER_PROJECT_DIR", "FACTORY_PROJECT_DIR", "DEVIN_PROJECT_DIR", "TRAE_PROJECT_DIR", "OPENHANDS_PROJECT_DIR", "AUGMENT_PROJECT_DIR"}

// TestMain points HOME at a temp dir and unsets the variables that move
// or turn off a tool's own config, so no test reads the user's own, such
// as ~/.codex/config.toml. The Go caches and `go env -w` settings stay
// where they were: they can default under HOME, and the tests that build
// the binary would start cold or lose a private proxy.
func TestMain(m *testing.M) {
	code, err := runIsolated(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func runIsolated(m *testing.M) (code int, err error) {
	if err := testutil.DisableGitBackgroundWork(); err != nil {
		return 1, err
	}
	if err := pinGoEnv("GOCACHE", "GOMODCACHE", "GOPATH", "GOENV"); err != nil {
		return 1, err
	}
	home, err := os.MkdirTemp("", "agnostic-integration-tests-")
	if err != nil {
		return 1, err
	}
	defer func() {
		if rmErr := os.RemoveAll(home); rmErr != nil && err == nil {
			err = rmErr
		}
	}()
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, home); err != nil {
			return 1, err
		}
	}
	for _, key := range isolatedEnvKeys {
		if err := os.Unsetenv(key); err != nil {
			return 1, err
		}
	}
	return m.Run(), nil
}

func TestMainUnsetsAgnosticAIHome(t *testing.T) {
	probeKeys := []string{"AGNOSTIC_AI_HOME", "AGNOSTIC_AI_TARGET", "GEMINI_PROJECT_DIR", "CURSOR_PROJECT_DIR", "QODER_PROJECT_DIR", "FACTORY_PROJECT_DIR", "DEVIN_PROJECT_DIR", "TRAE_PROJECT_DIR", "CLAUDE_PROJECT_DIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR"}
	if os.Getenv("AGNOSTIC_AI_HOME_PROBE") == "1" {
		for _, key := range probeKeys {
			if value, ok := os.LookupEnv(key); ok {
				t.Errorf("%s=%q reached the tests; TestMain must unset it", key, value)
			}
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainUnsetsAgnosticAIHome$")
	cmd.Env = os.Environ()
	for _, key := range probeKeys {
		cmd.Env = append(cmd.Env, key+"="+t.TempDir())
	}
	cmd.Env = append(cmd.Env, "AGNOSTIC_AI_HOME_PROBE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("TestMain left isolated variables set: %v\n%s", err, out)
	}
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

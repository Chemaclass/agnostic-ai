package cli

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

var isolatedEnvKeys = []string{"AGNOSTIC_AI_HOME", "AGNOSTIC_AI_TARGET", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_DISABLE_AUTO_MEMORY", "CLAUDE_PROJECT_DIR", "GEMINI_PROJECT_DIR", "CURSOR_PROJECT_DIR", "QODER_PROJECT_DIR", "FACTORY_PROJECT_DIR", "DEVIN_PROJECT_DIR", "TRAE_PROJECT_DIR", "OPENHANDS_PROJECT_DIR", "AUGMENT_PROJECT_DIR"}

func TestMain(m *testing.M) {
	if err := testutil.DisableGitBackgroundWork(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	home, err := os.MkdirTemp("", "agnostic-cli-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, home); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	for _, key := range isolatedEnvKeys {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
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

package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

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
	for _, key := range []string{"AGNOSTIC_AI_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_DISABLE_AUTO_MEMORY"} {
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
	if value, ok := os.LookupEnv("AGNOSTIC_AI_HOME"); ok {
		t.Fatalf("AGNOSTIC_AI_HOME=%q reached the tests; TestMain must unset it", value)
	}
}

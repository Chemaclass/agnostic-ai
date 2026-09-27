package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

func TestErrorText_AddsTheRegistryFixToACodedError(t *testing.T) {
	err := errs.Coded(errs.CodeConfigMissing, "read config: no agnostic-ai.yaml in .")

	got := ErrorText(err)

	if !strings.HasPrefix(got, "[AAI-003] read config") || !strings.Contains(got, "\n  fix: Run `agnostic-ai init`") {
		t.Errorf("got %q", got)
	}
}

func TestErrorText_LeavesAPlainErrorAlone(t *testing.T) {
	if got := ErrorText(errors.New("boom")); got != "boom" {
		t.Errorf("got %q", got)
	}
}

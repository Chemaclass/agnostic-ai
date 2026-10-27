package gitquery

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
	os.Exit(m.Run())
}

package conversations

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	shellSystem = "darwin"
	os.Exit(m.Run())
}

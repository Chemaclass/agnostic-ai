package emit

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Under `sync --jobs`, Codex sweeps its legacy `.agents/agents/*.toml`
// while Antigravity writes `.agents/agents/<name>/agent.md` into the same
// tree. The sweep must not prune the directory Antigravity just created;
// on Windows that write then fails with "Access is denied" (#1548).
func TestRemoveGeneratedTreeExt_DoesNotRaceAConcurrentWriter(t *testing.T) {
	testutil.TempCwd(t)
	shared := filepath.Join(".agents", "agents")
	const rounds = 300

	var wg sync.WaitGroup
	errs := make(chan error, 2*rounds)
	wg.Add(2)
	go func() {
		defer wg.Done()
		sweeper := NewSession()
		for i := 0; i < rounds; i++ {
			if err := sweeper.RemoveGeneratedTreeExt(shared, ".toml", false); err != nil {
				errs <- err
			}
		}
	}()
	go func() {
		defer wg.Done()
		writer := NewSession()
		for i := 0; i < rounds; i++ {
			path := filepath.Join(shared, "agent-"+strconv.Itoa(i), "agent.md")
			if err := writer.WriteFile(path, "x\n", false); err != nil {
				errs <- err
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

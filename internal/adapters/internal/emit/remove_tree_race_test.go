package emit

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	path := filepath.Join(shared, "agent-1", "agent.md")
	const rounds = 2000

	var done atomic.Bool
	var wg sync.WaitGroup
	errs := make(chan error, 2*rounds)
	wg.Add(2)
	go func() {
		defer wg.Done()
		sweeper := NewSession()
		for !done.Load() {
			if err := sweeper.RemoveGeneratedTreeExt(shared, ".toml", false); err != nil {
				errs <- err
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer done.Store(true)
		for i := 0; i < rounds; i++ {
			// A fresh session each round, as each sync is, so the write
			// is a create into a directory that may have to be made.
			if err := NewSession().WriteFile(path, "x\n", false); err != nil {
				errs <- err
			}
			_ = os.Remove(path)
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

package cli

import (
	"strings"
	"sync"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// TestSync_ConcurrentSessionsDoNotCrossTalk runs two independent emission
// sessions in parallel goroutines, each rendering a distinct bundle, and
// asserts neither session's captured output leaks into the other, while
// staying byte-identical to a sequential baseline.
//
// Before emit session state was threaded per run (issue #486) capture
// buffers lived in a single package global, so two in-process syncs
// interleaved their captured files: this test fails against that global
// and pins the isolation that makes concurrent library / wasm use safe.
// Run it under `-race` to also catch the shared-buffer data race.
func TestSync_ConcurrentSessionsDoNotCrossTalk(t *testing.T) {
	bundleFor := func(tag string) spec.Bundle {
		return spec.NewBundle([]spec.Entry{{
			Kind: spec.KindAgent,
			Name: tag + "-agent",
			Path: "agents/" + tag + "-agent.md",
			Body: tag + " agent body",
		}})
	}
	// capture renders one bundle through the real adapter dispatch under a
	// fresh session in capture mode, so no disk IO occurs and the two
	// goroutines never contend on the shared cwd.
	capture := func(tag string) []adapters.CapturedFile {
		sess := adapters.NewSession()
		sess.StartCapture()
		adapter, err := adapters.Resolve("claude")
		if err != nil {
			t.Errorf("resolve claude: %v", err)
			return nil
		}
		if err := adapters.EmitWithProvenance(sess, adapter, bundleFor(tag), &config.Config{}, false); err != nil {
			t.Errorf("emit %s: %v", tag, err)
		}
		return sess.StopCapture()
	}

	// Sequential baseline for the byte-parity comparison.
	wantAlpha := capture("alpha")
	if len(wantAlpha) == 0 {
		t.Fatal("baseline produced no captured files")
	}

	var gotAlpha, gotBeta []adapters.CapturedFile
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); gotAlpha = capture("alpha") }()
	go func() { defer wg.Done(); gotBeta = capture("beta") }()
	wg.Wait()

	assertOnlyTag(t, "alpha", "beta", gotAlpha)
	assertOnlyTag(t, "beta", "alpha", gotBeta)

	if !sameCaptured(wantAlpha, gotAlpha) {
		t.Errorf("concurrent alpha output diverged from the sequential baseline:\nwant %+v\ngot  %+v", wantAlpha, gotAlpha)
	}
}

// assertOnlyTag fails when any captured file mentions the foreign tag in
// its path or content, or when none mention the expected tag — the two
// signatures of a leaked buffer from the other session.
func assertOnlyTag(t *testing.T, want, foreign string, files []adapters.CapturedFile) {
	t.Helper()
	if len(files) == 0 {
		t.Errorf("%s session captured no files", want)
		return
	}
	for _, f := range files {
		blob := f.Path + "\n" + f.Content
		if strings.Contains(blob, foreign) {
			t.Errorf("%s session leaked %q output: %s", want, foreign, f.Path)
		}
		if !strings.Contains(blob, want) {
			t.Errorf("%s session captured an unexpected file: %s", want, f.Path)
		}
	}
}

// sameCaptured reports whether two capture results hold the same paths
// with byte-identical content.
func sameCaptured(a, b []adapters.CapturedFile) bool {
	if len(a) != len(b) {
		return false
	}
	byPath := make(map[string]string, len(a))
	for _, f := range a {
		byPath[f.Path] = f.Content
	}
	for _, f := range b {
		if content, ok := byPath[f.Path]; !ok || content != f.Content {
			return false
		}
	}
	return true
}

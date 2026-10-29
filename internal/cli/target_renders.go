package cli

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// targetRenders keeps each target's capture render of one bundle for one
// run, so collision detection, shared skills, the gitignore planners, and
// the drift check share it. Every config passed in must be the run's
// config or a copy with more targets: a render is keyed by the target
// and the config's target list, the only part that differs between them.
type targetRenders struct {
	b       spec.Bundle
	sess    *adapters.Session
	renders map[string]targetRender
}

// targetRender is one target's captured files and removals, or the error
// its emit returned.
type targetRender struct {
	files    []adapters.CapturedFile
	removals []adapters.CapturedRemoval
	err      error
	// quiet marks a render made while notes were set aside, which left
	// its capability warnings and coverage notes out of the buffers.
	quiet bool
}

func newTargetRenders(b spec.Bundle) *targetRenders {
	return &targetRenders{b: b, sess: adapters.NewSession(), renders: map[string]targetRender{}}
}

// capture returns the adapter's render under cfg and leaves its notes in
// the buffers. Callers read the files and never change them.
func (r *targetRenders) capture(adapter adapters.Adapter, cfg *config.Config) targetRender {
	return r.render(adapter, cfg, false)
}

// captureQuiet is capture for a caller that set the notes aside, so a
// render it makes may stay out of the buffers.
func (r *targetRenders) captureQuiet(adapter adapters.Adapter, cfg *config.Config) targetRender {
	return r.render(adapter, cfg, true)
}

func (r *targetRenders) render(adapter adapters.Adapter, cfg *config.Config, quiet bool) targetRender {
	key := adapter.Name() + "\x00" + strings.Join(cfg.Targets, "\x00")
	if got, ok := r.renders[key]; ok && (quiet || !got.quiet) {
		return got
	}
	r.sess.StartCapture()
	err := emitAdapter(r.sess, adapter, r.b, cfg, false)
	got := targetRender{files: r.sess.StopCapture(), removals: slices.Clone(r.sess.CapturedRemovals()), err: err, quiet: quiet}
	if err != nil {
		got.files = nil
	}
	r.renders[key] = got
	return got
}

package cli

import (
	"reflect"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// targetRenders keeps each target's capture render of one bundle under
// one config for one run, so collision detection, shared skills, the
// gitignore planners, and the drift check share it. A render is keyed by
// the target and the config's target list. A config that differs from
// the run's in anything but its targets renders on its own, uncached.
type targetRenders struct {
	base    config.Config
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

func newTargetRenders(cfg *config.Config, b spec.Bundle) *targetRenders {
	base := *cfg
	base.Targets = nil
	return &targetRenders{base: base, b: b, sess: adapters.NewSession(), renders: map[string]targetRender{}}
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
	if !r.runConfig(cfg) {
		return captureRender(adapters.NewSession(), adapter, r.b, cfg, quiet)
	}
	key := adapter.Name() + "\x00" + strings.Join(cfg.Targets, "\x00")
	if got, ok := r.renders[key]; ok && (quiet || !got.quiet) {
		return got
	}
	got := captureRender(r.sess, adapter, r.b, cfg, quiet)
	r.renders[key] = got
	return got
}

// runConfig reports whether cfg matches the run's config apart from its
// targets.
func (r *targetRenders) runConfig(cfg *config.Config) bool {
	other := *cfg
	other.Targets = nil
	return reflect.DeepEqual(other, r.base)
}

func captureRender(sess *adapters.Session, adapter adapters.Adapter, b spec.Bundle, cfg *config.Config, quiet bool) targetRender {
	sess.StartCapture()
	err := emitAdapter(sess, adapter, b, cfg, false)
	got := targetRender{files: sess.StopCapture(), removals: slices.Clone(sess.CapturedRemovals()), err: err, quiet: quiet}
	if err != nil {
		got.files = nil
	}
	return got
}

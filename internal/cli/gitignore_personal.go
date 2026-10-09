package cli

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// checkoutRender is what targets write with personal memory in the
// checkout, rendered before sync writes anything: an adapter that reads
// a file on disk then sees the one the last sync left, as a sync without
// repo mode does.
type checkoutRender struct {
	paths map[string]bool
	// failed names the targets that did not render. Their files stay in
	// the managed block.
	failed map[string]bool
}

// renderWithCheckoutMemory renders targets with the checkout store when
// cfg keeps personal memory in the repo store, and returns nil otherwise.
func renderWithCheckoutMemory(cfg *config.Config, b spec.Bundle, targets []string) *checkoutRender {
	if !cfg.RepoPersonalMemory() {
		return nil
	}
	checkout := withCheckoutMemory(cfg.WithAdditionalTargets(targets...))
	defer adapters.SetAsideNotes()()
	sess := adapters.NewSession()
	render := &checkoutRender{paths: map[string]bool{}, failed: map[string]bool{}}
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			render.failed[t] = true
			continue
		}
		files, err := captureAdapterFiles(sess, adapter, b, checkout)
		if err != nil {
			render.failed[t] = true
			continue
		}
		for _, f := range files {
			render.paths[normalizeGitignorePath(f.Path)] = true
		}
	}
	return render
}

// personalOutputs returns the outputs that exist only because personal
// memory lives in the repo store: a file a target that named the store
// wrote and no target writes with the checkout store. That mode comes
// from agnostic-ai.local.yaml, which stays out of Git, so these files go
// to the repository's info/exclude instead of the managed .gitignore
// block and .worktreeinclude, which a team commits. A path an earlier
// sync found stays personal while this sync does not write it, as when it
// leaves that file's target out.
func personalOutputs(checkout *checkoutRender, emits []targetEmit, sessions []*adapters.Session, prior []string, written map[string]string) ([]string, error) {
	var out []string
	for _, p := range prior {
		if _, ok := written[p]; !ok {
			out = append(out, p)
		}
	}
	if checkout != nil {
		for i, e := range emits {
			if e.err != nil || checkout.failed[e.target] || i >= len(sessions) || sessions[i] == nil || !sessions[i].NamedRepoStore() {
				continue
			}
			for _, p := range e.recorded {
				if !checkout.paths[normalizeGitignorePath(p)] {
					out = append(out, p)
				}
			}
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	for _, p := range out {
		if err := adapters.ExcludeOutputFromGit(p); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// withCheckoutMemory returns a copy of cfg that keeps personal memory in
// the checkout, as a project without agnostic-ai.local.yaml does.
func withCheckoutMemory(cfg *config.Config) *config.Config {
	checkout := *cfg
	checkout.Memory.Personal = ""
	return &checkout
}

// keepLedgered returns the paths of personal the ledger still holds.
func keepLedgered(personal, ledger []string) []string {
	var out []string
	for _, p := range personal {
		if slices.Contains(ledger, p) {
			out = append(out, p)
		}
	}
	return out
}

// withoutPersonal returns entries without the paths in personal.
func withoutPersonal(entries, personal []string) []string {
	if len(personal) == 0 {
		return entries
	}
	drop := make(map[string]struct{}, len(personal))
	for _, p := range personal {
		drop[normalizeGitignorePath(p)] = struct{}{}
	}
	return slices.DeleteFunc(slices.Clone(entries), func(e string) bool {
		_, ok := drop[normalizeGitignorePath(e)]
		return ok
	})
}

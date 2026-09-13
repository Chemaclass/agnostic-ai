package goose

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultReviewFile = ".agents/REVIEW.md"

// Goose reads root and ancestor review files as separate checks. Keep
// each scope's body in its own file so a changed path selects both.
func emitReviews(sess *emit.Session, reviews []spec.Entry, cfg *config.Config, dryRun bool) error {
	byScope := map[string][]string{}
	var scopes []string
	for _, review := range reviews {
		scope := filepath.Clean(review.EffectiveScope())
		if emit.ScopeEscapesRoot(scope) {
			continue
		}
		if _, ok := byScope[scope]; !ok {
			scopes = append(scopes, scope)
		}
		byScope[scope] = append(byScope[scope], strings.TrimRight(review.Body, "\n"))
	}
	file := emit.OutputReviewFile(cfg, target, defaultReviewFile)
	for _, scope := range scopes {
		body := strings.Join(byScope[scope], "\n\n") + "\n"
		if err := sess.WriteFile(filepath.Join(scope, file), emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}

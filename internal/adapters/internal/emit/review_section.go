package emit

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Codex code review reads a `## Code Review Rules` section from the root
// AGENTS.md and the one nearest each changed file
// (learn.chatgpt.com/docs/third-party/github). The markers let import
// read the section back and strip it from the AGNOSTIC_AI.md mirror.
const (
	ReviewsStartMarker = "<!-- agnostic-ai:reviews:start -->"
	ReviewsEndMarker   = "<!-- agnostic-ai:reviews:end -->"
	ReviewsHeading     = "Code Review Rules"
)

// reviewSectionTarget is the target whose review specs fill the section.
// Every AGENTS.md reader writes the same bytes, so the section follows
// that target's view of the specs, not the writer's.
const reviewSectionTarget = "codex"

// RenderReviewSection renders the marked section for reviews sharing one
// scope, bodies concatenated like a BUGBOT.md. "" when reviews is empty.
func RenderReviewSection(reviews []spec.Entry) string {
	if len(reviews) == 0 {
		return ""
	}
	bodies := make([]string, 0, len(reviews))
	for _, r := range reviews {
		bodies = append(bodies, strings.Trim(r.Body, "\n"))
	}
	return ReviewsStartMarker + "\n\n## " + ReviewsHeading + "\n\n" + strings.Join(bodies, "\n\n") + "\n\n" + ReviewsEndMarker + "\n"
}

// AppendReviewSection returns body with section after one blank line,
// replacing a section already there.
func AppendReviewSection(body, section string) string {
	if section == "" {
		return body
	}
	body = StripReviewSection(body)
	return strings.TrimRight(body, "\n") + "\n\n" + section
}

// StripReviewSection removes the marked review section from body.
func StripReviewSection(body string) string {
	return stripMarkedBlock(body, ReviewsStartMarker, ReviewsEndMarker)
}

// ReviewSections returns the review section per scope, keyed by the
// scope ("" for the root), when the project syncs codex. requested adds
// the targets a partial run names.
func ReviewSections(b spec.Bundle, cfg *config.Config, requested ...string) map[string]string {
	if cfg == nil || !slices.Contains(cfg.Targets, reviewSectionTarget) && !slices.Contains(requested, reviewSectionTarget) {
		return nil
	}
	byScope := map[string][]spec.Entry{}
	for _, r := range b.For(reviewSectionTarget).Reviews {
		scope := filepath.ToSlash(r.EffectiveScope())
		if ScopeEscapesRoot(scope) {
			continue
		}
		byScope[scope] = append(byScope[scope], r)
	}
	if len(byScope) == 0 {
		return nil
	}
	out := make(map[string]string, len(byScope))
	for scope, reviews := range byScope {
		out[scope] = RenderReviewSection(reviews)
	}
	return out
}

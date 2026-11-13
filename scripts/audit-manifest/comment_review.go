package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type commentReview struct {
	ID       string   `json:"id"`
	Baseline string   `json:"baseline"`
	Path     string   `json:"path"`
	Targets  []string `json:"targets"`
	Before   artifact `json:"before"`
	After    artifact `json:"after"`
	Outcome  *outcome `json:"outcome,omitempty"`
}

func (a app) commentReviews(m *manifest, capture bool) ([]commentReview, error) {
	args := []string{"scripts/target-facts.sh", "--review-reasons", filepath.Join(m.Directory, "docfetch.tsv")}
	if m.BuiltinsSince != "" {
		args = append(args, "--builtins-since", m.BuiltinsSince)
	}
	rows, err := a.command("bash", args...)
	if err != nil {
		return nil, err
	}
	byID := map[string]*commentReview{}
	for _, row := range strings.Split(rows, "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) != 6 || fields[1] != "comment-only" {
			continue
		}
		target, baseline, path := fields[0], fields[2], fields[3]
		if !slices.Contains(m.Targets, target) {
			return nil, fmt.Errorf("%s: unexpected comment-review target", target)
		}
		id := digest([]byte(baseline + "\x00" + path))
		if existing := byID[id]; existing != nil {
			if !slices.Contains(existing.Targets, target) {
				existing.Targets = append(existing.Targets, target)
			}
			continue
		}
		before, after, err := a.commentSource(baseline, path)
		if err != nil {
			return nil, err
		}
		beforePath := filepath.ToSlash(filepath.Join("repository-comments", id+".before.go"))
		afterPath := filepath.ToSlash(filepath.Join("repository-comments", id+".after.go"))
		if capture {
			if err := os.MkdirAll(filepath.Join(m.Directory, "repository-comments"), 0700); err != nil {
				return nil, fmt.Errorf("repository comments: %w", err)
			}
			for name, body := range map[string][]byte{beforePath: before, afterPath: after} {
				if err := atomicWrite(filepath.Join(m.Directory, name), body); err != nil {
					return nil, err
				}
			}
		}
		byID[id] = &commentReview{ID: id, Baseline: baseline, Path: path, Targets: []string{target}, Before: artifact{Path: beforePath, SHA256: digest(before)}, After: artifact{Path: afterPath, SHA256: digest(after)}}
	}
	var result []commentReview
	for _, check := range byID {
		sort.Strings(check.Targets)
		result = append(result, *check)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (a app) validateCommentEvidence(m *manifest) error {
	expected, err := a.commentReviews(m, false)
	if err != nil {
		return err
	}
	if len(expected) != len(m.CommentReviews) {
		return errors.New("shared comment-review coverage changed")
	}
	for i, want := range expected {
		got := m.CommentReviews[i]
		if got.ID != want.ID || got.Path != want.Path || got.Baseline != want.Baseline || !slices.Equal(got.Targets, want.Targets) || got.Before != want.Before || got.After != want.After {
			return fmt.Errorf("%s: shared comment-review inputs changed", want.Path)
		}
		for _, entry := range []artifact{got.Before, got.After} {
			if err := checkArtifact(m.Directory, entry); err != nil {
				return err
			}
		}
		if got.Outcome != nil {
			if got.Outcome.Disposition != "reviewed" || strings.TrimSpace(got.Outcome.Reason) == "" {
				return fmt.Errorf("%s: comment review requires reviewed evidence", got.Path)
			}
			if err := checkArtifact(m.Directory, got.Outcome.Artifact); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireCommentReviews(m *manifest) error {
	for _, check := range m.CommentReviews {
		if check.Outcome == nil || check.Outcome.Disposition != "reviewed" || strings.TrimSpace(check.Outcome.Reason) == "" {
			return fmt.Errorf("%s: pending shared comment review %s", check.Path, check.ID)
		}
		if err := checkArtifact(m.Directory, check.Outcome.Artifact); err != nil {
			return err
		}
	}
	return nil
}

func (a app) reviewComment(m *manifest, args []string) error {
	if len(args) != 3 {
		return errors.New("review-comment requires run, check ID, evidence artifact and reason")
	}
	if err := a.validateEvidence(m, m.LockBaseline); err != nil {
		return err
	}
	out, err := readOutcome(m, append([]string{"reviewed"}, args[1:]...))
	if err != nil {
		return err
	}
	for i := range m.CommentReviews {
		check := &m.CommentReviews[i]
		if check.ID != args[0] {
			continue
		}
		if check.Outcome != nil {
			if *check.Outcome == *out {
				return nil
			}
			return fmt.Errorf("%s: comment review already recorded", check.Path)
		}
		check.Outcome = out
		return save(m)
	}
	return fmt.Errorf("%s: unknown shared comment review", args[0])
}

func commentReviewReport(m *manifest) string {
	var out strings.Builder
	out.WriteString("id\tpath\tstatus\tbefore\tafter\n")
	for _, check := range m.CommentReviews {
		state := "pending"
		if check.Outcome != nil {
			state = check.Outcome.Disposition
		}
		fmt.Fprintf(&out, "%s\t%s\t%s\t%s\t%s\n", check.ID, check.Path, state, check.Before.Path, check.After.Path)
	}
	return out.String()
}

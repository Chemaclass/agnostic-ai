package cli

import (
	"fmt"
	"slices"
	"strings"
)

// Shorter repeats are headings and list labels, not instructions worth merging.
const minDuplicatePassageWords = 8

type contextPassageSource struct {
	Source   string `json:"source"`
	Category string `json:"category"`
}

type contextOpportunity struct {
	Kind       string                 `json:"kind"`
	Target     string                 `json:"target"`
	Load       string                 `json:"load"`
	Passage    string                 `json:"passage"`
	Copies     int                    `json:"copies"`
	Size       contextSize            `json:"size"`
	Reduction  contextSize            `json:"reduction"`
	Sources    []contextPassageSource `json:"sources"`
	Suggestion string                 `json:"suggestion"`
}

// duplicatedStartupPassages finds paragraphs written more than once in the
// specs that load at startup, byte for byte after trimming. Similar wording
// is not a match: only the author can tell whether two instructions mean the
// same. A source rendered into two generated files counts once, since
// removing a copy from the specs cannot fix that.
func duplicatedStartupPassages(target string, contributions []contextContribution) []contextOpportunity {
	type occurrence struct {
		source contextPassageSource
		copies int
	}
	found := map[string][]occurrence{}
	var order []string
	for _, c := range contributions {
		if c.Load != "startup" {
			continue
		}
		inThis := map[string]int{}
		for _, passage := range contextParagraphs(c.text) {
			if wordsIn(passage) >= minDuplicatePassageWords {
				inThis[passage]++
			}
		}
		source := contextPassageSource{Source: c.Source, Category: c.Category}
		for _, passage := range contextParagraphs(c.text) {
			copies, ok := inThis[passage]
			if !ok {
				continue
			}
			delete(inThis, passage)
			if _, seen := found[passage]; !seen {
				order = append(order, passage)
			}
			occurrences := found[passage]
			at := slices.IndexFunc(occurrences, func(o occurrence) bool { return o.source.Source == source.Source })
			if at < 0 {
				found[passage] = append(occurrences, occurrence{source: source, copies: copies})
			} else if copies > occurrences[at].copies {
				occurrences[at].copies = copies
			}
		}
	}
	opportunities := []contextOpportunity{}
	for _, passage := range order {
		o := contextOpportunity{Kind: "duplicate-passage", Target: target, Load: "startup", Passage: passage, Size: contextSize{Words: wordsIn(passage), Bytes: len(passage)}}
		for _, occ := range found[passage] {
			o.Copies += occ.copies
			o.Sources = append(o.Sources, occ.source)
		}
		if o.Copies < 2 {
			continue
		}
		o.Reduction = contextSize{Words: o.Size.Words * (o.Copies - 1), Bytes: o.Size.Bytes * (o.Copies - 1)}
		o.Suggestion = fmt.Sprintf("This passage is written %d times in specs that load at startup for %s. Review the copies and remove one if they say the same thing.", o.Copies, target)
		opportunities = append(opportunities, o)
	}
	slices.SortStableFunc(opportunities, func(a, b contextOpportunity) int {
		if a.Reduction.Words != b.Reduction.Words {
			return b.Reduction.Words - a.Reduction.Words
		}
		return b.Reduction.Bytes - a.Reduction.Bytes
	})
	return opportunities
}

func contextParagraphs(text string) []string {
	var paragraphs []string
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		lines := strings.Split(block, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		if p := strings.TrimSpace(strings.Join(lines, "\n")); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	return paragraphs
}

func opportunitySources(o contextOpportunity) string {
	paths := make([]string, len(o.Sources))
	for i, s := range o.Sources {
		paths[i] = s.Source
	}
	return strings.Join(paths, ", ")
}

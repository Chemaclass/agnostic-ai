package adapters

import "strings"

// SuggestName returns the name in names closest to a mistyped input, or ""
// when nothing is close enough to be a likely typo. An exact match returns
// "" too: there is nothing to suggest.
func SuggestName(input string, names []string) string {
	in := strings.ToLower(input)
	if in == "" {
		return ""
	}
	// Two edits on a short name reach unrelated targets ("amp" to "zed").
	limit := 1
	if len(in) >= 6 {
		limit = 2
	}
	best, bestDist := "", limit+1
	for _, n := range names {
		if n == input {
			return ""
		}
		if d := editDistance(in, strings.ToLower(n)); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

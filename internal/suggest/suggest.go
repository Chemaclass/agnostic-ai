package suggest

import "strings"

// maxInput bounds the input Name compares. Every name is far shorter, so
// a longer input is never two edits from one, and the bound keeps a
// value read from a spec file from sizing the distance table.
const maxInput = 256

// Name returns the name in names closest to a mistyped input, or ""
// when nothing is close enough to be a likely typo, when two names are
// equally close, or when input already is one of the names.
func Name(input string, names []string) string {
	in := strings.ToLower(input)
	if in == "" || len(in) > maxInput {
		return ""
	}
	// Two edits on a short name reach unrelated targets ("amp" to "zed").
	limit := 1
	if len(in) >= 6 {
		limit = 2
	}
	best, bestDist, tied := "", limit+1, false
	for _, n := range names {
		if n == input {
			return ""
		}
		switch d := editDistance(in, strings.ToLower(n)); {
		case d < bestDist:
			best, bestDist, tied = n, d, false
		case d == bestDist && best != "":
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}

// editDistance is the optimal string alignment distance between a and b:
// Levenshtein plus a swap of two adjacent letters as one edit, the most
// common typo.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

package emit

import (
	"fmt"
	"os"
	"strings"
)

func (s *Session) resolvePriorOutput(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupSums[path] != "" || s.keepSums[path] != "" || len(s.backupSums)+len(s.keepSums) == 0 {
		return nil
	}
	if _, resolved := s.priorPaths[path]; resolved {
		return nil
	}
	if s.priorCandidates == nil {
		seen := make(map[string]bool)
		for _, sums := range []map[string]string{s.backupSums, s.keepSums} {
			for prior, sum := range sums {
				if sum != "" && !seen[prior] {
					seen[prior] = true
					s.priorCandidates = append(s.priorCandidates, prior)
				}
			}
		}
	}
	prior, err := s.priorAliases.Match(path, nil, s.priorCandidates)
	if err != nil {
		return fmt.Errorf("resolve prior output: %w", err)
	}
	if s.priorPaths == nil {
		s.priorPaths = make(map[string]string)
	}
	s.priorPaths[path] = prior
	return nil
}

func (s *Session) priorChecksum(path string, sums map[string]string) string {
	if sum := sums[path]; sum != "" {
		return sum
	}
	return sums[s.priorPaths[path]]
}

// matchesAliasSum reports whether existing holds the bytes the last sync
// recorded under another spelling of path that names the same file, as
// `agents.md` and `AGENTS.md` do where letter case does not count.
func matchesAliasSum(path string, existing []byte, sums map[string]string) bool {
	for prior, sum := range sums {
		if prior == path || !strings.EqualFold(prior, path) || !MatchesSum(existing, sum) {
			continue
		}
		a, err := os.Stat(prior)
		if err != nil {
			continue
		}
		if b, err := os.Stat(path); err == nil && os.SameFile(a, b) {
			return true
		}
	}
	return false
}

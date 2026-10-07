package config

import (
	"path"
	"path/filepath"
	"strings"
)

func (g Gitignore) AllowsPath(p string) bool {
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(p), "./"), "/")
	for _, pattern := range g.Allow {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		pattern = strings.TrimPrefix(pattern, "./")
		pattern = strings.TrimPrefix(pattern, "!")
		anchored := strings.HasPrefix(pattern, "/")
		directory := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/")
		if pattern == "" {
			continue
		}
		// Unknown glob syntax must not expose a personal path in a committable file.
		if strings.Contains(pattern, "[:") {
			return true
		}
		pattern = normalizeAllowClasses(pattern)
		if _, err := path.Match(pattern, ""); err != nil {
			return true
		}
		limit := len(parts)
		if directory {
			limit--
		}
		for end := 1; end <= limit; end++ {
			if !anchored && !strings.Contains(pattern, "/") {
				if matched, _ := path.Match(pattern, parts[end-1]); matched {
					return true
				}
			} else if matchAllowSegments(strings.Split(pattern, "/"), parts[:end]) {
				return true
			}
		}
	}
	return false
}

func normalizeAllowClasses(pattern string) string {
	chars := []byte(pattern)
	classStart := -1
	for i := 0; i < len(chars); i++ {
		switch chars[i] {
		case '\\':
			i++
		case '[':
			if classStart < 0 {
				classStart = i
			}
		case '!':
			if classStart >= 0 && i == classStart+1 {
				chars[i] = '^'
			}
		case ']':
			classStart = -1
		}
	}
	return string(chars)
}

func matchAllowSegments(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}
	if pattern[0] == "**" {
		for start := 0; start <= len(parts); start++ {
			if matchAllowSegments(pattern[1:], parts[start:]) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return false
	}
	matched, _ := path.Match(pattern[0], parts[0])
	return matched && matchAllowSegments(pattern[1:], parts[1:])
}

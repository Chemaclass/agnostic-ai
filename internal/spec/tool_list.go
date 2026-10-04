package spec

import "strings"

func SplitToolList(value string) []string {
	var names []string
	start, depth := 0, 0
	for i, char := range value {
		switch char {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				names = append(names, strings.TrimSpace(value[start:i]))
				start = i + 1
			}
		}
	}
	return append(names, strings.TrimSpace(value[start:]))
}

package approvals

import "strings"

// sedScriptReadable reports a script made only of s, y, d, p, q, =, and
// the hold-space commands, each with an optional line or regexp
// address, and s flags that do not run or write.
func sedScriptReadable(script string) bool {
	runes := []rune(script)
	i := 0
	skipSpace := func() {
		for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
			i++
		}
	}
	section := func(delim rune) bool {
		for ; i < len(runes); i++ {
			if runes[i] == '\\' {
				i++
				continue
			}
			if runes[i] == '\n' {
				return false
			}
			if runes[i] == delim {
				i++
				return true
			}
		}
		return false
	}
	address := func() bool {
		for k := range 2 {
			skipSpace()
			switch {
			case i < len(runes) && runes[i] >= '0' && runes[i] <= '9':
				for i < len(runes) && (runes[i] >= '0' && runes[i] <= '9' || runes[i] == '~') {
					i++
				}
			case i < len(runes) && runes[i] == '$':
				i++
			case i < len(runes) && runes[i] == '/':
				i++
				if !section('/') {
					return false
				}
				for i < len(runes) && (runes[i] == 'I' || runes[i] == 'M') {
					i++
				}
			}
			skipSpace()
			if k == 0 && i < len(runes) && runes[i] == ',' {
				i++
				continue
			}
			break
		}
		skipSpace()
		if i < len(runes) && runes[i] == '!' {
			i++
		}
		return true
	}
	for i < len(runes) {
		skipSpace()
		if i < len(runes) && (runes[i] == ';' || runes[i] == '\n') {
			i++
			continue
		}
		if i >= len(runes) {
			break
		}
		if !address() || i >= len(runes) {
			return false
		}
		cmd := runes[i]
		i++
		switch cmd {
		case 's', 'y':
			if i >= len(runes) {
				return false
			}
			delim := runes[i]
			if delim == '\\' || delim == '\n' || delim == ' ' {
				return false
			}
			i++
			// s and y take two sections: the pattern and its replacement.
			for range 2 {
				if !section(delim) {
					return false
				}
			}
			for i < len(runes) && strings.ContainsRune("gpiIm0123456789", runes[i]) && cmd == 's' {
				i++
			}
		case 'd', 'D', 'p', 'P', 'q', 'Q', '=', 'n', 'N', 'g', 'G', 'h', 'H', 'x', 'l', 'z':
			if cmd == 'q' || cmd == 'Q' {
				skipSpace()
				for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
					i++
				}
			}
		default:
			return false
		}
		skipSpace()
		if i < len(runes) && runes[i] != ';' && runes[i] != '\n' {
			return false
		}
	}
	return true
}

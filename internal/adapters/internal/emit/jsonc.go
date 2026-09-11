package emit

// StripJSONC rewrites JSONC bytes into strict JSON that
// `encoding/json` accepts: `//` line comments, `/* ... */` block
// comments, and a comma trailing the last member of an object or array
// are all replaced with spaces. Bytes inside a string literal are
// copied verbatim, so a value such as "https://example.com" never reads
// as the start of a comment. The input slice is not modified.
//
// hadComments reports whether a comment was found. Comments do not
// survive a merge (the document is re-rendered from parsed values), so
// the caller warns about the ones it is about to drop.
//
// Adapters merge into files several vendors document as JSONC: Kilo's
// `kilo.jsonc` ("JSONC supports // comments"), Qoder's
// `.qoder/settings.json` ("supporting // comments"), and Augment's
// `.augment/settings.json` ("JSON with Comments (JSONC), allowing
// comments and trailing commas"). Without this the merge read failed
// and every user-authored key in those files was overwritten (#725).
//
// Comments become spaces rather than being deleted so the byte offset
// in a json.SyntaxError still points at the same place in the original
// file, which keeps a parse error message usable. Newlines inside a
// block comment are kept for the same reason.
func StripJSONC(data []byte) (stripped []byte, hadComments bool) {
	out, hadComments := blankJSONCComments(data)
	return dropTrailingCommas(out), hadComments
}

// blankJSONCComments returns a copy of data with every comment outside
// a string literal overwritten by spaces (newlines preserved).
func blankJSONCComments(data []byte) ([]byte, bool) {
	out := make([]byte, len(data))
	copy(out, data)
	found := false
	inString := false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			switch c {
			case '\\':
				i++ // skip the escaped byte, so `\"` does not close the string
			case '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
		case c == '/' && i+1 < len(out) && out[i+1] == '/':
			found = true
			for ; i < len(out) && out[i] != '\n'; i++ {
				out[i] = ' '
			}
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			found = true
			out[i], out[i+1] = ' ', ' '
			for i += 2; i < len(out); i++ {
				if out[i] == '*' && i+1 < len(out) && out[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i++
					break
				}
				if out[i] != '\n' {
					out[i] = ' '
				}
			}
		}
	}
	return out, found
}

// dropTrailingCommas blanks each comma that only whitespace separates
// from a closing `}` or `]`. It mutates data in place, so callers pass
// the copy blankJSONCComments already made.
func dropTrailingCommas(data []byte) []byte {
	inString := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c != ',' {
			continue
		}
		for j := i + 1; j < len(data); j++ {
			switch data[j] {
			case ' ', '\t', '\r', '\n':
				continue
			case '}', ']':
				data[i] = ' '
			}
			break
		}
	}
	return data
}

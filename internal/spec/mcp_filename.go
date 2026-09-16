package spec

import "strings"

const upperHex = "0123456789ABCDEF"

// MCPFileName returns one flat YAML filename for an MCP server name. Existing
// single-segment names keep their filenames. Names containing a slash encode
// punctuation too, which also avoids Windows-invalid package-style paths.
func MCPFileName(name string) string {
	encodePackageName := strings.ContainsRune(name, '/')
	var encoded strings.Builder
	encoded.Grow(len(name))
	for _, char := range []byte(name) {
		if char != '%' && (!encodePackageName || isPortableFilenameByte(char)) {
			encoded.WriteByte(char)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(upperHex[char>>4])
		encoded.WriteByte(upperHex[char&0x0f])
	}
	return encoded.String() + ".yaml"
}

func isPortableFilenameByte(char byte) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '-' || char == '_' || char == '.'
}

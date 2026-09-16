package spec

import (
	"fmt"
	"strings"
)

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

// ValidateMCPNames validates logical names and rejects destination filenames
// that alias on case-insensitive filesystems.
func ValidateMCPNames(names []string) error {
	seen := make(map[string]string, len(names))
	for _, name := range names {
		if err := ValidateName(KindMCP, name); err != nil {
			return err
		}
		filename := MCPFileName(name)
		key := strings.ToLower(filename)
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("MCP names %q and %q map to the same filename %q on a case-insensitive filesystem", previous, name, filename)
		}
		seen[key] = name
	}
	return nil
}

func isPortableFilenameByte(char byte) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '-' || char == '_' || char == '.'
}

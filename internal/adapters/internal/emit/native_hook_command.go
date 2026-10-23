package emit

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

const windowsHookPrefix = "powershell -NoProfile -EncodedCommand "

func NativeHookCommand(command, target, platform string) string {
	if platform != "windows" {
		return ExportHookTarget(command, target)
	}
	// EncodedCommand survives both Git Bash and PowerShell argument parsing.
	script := "$env:AGNOSTIC_AI_TARGET='" + target + "'; & { " + command + " }"
	words := utf16.Encode([]rune(script))
	data := make([]byte, 2*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint16(data[i*2:], w)
	}
	return windowsHookPrefix + base64.StdEncoding.EncodeToString(data)
}
func DeclaredNativeHookCommand(command, target string) string {
	if text, ok := strings.CutPrefix(command, windowsHookPrefix); ok {
		data, err := base64.StdEncoding.DecodeString(text)
		if err == nil && len(data)%2 == 0 {
			words := make([]uint16, len(data)/2)
			for i := range words {
				words[i] = binary.LittleEndian.Uint16(data[i*2:])
			}
			script := string(utf16.Decode(words))
			if inner, ok := strings.CutPrefix(script, "$env:AGNOSTIC_AI_TARGET='"+target+"'; & { "); ok {
				if inner, ok := strings.CutSuffix(inner, " }"); ok {
					return inner
				}
			}
		}
	}
	return StripHookTargetExport(command, target)
}

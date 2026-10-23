package vibe

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

const windowsHookPrefix = "powershell -NoProfile -EncodedCommand "

func hookCommand(command, platform string) string {
	if platform != "windows" {
		return emit.ExportHookTarget(command, target)
	}
	// EncodedCommand survives both Git Bash and PowerShell argument parsing.
	script := "$env:AGNOSTIC_AI_TARGET='vibe'; & { " + command + " }"
	words := utf16.Encode([]rune(script))
	data := make([]byte, 2*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint16(data[i*2:], w)
	}
	return windowsHookPrefix + base64.StdEncoding.EncodeToString(data)
}
func DeclaredHookCommand(command string) string {
	if text, ok := strings.CutPrefix(command, windowsHookPrefix); ok {
		data, err := base64.StdEncoding.DecodeString(text)
		if err == nil && len(data)%2 == 0 {
			words := make([]uint16, len(data)/2)
			for i := range words {
				words[i] = binary.LittleEndian.Uint16(data[i*2:])
			}
			script := string(utf16.Decode(words))
			if inner, ok := strings.CutPrefix(script, "$env:AGNOSTIC_AI_TARGET='vibe'; & { "); ok {
				if inner, ok := strings.CutSuffix(inner, " }"); ok {
					return inner
				}
			}
		}
	}
	return emit.StripHookTargetExport(command, target)
}

func nativeHookCommand(command string, args []string, platform string) (string, error) {
	if platform == "windows" && !emit.PowerShellReadsFold(command, args) {
		return "", fmt.Errorf("arguments cannot keep their exact values in Windows PowerShell; put them in a native command instead")
	}
	return hookCommand(emit.ExecFormCommand(command, args), platform), nil
}

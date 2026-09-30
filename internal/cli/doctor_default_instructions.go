package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// reportLegacyDefaultInstructions prints a hint, never a failure, when
// AGNOSTIC_AI.md still opens with the long default text older releases
// seeded. Every session loads that text, but the file belongs to the
// user, so doctor points at it instead of rewriting it.
func reportLegacyDefaultInstructions(cmd *cobra.Command) {
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		return
	}
	body := strings.TrimLeft(header.Strip(string(data)), "\n")
	if !strings.HasPrefix(body, adapters.LegacyEntryPointTemplateLead) {
		return
	}
	cmd.Println()
	cmd.Println("Instructions:")
	cmd.Printf("  i %s still holds the old default text, which every session loads. Replace it with your project instructions; sync keeps what you write.\n",
		adapters.AgnosticEntryPointPath)
}

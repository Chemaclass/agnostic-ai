package cli

import "github.com/chemaclass/agnostic-ai/internal/config"

// Kilo import currently covers only the compatibility ignore file, so
// sync's hand-authored-file refusal has a working recovery command.
func importKiloIgnore(root string, src config.Sources) error {
	ignores, err := importIgnoreFile(root, "kilo", src)
	if err != nil {
		return err
	}
	summaryf("imported %d ignores (other Kilo configuration is not imported)\n", ignores)
	printImportNextSteps(root, "kilo")
	return nil
}

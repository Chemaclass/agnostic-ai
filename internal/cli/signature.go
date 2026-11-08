package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/term"
)

const (
	authorName = "Chemaclass"
	authorURL  = "https://chemaclass.com"
)

func signature(linked bool) string {
	if linked {
		return "Made by \x1b]8;;" + authorURL + "\x1b\\" + authorName + "\x1b]8;;\x1b\\"
	}
	return "Made by " + authorName + " · " + authorURL
}

func linksEnabled(w io.Writer) bool {
	return term.ColorEnabled(w) && os.Getenv("TERM") != "dumb"
}

func signRootHelp(root *cobra.Command) {
	help := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		help(cmd, args)
		if cmd == root {
			out := cmd.OutOrStdout()
			_, _ = io.WriteString(out, "\n"+signature(linksEnabled(out))+"\n")
		}
	})
}

// SignVersion follows a --version run with the signature, on a terminal
// only: upgrade and install checks compare piped --version output to one
// exact line. A version template would do it too, but it links
// text/template into the binary (2.7 MB).
func SignVersion(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	if linksEnabled(out) {
		signVersion(cmd, out)
	}
}

func signVersion(cmd *cobra.Command, out io.Writer) {
	version, _ := cmd.Flags().GetBool("version")
	help, _ := cmd.Flags().GetBool("help")
	if cmd == cmd.Root() && version && !help {
		_, _ = io.WriteString(out, signature(true)+"\n")
	}
}

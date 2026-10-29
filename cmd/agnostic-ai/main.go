package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/cli"
)

var version = "0.90.0"

func main() {
	cmd, err := cli.NewRootCmd(version).ExecuteC()
	if err != nil {
		fmt.Fprintln(os.Stderr, cli.ErrorText(err))
		if recoveryErr := cli.OfferErrorRecovery(cmd, err); recoveryErr != nil {
			fmt.Fprintln(os.Stderr, cli.ErrorText(recoveryErr))
		}
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

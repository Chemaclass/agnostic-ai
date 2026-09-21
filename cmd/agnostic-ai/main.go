package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/cli"
)

var version = "0.64.0"

func main() {
	if err := cli.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

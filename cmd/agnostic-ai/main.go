package main

import (
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/cli"
)

var version = "0.36.0"

func main() {
	if err := cli.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

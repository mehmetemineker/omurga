package main

import (
	"fmt"
	"os"

	"omurga/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if !cli.IsSilentError(err) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(cli.ExitCode(err))
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/nimsforest/mycelium/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.Root(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

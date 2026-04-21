package main

import (
	"os"

	"github.com/kluzzebass/imux/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

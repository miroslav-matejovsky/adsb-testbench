package main

import (
	"os"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/cli"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

func main() {
	os.Exit(cli.Main(testbench.ModeDisplay, os.Args[1:]))
}

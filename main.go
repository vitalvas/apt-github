package main

import (
	"os"

	"github.com/vitalvas/apt-github/internal/app"
)

var version = "dev"

func main() {
	if err := app.NewRootCmd(version).Execute(); err != nil {
		os.Exit(1)
	}
}

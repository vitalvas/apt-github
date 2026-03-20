package main

import (
	"os"

	"github.com/vitalvas/apt-github/internal/app"
)

func main() {
	if err := app.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

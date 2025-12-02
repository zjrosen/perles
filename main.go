// Package main is the entry point for the perles TUI application.
package main

import (
	"fmt"
	"os"

	"perles/cmd"
)

// Build information injected via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	versionString := fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
	cmd.SetVersion(versionString)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

package main

import (
	"os"

	"github.com/dvhthomas/gh-velocity/cmd"
)

// Version and BuildTime are set via ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	os.Exit(cmd.Execute(Version, BuildTime))
}

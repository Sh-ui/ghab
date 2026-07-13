package main

import (
	"flag"
	"fmt"
)

func newFlagSet(showVersion, checkConfig *bool) *flag.FlagSet {
	fs := flag.NewFlagSet("ghab", flag.ContinueOnError)
	fs.BoolVar(showVersion, "version", false, "print ghab's version and exit")
	fs.BoolVar(checkConfig, "check-config", false, "print the resolved config, palette source, and any warnings, then exit")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: ghab [--version] [--check-config] [owner/repo]")
		fs.PrintDefaults()
	}
	return fs
}

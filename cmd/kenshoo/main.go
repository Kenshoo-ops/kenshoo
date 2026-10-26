// Command kenshoo restores a database backup into a throwaway database,
// checks the restored data and reports whether the backup is usable.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Kenshoo-ops/kenshoo/internal/config"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("kenshoo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "kenshoo.yaml", "path to the configuration file")
	showVersion := fs.Bool("version", false, "print the version and exit")
	validateOnly := fs.Bool("validate", false, "validate the configuration and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if *validateOnly {
		fmt.Fprintln(stdout, "configuration is valid")
		return 0
	}

	fmt.Fprintf(stderr, "verification of %q backups is not implemented yet\n", cfg.Source.Type)
	return 3
}

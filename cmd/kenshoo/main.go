// Command kenshoo restores a database backup into a throwaway database,
// checks the restored data and reports whether the backup is usable.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Kenshoo-ops/kenshoo/internal/config"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// storeFactory builds the Store that a configuration points at.
// Production uses the S3-compatible store, tests use an in-memory one.
type storeFactory func(ctx context.Context, s config.Storage) (storage.Store, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the production entry point. Exit codes: 0 success, 1 error, 2 bad usage,
// 3 not implemented yet, 4 the store holds no backup.
func run(args []string, stdout, stderr io.Writer) int {
	return runWith(context.Background(), args, stdout, stderr, newS3Store)
}

func newS3Store(ctx context.Context, s config.Storage) (storage.Store, error) {
	st, err := storage.NewS3Store(ctx, storage.S3Options{
		Endpoint: s.Endpoint,
		Region:   s.Region,
		Bucket:   s.Bucket,
	})
	if err != nil {
		return nil, err
	}
	return st, nil
}

func runWith(ctx context.Context, args []string, stdout, stderr io.Writer, newStore storeFactory) int {
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

	rs, err := restorer.New(cfg.Source.Type)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	store, err := newStore(ctx, cfg.Source.Storage)
	if err != nil {
		fmt.Fprintf(stderr, "error: connect to storage: %v\n", err)
		return 1
	}

	art, err := rs.Fetch(ctx, store, cfg.Source.Storage.Prefix)
	if err != nil {
		if errors.Is(err, storage.ErrNoBackup) {
			fmt.Fprintf(stderr, "no backup found under prefix %q\n", cfg.Source.Storage.Prefix)
			return 4
		}
		fmt.Fprintf(stderr, "error: fetch backup: %v\n", err)
		return 1
	}
	defer func() { _ = art.Remove() }()
	fmt.Fprintf(stdout, "fetched backup %s\n", art.Name)

	if err := rs.Restore(ctx, art, restorer.Target{}); err != nil {
		if errors.Is(err, restorer.ErrNotImplemented) {
			fmt.Fprintf(stderr, "restoring %q backups is not implemented yet\n", cfg.Source.Type)
			return 3
		}
		fmt.Fprintf(stderr, "error: restore: %v\n", err)
		return 1
	}
	return 0
}

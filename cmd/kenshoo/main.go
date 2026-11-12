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
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/check"
	"github.com/Kenshoo-ops/kenshoo/internal/config"
	"github.com/Kenshoo-ops/kenshoo/internal/pgdb"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
	"github.com/Kenshoo-ops/kenshoo/internal/verify"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// adminDSNEnv names the environment variable that holds the DSN of the throwaway PostgreSQL
// server. kenshoo creates, uses and drops one database there and never touches another.
const adminDSNEnv = "KENSHOO_ADMIN_DSN"

// Exit codes. 5 and 6 mean the backup is bad; 1 means kenshoo itself could not judge it.
const (
	exitVerified       = 0
	exitError          = 1
	exitUsage          = 2
	exitNotImplemented = 3
	exitNoBackup       = 4
	exitRestoreFailed  = 5
	exitCheckFailed    = 6
)

// storeFactory builds the Store that a configuration points at.
// Production uses the S3-compatible store, tests use an in-memory one.
type storeFactory func(ctx context.Context, s config.Storage) (storage.Store, error)

// database is the throwaway database a run restores into and queries.
type database interface {
	check.Querier
	Name() string
	DSN() string
	ServerMajor() int
	Drop(ctx context.Context) error
}

// provisionFunc creates the throwaway database on the server adminDSN points at.
type provisionFunc func(ctx context.Context, adminDSN string) (database, error)

// deps is everything a run takes from the outside world, so tests can replace it.
type deps struct {
	newStore    storeFactory
	newRestorer func(name string) (restorer.Restorer, error)
	provision   provisionFunc
	getenv      func(key string) string
}

func productionDeps() deps {
	return deps{
		newStore:    newS3Store,
		newRestorer: restorer.New,
		provision:   provisionPostgres,
		getenv:      os.Getenv,
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the production entry point. A SIGINT or SIGTERM cancels the run's context, so the
// deferred cleanup (drop the database, remove the download) still happens when a person
// presses Ctrl-C or Kubernetes stops the pod.
func run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWith(ctx, args, stdout, stderr, productionDeps())
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

func provisionPostgres(ctx context.Context, adminDSN string) (database, error) {
	db, err := pgdb.Provision(ctx, adminDSN)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// exitCode maps a verdict to the process exit code. An unknown state is an error, never a pass.
func exitCode(s verify.State) int {
	switch s {
	case verify.Verified:
		return exitVerified
	case verify.NoBackup:
		return exitNoBackup
	case verify.RestoreFailed:
		return exitRestoreFailed
	case verify.CheckFailed:
		return exitCheckFailed
	default:
		return exitError
	}
}

func runWith(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("kenshoo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "kenshoo.yaml", "path to the configuration file")
	showVersion := fs.Bool("version", false, "print the version and exit")
	validateOnly := fs.Bool("validate", false, "validate the configuration and exit")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	if *showVersion {
		fmt.Fprintln(stdout, version)
		return exitVerified
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if *validateOnly {
		fmt.Fprintln(stdout, "configuration is valid")
		return exitVerified
	}

	// Fail before any network call: a missing setting should not cost a download.
	adminDSN := d.getenv(adminDSNEnv)
	if adminDSN == "" {
		fmt.Fprintf(stderr, "error: set %s to the DSN of a throwaway PostgreSQL server\n", adminDSNEnv)
		return exitUsage
	}
	wantMajor, _ := strconv.Atoi(cfg.Target.Version) // config.Validate guarantees a clean integer

	checks := make([]check.Check, 0, len(cfg.Checks))
	for _, c := range cfg.Checks {
		ck, err := check.NewSQL(c.Name, c.SQL, c.Expect)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitError
		}
		checks = append(checks, ck)
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Limits.RestoreTimeout)
	defer cancel()

	rs, err := d.newRestorer(cfg.Source.Type)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	store, err := d.newStore(ctx, cfg.Source.Storage)
	if err != nil {
		fmt.Fprintf(stderr, "error: connect to storage: %v\n", err)
		return exitError
	}

	art, fetchErr := rs.Fetch(ctx, store, cfg.Source.Storage.Prefix)
	if fetchErr != nil {
		state := verify.Decide(fetchErr, nil, nil)
		switch {
		case state == verify.NoBackup:
			fmt.Fprintf(stderr, "no backup found under prefix %q\n", cfg.Source.Storage.Prefix)
		case timedOut(ctx):
			reportTimeout(stderr, cfg.Limits.RestoreTimeout)
		default:
			fmt.Fprintf(stderr, "error: fetch backup: %v\n", fetchErr)
		}
		return exitCode(state)
	}
	defer func() {
		if err := art.Remove(); err != nil {
			fmt.Fprintf(stderr, "warning: could not remove the downloaded backup: %v\n", err)
		}
	}()
	fmt.Fprintf(stdout, "fetched backup %s\n", art.Name)

	db, err := d.provision(ctx, adminDSN)
	if err != nil {
		if timedOut(ctx) {
			reportTimeout(stderr, cfg.Limits.RestoreTimeout)
		} else {
			fmt.Fprintf(stderr, "error: provision database: %v\n", err)
		}
		return exitError
	}
	// Registered right after provisioning, so every later return drops the database, and with a
	// context that outlives the run's: a timeout must not leave a database behind.
	defer func() {
		if err := db.Drop(context.WithoutCancel(ctx)); err != nil {
			fmt.Fprintf(stderr, "warning: could not drop database %s, remove it by hand: %v\n", db.Name(), err)
		}
	}()

	if got := db.ServerMajor(); got != wantMajor {
		fmt.Fprintf(stderr, "error: the throwaway server runs PostgreSQL %d but target.version is %d: point %s at a server of that version\n", got, wantMajor, adminDSNEnv)
		return exitError
	}
	fmt.Fprintf(stdout, "provisioned %s on PostgreSQL %d\n", db.Name(), db.ServerMajor())

	restoreErr := rs.Restore(ctx, art, restorer.Target{DSN: db.DSN()})
	if errors.Is(restoreErr, restorer.ErrNotImplemented) {
		fmt.Fprintf(stderr, "restoring %q backups is not implemented yet\n", cfg.Source.Type)
		return exitNotImplemented
	}
	if restoreErr != nil {
		if timedOut(ctx) {
			reportTimeout(stderr, cfg.Limits.RestoreTimeout)
			return exitError
		}
		state := verify.Decide(nil, restoreErr, nil)
		if state == verify.RestoreFailed {
			fmt.Fprintf(stderr, "backup could not be restored: %v\n", restoreErr)
		} else {
			fmt.Fprintf(stderr, "error: restore: %v\n", restoreErr)
		}
		return exitCode(state)
	}
	fmt.Fprintf(stdout, "restored backup into %s\n", db.Name())

	results := check.RunAll(ctx, db, checks)
	for _, r := range results {
		fmt.Fprintf(stdout, "%-5s %s: %s\n", r.Status, r.Name, r.Detail)
	}
	state := verify.Decide(nil, nil, results)
	if state == verify.Error && timedOut(ctx) {
		reportTimeout(stderr, cfg.Limits.RestoreTimeout)
	}
	fmt.Fprintf(stdout, "verdict: %s\n", state)
	return exitCode(state)
}

func reportTimeout(w io.Writer, limit time.Duration) {
	fmt.Fprintf(w, "error: timed out after %s (limits.restore_timeout)\n", limit)
}

// timedOut reports whether the run's deadline ended the call, as opposed to a real failure.
func timedOut(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}

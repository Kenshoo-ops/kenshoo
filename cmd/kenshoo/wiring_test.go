package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
	"github.com/Kenshoo-ops/kenshoo/internal/verify"
)

// fakeRestorer fetches a real temporary file, so tests can see it being removed.
type fakeRestorer struct {
	dir               string
	fetchErr          error
	restoreErr        error
	restoreFn         func(ctx context.Context) error
	fetches, restores int
	target            restorer.Target
	artPath           string
}

func (f *fakeRestorer) Name() string { return "pg_dump" }

func (f *fakeRestorer) Fetch(context.Context, storage.Store, string) (restorer.Artifact, error) {
	f.fetches++
	if f.fetchErr != nil {
		return restorer.Artifact{}, f.fetchErr
	}
	f.artPath = filepath.Join(f.dir, "backup.dump")
	if err := os.WriteFile(f.artPath, []byte("x"), 0o600); err != nil {
		return restorer.Artifact{}, err
	}
	return restorer.Artifact{Name: "backup.dump", Path: f.artPath}, nil
}

func (f *fakeRestorer) Restore(ctx context.Context, _ restorer.Artifact, t restorer.Target) error {
	f.restores++
	f.target = t
	if f.restoreFn != nil {
		return f.restoreFn(ctx)
	}
	return f.restoreErr
}

// recDB records what the run did to it. dropCtxErr is the state of the context Drop received.
type recDB struct {
	major      int
	scalar     string
	queryErr   error
	dropErr    error
	drops      int
	dropCtxErr error
}

func (r *recDB) Name() string     { return "kenshoo_rec" }
func (r *recDB) DSN() string      { return "postgres://u:hunter2@db.example/kenshoo_rec" }
func (r *recDB) ServerMajor() int { return r.major }
func (r *recDB) Drop(ctx context.Context) error {
	r.drops++
	r.dropCtxErr = ctx.Err()
	return r.dropErr
}
func (r *recDB) QueryScalar(context.Context, string) (string, error) { return r.scalar, r.queryErr }

type harness struct {
	fr           *fakeRestorer
	db           *recDB
	provisionErr error
	provisions   int
	cfg          string
	noAdminDSN   bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{
		fr:  &fakeRestorer{dir: t.TempDir()},
		db:  &recDB{major: 16, scalar: "123456789"},
		cfg: wiringConfig,
	}
}

// run executes one full run with fakes. Every output of every test is checked for the DSN password.
func (h *harness) run(ctx context.Context, t *testing.T) (code int, stdout, stderr string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte(h.cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	d := deps{
		newStore:    memFactory(storage.NewMemStore()),
		newRestorer: func(string) (restorer.Restorer, error) { return h.fr, nil },
		provision: func(context.Context, string) (database, error) {
			h.provisions++
			if h.provisionErr != nil {
				return nil, h.provisionErr
			}
			return h.db, nil
		},
		getenv: func(k string) string {
			if k == adminDSNEnv && !h.noAdminDSN {
				return fakeAdminDSN
			}
			return ""
		},
	}
	var out, errOut bytes.Buffer
	code = runWith(ctx, []string{"--config", path}, &out, &errOut, d)
	if all := out.String() + errOut.String(); strings.Contains(all, "hunter2") {
		t.Errorf("output leaks a DSN password:\n%s", all)
	}
	return code, out.String(), errOut.String()
}

func gone(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		t.Fatal("no artifact was fetched")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s was not removed", path)
	}
}

func contains(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s = %q, want it to contain %q", what, got, w)
		}
	}
}

func TestRunVerified(t *testing.T) {
	h := newHarness(t)
	code, out, errOut := h.run(context.Background(), t)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut)
	}
	contains(t, "stdout", out, "fetched backup backup.dump", "provisioned kenshoo_rec on PostgreSQL 16",
		"restored backup into kenshoo_rec", "users-not-empty: expected > 0", "verdict: verified")
	if strings.Contains(out, "123456789") {
		t.Errorf("stdout echoes a query result: %q", out)
	}
	if h.fr.target.DSN != h.db.DSN() {
		t.Errorf("restored into %q, want the provisioned database", h.fr.target.DSN)
	}
	if h.db.drops != 1 {
		t.Errorf("drops = %d, want 1", h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunCheckFailedIsABadBackup(t *testing.T) {
	h := newHarness(t)
	h.db.scalar = "0"
	code, out, _ := h.run(context.Background(), t)
	if code != 6 {
		t.Fatalf("exit code = %d, want 6", code)
	}
	contains(t, "stdout", out, "verdict: check_failed")
	if h.db.drops != 1 {
		t.Errorf("drops = %d, want 1", h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunCheckErrorIsABrokenVerifierNotABadBackup(t *testing.T) {
	h := newHarness(t)
	h.db.queryErr = errors.New("boom")
	code, out, _ := h.run(context.Background(), t)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: a check that could not run says nothing about the backup", code)
	}
	contains(t, "stdout", out, "query failed: boom", "verdict: error")
}

func TestRunRestoreFailedIsABadBackup(t *testing.T) {
	h := newHarness(t)
	h.fr.restoreErr = fmt.Errorf("pg_restore: %w", restorer.ErrRestoreFailed)
	code, out, errOut := h.run(context.Background(), t)
	if code != 5 {
		t.Fatalf("exit code = %d, want 5", code)
	}
	contains(t, "stderr", errOut, "backup could not be restored")
	if strings.Contains(out, "verdict") || strings.Contains(out, "users-not-empty") {
		t.Errorf("checks ran after a failed restore: %q", out)
	}
	if h.db.drops != 1 {
		t.Errorf("drops = %d, want 1", h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunRestoreInfraErrorIsABrokenVerifier(t *testing.T) {
	h := newHarness(t)
	h.fr.restoreErr = errors.New("pg_restore not found")
	code, out, errOut := h.run(context.Background(), t)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	contains(t, "stderr", errOut, "error: restore: pg_restore not found")
	if strings.Contains(out, "verdict") {
		t.Errorf("checks ran after a failed restore: %q", out)
	}
}

func TestRunNotImplementedStillCleansUp(t *testing.T) {
	h := newHarness(t)
	h.fr.restoreErr = fmt.Errorf("pg_dump restore: %w", restorer.ErrNotImplemented)
	code, _, errOut := h.run(context.Background(), t)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	contains(t, "stderr", errOut, "not implemented yet")
	if h.db.drops != 1 {
		t.Errorf("drops = %d, want 1", h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunVersionMismatchNeverRestores(t *testing.T) {
	h := newHarness(t)
	h.db.major = 14
	code, _, errOut := h.run(context.Background(), t)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	contains(t, "stderr", errOut, "PostgreSQL 14", "target.version is 16", adminDSNEnv)
	if h.fr.restores != 0 {
		t.Errorf("restores = %d, want 0: a wrong server version must stop the run first", h.fr.restores)
	}
	if h.db.drops != 1 {
		t.Errorf("drops = %d, want 1", h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunFetchFailuresNeverProvision(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    int
		message string
	}{
		{"no backup", fmt.Errorf("list: %w", storage.ErrNoBackup), 4, "no backup found"},
		{"storage broken", errors.New("connection reset"), 1, "fetch backup: connection reset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.fr.fetchErr = tt.err
			code, _, errOut := h.run(context.Background(), t)
			if code != tt.want {
				t.Errorf("exit code = %d, want %d", code, tt.want)
			}
			contains(t, "stderr", errOut, tt.message)
			if h.provisions != 0 {
				t.Errorf("provisions = %d, want 0: no database for a run that has no backup", h.provisions)
			}
		})
	}
}

func TestRunProvisionFailure(t *testing.T) {
	h := newHarness(t)
	h.provisionErr = errors.New("connect: refused")
	code, _, errOut := h.run(context.Background(), t)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	contains(t, "stderr", errOut, "provision database: connect: refused")
	if h.fr.restores != 0 || h.db.drops != 0 {
		t.Errorf("restores = %d, drops = %d, want 0 and 0", h.fr.restores, h.db.drops)
	}
	gone(t, h.fr.artPath)
}

func TestRunMissingAdminDSNFailsBeforeAnyWork(t *testing.T) {
	h := newHarness(t)
	h.noAdminDSN = true
	code, _, errOut := h.run(context.Background(), t)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	contains(t, "stderr", errOut, adminDSNEnv)
	if h.fr.fetches != 0 || h.provisions != 0 {
		t.Errorf("fetches = %d, provisions = %d, want 0 and 0", h.fr.fetches, h.provisions)
	}
}

func TestRunTimeoutStillDropsWithALiveContext(t *testing.T) {
	h := newHarness(t)
	h.cfg = strings.Replace(wiringConfig, "target:", "limits:\n  restore_timeout: 50ms\ntarget:", 1)
	h.fr.restoreFn = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

	parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, _, errOut := h.run(parent, t)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, errOut)
	}
	contains(t, "stderr", errOut, "timed out after 50ms")
	if h.db.drops != 1 {
		t.Fatalf("drops = %d, want 1: a timeout must not leave the database behind", h.db.drops)
	}
	if h.db.dropCtxErr != nil {
		t.Errorf("Drop received an ended context (%v): it would fail on the real server", h.db.dropCtxErr)
	}
	gone(t, h.fr.artPath)
}

func TestRunCancelStillDropsWithALiveContext(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A SIGTERM from Kubernetes arrives as a cancelled context in the middle of the restore.
	h.fr.restoreFn = func(c context.Context) error { cancel(); <-c.Done(); return c.Err() }

	code, _, errOut := h.run(ctx, t)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, errOut)
	}
	contains(t, "stderr", errOut, "error: restore:")
	if h.db.drops != 1 {
		t.Fatalf("drops = %d, want 1", h.db.drops)
	}
	if h.db.dropCtxErr != nil {
		t.Errorf("Drop received an ended context (%v)", h.db.dropCtxErr)
	}
	gone(t, h.fr.artPath)
}

func TestRunDropFailureWarnsButKeepsTheVerdict(t *testing.T) {
	h := newHarness(t)
	h.db.dropErr = errors.New("boom")
	code, out, errOut := h.run(context.Background(), t)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: a leaked database must not change the verdict", code)
	}
	contains(t, "stdout", out, "verdict: verified")
	contains(t, "stderr", errOut, "could not drop database kenshoo_rec, remove it by hand")
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		state verify.State
		want  int
	}{
		{verify.Verified, 0},
		{verify.Error, 1},
		{verify.NoBackup, 4},
		{verify.RestoreFailed, 5},
		{verify.CheckFailed, 6},
		{verify.State("something new"), 1}, // an unknown verdict must never look like success
	}
	for _, tt := range tests {
		if got := exitCode(tt.state); got != tt.want {
			t.Errorf("exitCode(%q) = %d, want %d", tt.state, got, tt.want)
		}
	}
}

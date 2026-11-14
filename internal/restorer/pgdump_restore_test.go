package restorer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const restoreDSN = "postgres://kenshoo:s3cret@db.example:5433/kenshoo_abc?sslmode=disable"

// fakePgRestore puts an executable named pg_restore first on PATH. It records how it was called
// and exits with the given status, so Restore runs a real process without PostgreSQL's tools.
func fakePgRestore(t *testing.T, exit, stderr string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$FAKE_DIR/args\"\n" +
		"printf '%s' \"$PGPASSWORD\" > \"$FAKE_DIR/password\"\n" +
		"printf '%s' \"$PGHOST\" > \"$FAKE_DIR/host\"\n" +
		"echo \"$FAKE_STDERR\" >&2\n" +
		"exit \"$FAKE_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(dir, "pg_restore"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("FAKE_EXIT", exit)
	t.Setenv("FAKE_STDERR", stderr)
	return dir
}

func dumpFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "b.dump")
	if err := os.WriteFile(p, []byte("PGDMP\x01\x0e"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRestoreRunsPgRestoreAgainstTheTarget(t *testing.T) {
	dir := fakePgRestore(t, "0", "")
	dump := dumpFile(t)

	if err := (pgDump{}).Restore(context.Background(), Artifact{Name: "b.dump", Path: dump}, Target{DSN: restoreDSN}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatalf("pg_restore was not run: %v", err)
	}
	want := []string{"--no-owner", "--no-acl", "--exit-on-error", "--dbname=kenshoo_abc", dump}
	if got := strings.Split(strings.TrimSpace(string(args)), "\n"); !slices.Equal(got, want) {
		t.Errorf("pg_restore args = %q, want exactly %q (the password must never be an argument)", got, want)
	}
	for file, value := range map[string]string{"password": "s3cret", "host": "db.example"} {
		if got, _ := os.ReadFile(filepath.Join(dir, file)); string(got) != value {
			t.Errorf("pg_restore saw %s %q, want %q", file, got, value)
		}
	}
}

func TestRestoreExitStatusMeansABadBackup(t *testing.T) {
	fakePgRestore(t, "1", "pg_restore: error: could not execute query")
	err := (pgDump{}).Restore(context.Background(), Artifact{Name: "b.dump", Path: dumpFile(t)}, Target{DSN: restoreDSN})
	if !errors.Is(err, ErrRestoreFailed) {
		t.Fatalf("error = %v, want ErrRestoreFailed", err)
	}
	if !strings.Contains(err.Error(), "could not execute query") {
		t.Errorf("error = %q, want it to carry pg_restore's message", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks the password: %q", err)
	}
}

func TestRestoreWithoutPgRestoreIsNotABadBackup(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no pg_restore anywhere
	err := (pgDump{}).Restore(context.Background(), Artifact{Name: "b.dump", Path: dumpFile(t)}, Target{DSN: restoreDSN})
	if err == nil {
		t.Fatal("want an error when pg_restore is not installed")
	}
	if errors.Is(err, ErrRestoreFailed) || errors.Is(err, ErrNotImplemented) {
		t.Errorf("error = %v: a missing tool says nothing about the backup", err)
	}
}

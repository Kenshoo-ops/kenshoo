package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/config"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

const wiringConfig = `
source:
  type: pg_dump
  storage:
    type: s3
    bucket: backups
    region: af-casablanca-1
    prefix: my-app/
target:
  engine: postgres
  version: "16"
checks:
  - name: users-not-empty
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
`

func writeWiringConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte(wiringConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func memFactory(m *storage.MemStore) storeFactory {
	return func(context.Context, config.Storage) (storage.Store, error) { return m, nil }
}

func TestRunWithFetchesThenReportsRestoreNotImplemented(t *testing.T) {
	m := storage.NewMemStore()
	m.Put("my-app/backup.dump", []byte("fake dump"), time.Now())

	var out, errOut bytes.Buffer
	code := runWith(context.Background(), []string{"--config", writeWiringConfig(t)}, &out, &errOut, memFactory(m))

	if code != 3 {
		t.Fatalf("exit code = %d, want 3 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "fetched backup backup.dump") {
		t.Errorf("stdout = %q, want it to report the fetched backup", out.String())
	}
}

func TestRunWithNoBackup(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWith(context.Background(), []string{"--config", writeWiringConfig(t)}, &out, &errOut, memFactory(storage.NewMemStore()))

	if code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
	if !strings.Contains(errOut.String(), "no backup found") {
		t.Errorf("stderr = %q, want it to say no backup was found", errOut.String())
	}
}

func TestRunWithStorageError(t *testing.T) {
	failing := func(context.Context, config.Storage) (storage.Store, error) {
		return nil, errors.New("boom")
	}
	var out, errOut bytes.Buffer
	code := runWith(context.Background(), []string{"--config", writeWiringConfig(t)}, &out, &errOut, failing)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "boom") {
		t.Errorf("stderr = %q, want it to include the cause", errOut.String())
	}
}

func TestRunWithDownloadError(t *testing.T) {
	m := storage.NewMemStore()
	m.Put("my-app/backup.dump", []byte("fake dump"), time.Now())
	m.DownloadErr = errors.New("connection reset")

	var out, errOut bytes.Buffer
	code := runWith(context.Background(), []string{"--config", writeWiringConfig(t)}, &out, &errOut, memFactory(m))

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "connection reset") {
		t.Errorf("stderr = %q, want it to include the cause", errOut.String())
	}
}

package restorer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

func TestPgDumpFetchDownloadsNewestBackup(t *testing.T) {
	store := storage.NewMemStore()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store.Put("app/2026-10-01.dump", []byte("old data"), base)
	store.Put("app/2026-10-02.dump", []byte("new data"), base.Add(24*time.Hour))

	a, err := pgDump{}.Fetch(context.Background(), store, "app/")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer a.Remove()

	if a.Name != "2026-10-02.dump" {
		t.Errorf("Name = %q, want 2026-10-02.dump", a.Name)
	}
	got, err := os.ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new data" {
		t.Errorf("file content = %q, want %q", got, "new data")
	}
}

func TestPgDumpFetchNoBackup(t *testing.T) {
	_, err := pgDump{}.Fetch(context.Background(), storage.NewMemStore(), "app/")
	if !errors.Is(err, storage.ErrNoBackup) {
		t.Errorf("error = %v, want it to wrap storage.ErrNoBackup", err)
	}
}

func TestPgDumpFetchDetectsTruncatedDownload(t *testing.T) {
	store := storage.NewMemStore()
	store.Put("app/a.dump", []byte("12345"), time.Now())
	store.SetReportedSize("app/a.dump", 999) // the store claims a size the data does not match

	a, err := pgDump{}.Fetch(context.Background(), store, "app/")
	if err == nil {
		a.Remove()
		t.Fatal("expected an error when the size does not match")
	}
}

func TestPgDumpFetchDownloadErrorLeavesNoTempFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	store := storage.NewMemStore()
	store.Put("app/a.dump", []byte("data"), time.Now())
	store.DownloadErr = errors.New("connection reset")

	if _, err := (pgDump{}).Fetch(context.Background(), store, "app/"); err == nil {
		t.Fatal("expected a download error")
	}
	left, _ := filepath.Glob(filepath.Join(tmpDir, "kenshoo-*.dump"))
	if len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

func TestArtifactRemoveEmpty(t *testing.T) {
	if err := (Artifact{}).Remove(); err != nil {
		t.Errorf("Remove on empty artifact: %v", err)
	}
}

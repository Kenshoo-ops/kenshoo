package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/config"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

// blockingStore finds the backup normally but never finishes a download on its own.
type blockingStore struct{ *storage.MemStore }

func (b blockingStore) Download(ctx context.Context, _ string, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestRunWithTimesOut(t *testing.T) {
	m := storage.NewMemStore()
	m.Put("my-app/backup.dump", []byte("fake dump"), time.Now())

	cfg := strings.Replace(wiringConfig, "target:", "limits:\n  restore_timeout: 50ms\ntarget:", 1)
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	factory := func(context.Context, config.Storage) (storage.Store, error) {
		return blockingStore{m}, nil
	}

	// The parent deadline is only a safety net so a broken fix fails fast
	// instead of hanging the test run.
	parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out, errOut bytes.Buffer
	start := time.Now()
	code := runWith(parent, []string{"--config", path}, &out, &errOut, factory)
	elapsed := time.Since(start)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "timed out after 50ms") {
		t.Errorf("stderr = %q, want the timeout message", errOut.String())
	}
	if elapsed > time.Second {
		t.Errorf("took %s, want about 50ms: the config timeout did not fire", elapsed)
	}
}

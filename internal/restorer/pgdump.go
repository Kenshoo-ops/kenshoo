package restorer

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

func init() {
	Register("pg_dump", func() Restorer { return pgDump{} })
}

// pgDump restores pg_dump files. Fetch is implemented; Restore is still a stub.
type pgDump struct{}

func (pgDump) Name() string { return "pg_dump" }

// Fetch downloads the newest backup under prefix into a temporary file. The caller must
// remove the file with Artifact.Remove when it is done.
//
// If the store holds no backup, the returned error wraps storage.ErrNoBackup, so the
// caller can report "no_backup" instead of a generic failure.
func (pgDump) Fetch(ctx context.Context, store storage.Store, prefix string) (Artifact, error) {
	obj, err := store.Latest(ctx, prefix)
	if err != nil {
		return Artifact{}, fmt.Errorf("find latest backup: %w", err)
	}

	f, err := os.CreateTemp("", "kenshoo-*.dump")
	if err != nil {
		return Artifact{}, fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()

	// From here on, any failure must remove the temp file.
	fail := func(format string, args ...any) (Artifact, error) {
		f.Close()
		os.Remove(tmp)
		return Artifact{}, fmt.Errorf(format, args...)
	}

	if err := store.Download(ctx, obj.Key, f); err != nil {
		return fail("download %q: %w", obj.Key, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return Artifact{}, fmt.Errorf("close temp file: %w", err)
	}

	// A download that is shorter or longer than the stored size is a corrupted copy.
	info, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return Artifact{}, fmt.Errorf("stat temp file: %w", err)
	}
	if info.Size() != obj.Size {
		os.Remove(tmp)
		return Artifact{}, fmt.Errorf("download %q: got %d bytes, expected %d", obj.Key, info.Size(), obj.Size)
	}

	return Artifact{Name: path.Base(obj.Key), Path: tmp}, nil
}

func (pgDump) Restore(ctx context.Context, a Artifact, t Target) error {
	return fmt.Errorf("pg_dump restore: %w", ErrNotImplemented)
}

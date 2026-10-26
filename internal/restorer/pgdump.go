package restorer

import (
	"context"
	"fmt"

	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

func init() {
	Register("pg_dump", func() Restorer { return pgDump{} })
}

// pgDump will restore plain or custom-format pg_dump files. It is a stub for now.
type pgDump struct{}

func (pgDump) Name() string { return "pg_dump" }

func (pgDump) Fetch(ctx context.Context, store storage.Store, prefix string) (Artifact, error) {
	return Artifact{}, fmt.Errorf("pg_dump fetch: %w", ErrNotImplemented)
}

func (pgDump) Restore(ctx context.Context, a Artifact, t Target) error {
	return fmt.Errorf("pg_dump restore: %w", ErrNotImplemented)
}

// Package storage defines how kenshoo reads backups from object storage.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNoBackup means the store holds no backup under the requested prefix.
var ErrNoBackup = errors.New("no backup found")

// Object describes one stored backup file.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Store reads backups. The S3-compatible implementation comes later.
type Store interface {
	// Latest returns the newest object under prefix whose key ends with suffix (any key if
	// suffix is empty), or ErrNoBackup. The suffix filters before the newest is chosen, so a
	// newer checksum file next to the dump cannot win.
	Latest(ctx context.Context, prefix, suffix string) (Object, error)
	// Download streams the object with the given key into dst.
	Download(ctx context.Context, key string, dst io.Writer) error
}

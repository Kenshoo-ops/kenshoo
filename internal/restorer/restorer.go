// Package restorer turns one kind of backup into a running database.
package restorer

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

var (
	// ErrRestoreFailed means the backup itself could not be restored. A restorer returns it
	// (wrapped) only when the backup is the problem, so it can be told apart from a failure
	// of kenshoo itself, such as a network or permission error.
	ErrRestoreFailed = errors.New("restore failed")

	// ErrNotImplemented marks a restorer that exists but has no logic yet.
	ErrNotImplemented = errors.New("not implemented")
)

// Artifact is a backup that has been downloaded and is ready to restore.
type Artifact struct {
	Name string
	Path string
}

// Remove deletes the downloaded file. It is safe to call on an empty Artifact.
func (a Artifact) Remove() error {
	if a.Path == "" {
		return nil
	}
	return os.Remove(a.Path)
}

// Target is the empty database to restore into.
type Target struct {
	DSN string
}

// Restorer is implemented once per backup type: pg_dump now, WAL-G and pgBackRest later.
type Restorer interface {
	// Name is a stable identifier such as "pg_dump".
	Name() string
	// Fetch finds and downloads the newest suitable backup.
	Fetch(ctx context.Context, store storage.Store, prefix string) (Artifact, error)
	// Restore loads the artifact into the target database.
	Restore(ctx context.Context, a Artifact, t Target) error
}

var registry = map[string]func() Restorer{}

// Register makes a restorer available under its name. Call it from an init function.
func Register(name string, factory func() Restorer) {
	if _, dup := registry[name]; dup {
		panic("restorer: duplicate registration of " + name)
	}
	registry[name] = factory
}

// New returns the restorer registered under name.
func New(name string) (Restorer, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown restorer %q", name)
	}
	return factory(), nil
}

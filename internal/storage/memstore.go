package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// MemStore is an in-memory Store. It exists so other packages can test against object storage
// without a network, and it is not used in production.
type MemStore struct {
	// DownloadErr, when set, makes every Download fail with this error.
	DownloadErr error

	mu      sync.Mutex
	objects map[string]memObject
}

type memObject struct {
	obj  Object
	data []byte
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{objects: make(map[string]memObject)}
}

// Put stores data under key with the given modification time.
func (m *MemStore) Put(key string, data []byte, modified time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memObject{
		obj:  Object{Key: key, Size: int64(len(data)), LastModified: modified},
		data: data,
	}
}

// SetReportedSize changes the size a stored object claims to have, without changing its data.
// Tests use it to simulate a truncated or corrupted upload.
func (m *MemStore) SetReportedSize(key string, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := m.objects[key]
	o.obj.Size = size
	m.objects[key] = o
}

// Latest returns the newest object whose key starts with prefix, or ErrNoBackup.
func (m *MemStore) Latest(ctx context.Context, prefix string) (Object, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var best Object
	found := false
	for _, o := range m.objects {
		if !strings.HasPrefix(o.obj.Key, prefix) {
			continue
		}
		if !found || o.obj.LastModified.After(best.LastModified) {
			best, found = o.obj, true
		}
	}
	if !found {
		return Object{}, ErrNoBackup
	}
	return best, nil
}

// Download writes the object's data to dst.
func (m *MemStore) Download(ctx context.Context, key string, dst io.Writer) error {
	if m.DownloadErr != nil {
		return m.DownloadErr
	}
	m.mu.Lock()
	o, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("object %q not found", key)
	}
	_, err := io.Copy(dst, bytes.NewReader(o.data))
	return err
}

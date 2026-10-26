package storage

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemStoreLatestPicksNewestUnderPrefix(t *testing.T) {
	s := NewMemStore()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	s.Put("app/old.dump", []byte("old"), base)
	s.Put("app/new.dump", []byte("new"), base.Add(24*time.Hour))
	s.Put("other/newest.dump", []byte("x"), base.Add(48*time.Hour))

	got, err := s.Latest(context.Background(), "app/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "app/new.dump" {
		t.Errorf("Latest = %q, want app/new.dump", got.Key)
	}
}

func TestMemStoreLatestEmpty(t *testing.T) {
	_, err := NewMemStore().Latest(context.Background(), "app/")
	if !errors.Is(err, ErrNoBackup) {
		t.Errorf("error = %v, want ErrNoBackup", err)
	}
}

func TestMemStoreDownload(t *testing.T) {
	s := NewMemStore()
	s.Put("a", []byte("hello"), time.Now())
	var buf bytes.Buffer
	if err := s.Download(context.Background(), "a", &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Errorf("data = %q, want hello", buf.String())
	}
	if err := s.Download(context.Background(), "missing", &buf); err == nil {
		t.Error("expected an error for a missing object")
	}
}

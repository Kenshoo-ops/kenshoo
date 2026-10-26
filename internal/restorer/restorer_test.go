package restorer

import (
	"context"
	"errors"
	"testing"
)

func TestNewKnownRestorer(t *testing.T) {
	r, err := New("pg_dump")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r.Name() != "pg_dump" {
		t.Errorf("Name() = %q, want pg_dump", r.Name())
	}
}

func TestNewUnknownRestorer(t *testing.T) {
	if _, err := New("nope"); err == nil {
		t.Fatal("expected an error for an unknown restorer")
	}
}

func TestStubIsNotImplemented(t *testing.T) {
	r, _ := New("pg_dump")
	err := r.Restore(context.Background(), Artifact{}, Target{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("error = %v, want ErrNotImplemented", err)
	}
}

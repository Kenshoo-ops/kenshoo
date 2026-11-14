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

func TestRestoreRejectsAnEmptyArtifact(t *testing.T) {
	r, _ := New("pg_dump")
	err := r.Restore(context.Background(), Artifact{}, Target{})
	if err == nil || errors.Is(err, ErrNotImplemented) || errors.Is(err, ErrRestoreFailed) {
		t.Errorf("error = %v, want a plain error: an empty artifact is a kenshoo bug, not a bad backup", err)
	}
}

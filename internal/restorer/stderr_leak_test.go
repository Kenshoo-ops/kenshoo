package restorer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRestoreErrorDoesNotCarryPgRestoreStderr(t *testing.T) {
	const leak = "DETAIL: Key (email)=(victim@example.com) already exists\nlevel=INFO msg=forged"
	var rec recorded
	a := writeTestArchive(t, "PGDMP")
	err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, leak, realExitError(t, "1")), a, Target{DSN: testDSN})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrRestoreFailed) {
		t.Errorf("error does not wrap ErrRestoreFailed: %v", err)
	}
	if !strings.Contains(err.Error(), "status 1") {
		t.Errorf("error lost the exit status: %v", err)
	}
	for _, bad := range []string{"victim@example.com", "forged", "\n"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error carries pg_restore output (%q): %v", bad, err)
		}
	}
}

func TestRestoreDebugKeepsStderrQuotedOnOneLine(t *testing.T) {
	var rec recorded
	a := writeTestArchive(t, "PGDMP")
	err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, "line1\nlevel=INFO msg=forged", realExitError(t, "1")), a, Target{DSN: testDSN, Debug: true})
	if !errors.Is(err, ErrRestoreFailed) {
		t.Fatalf("error does not wrap ErrRestoreFailed: %v", err)
	}
	if !strings.Contains(err.Error(), `line1\nlevel=INFO`) {
		t.Errorf("debug error lost the output: %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("debug error contains a real newline: %q", err.Error())
	}
}

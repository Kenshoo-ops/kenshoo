package verify

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Kenshoo-ops/kenshoo/internal/check"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

func TestDecide(t *testing.T) {
	pass := []check.Result{{Status: check.Pass}}
	tests := []struct {
		name       string
		fetchErr   error
		restoreErr error
		results    []check.Result
		want       State
	}{
		{"all good", nil, nil, pass, Verified},
		{"no backup", storage.ErrNoBackup, nil, nil, NoBackup},
		{"no backup, wrapped", fmt.Errorf("list: %w", storage.ErrNoBackup), nil, nil, NoBackup},
		{"fetch broke", errors.New("network down"), nil, nil, Error},
		{"backup is bad", nil, fmt.Errorf("pg_restore: %w", restorer.ErrRestoreFailed), nil, RestoreFailed},
		{"restore tool broke", nil, errors.New("docker unavailable"), nil, Error},
		{"data check failed", nil, nil, []check.Result{{Status: check.Fail}}, CheckFailed},
		{"data check could not run", nil, nil, []check.Result{{Status: check.Error}}, Error},
		{"no checks ran", nil, nil, nil, CheckFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.fetchErr, tt.restoreErr, tt.results); got != tt.want {
				t.Errorf("Decide = %q, want %q", got, tt.want)
			}
		})
	}
}

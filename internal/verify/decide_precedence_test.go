package verify

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Kenshoo-ops/kenshoo/internal/check"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

// Each case has two problems at once, so only the order of Decide's switch can
// produce the expected state.
func TestDecidePrecedence(t *testing.T) {
	restoreBad := fmt.Errorf("pg_restore: %w", restorer.ErrRestoreFailed)
	toolBroke := errors.New("docker unavailable")
	failed := []check.Result{{Status: check.Fail}}

	tests := []struct {
		name       string
		fetchErr   error
		restoreErr error
		results    []check.Result
		want       State
	}{
		{"no backup beats a restore failure", storage.ErrNoBackup, restoreBad, nil, NoBackup},
		{"fetch error beats a restore failure", errors.New("network down"), restoreBad, nil, Error},
		{"bad backup beats failed checks", nil, restoreBad, failed, RestoreFailed},
		{"bad backup beats an errored check", nil, restoreBad, []check.Result{{Status: check.Error}}, RestoreFailed},
		{"restore tool error beats failed checks", nil, toolBroke, failed, Error},
		{"errored check beats failed check", nil, nil,
			[]check.Result{{Status: check.Pass}, {Status: check.Fail}, {Status: check.Error}}, Error},
		{"errored check beats failed check, other order", nil, nil,
			[]check.Result{{Status: check.Error}, {Status: check.Fail}}, Error},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.fetchErr, tt.restoreErr, tt.results); got != tt.want {
				t.Errorf("Decide = %q, want %q", got, tt.want)
			}
		})
	}
}

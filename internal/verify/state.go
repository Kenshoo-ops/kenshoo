// Package verify holds the decision logic of a verification run.
package verify

import (
	"errors"

	"github.com/Kenshoo-ops/kenshoo/internal/check"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

// State is the final verdict of a run.
type State string

const (
	// Verified: restored, and every check passed.
	Verified State = "verified"
	// RestoreFailed: the backup could not be restored. The BACKUP is bad.
	RestoreFailed State = "restore_failed"
	// CheckFailed: restored, but the data checks failed.
	CheckFailed State = "check_failed"
	// NoBackup: no backup was found in storage.
	NoBackup State = "no_backup"
	// Error: kenshoo itself failed (configuration, network, permissions). The TOOL is the problem.
	Error State = "error"
)

// Decide turns the outcome of each step into one State. Keeping Error separate from
// RestoreFailed tells you whether the backup is bad or the verifier is broken.
func Decide(fetchErr, restoreErr error, results []check.Result) State {
	switch {
	case errors.Is(fetchErr, storage.ErrNoBackup):
		return NoBackup
	case fetchErr != nil:
		return Error
	case errors.Is(restoreErr, restorer.ErrRestoreFailed):
		return RestoreFailed
	case restoreErr != nil:
		return Error
	case check.AnyErrored(results):
		return Error
	case !check.AllPassed(results):
		return CheckFailed
	default:
		return Verified
	}
}

// Package check defines the assertions run against the restored database.
package check

import (
	"context"
	"time"
)

// Status is the outcome of one check.
type Status string

const (
	// Pass means the check ran and the data was as expected.
	Pass Status = "pass"
	// Fail means the check ran and the data was NOT as expected.
	Fail Status = "fail"
	// Error means the check could not run (for example the query failed to execute).
	Error Status = "error"
)

// Result is what one check reports.
type Result struct {
	Name     string
	Status   Status
	Detail   string
	Duration time.Duration
}

// Querier runs a query that returns a single value. The Postgres implementation
// comes later, which keeps checks testable without a database.
type Querier interface {
	QueryScalar(ctx context.Context, sql string) (string, error)
}

// Check is one assertion about the restored data.
type Check interface {
	Name() string
	Run(ctx context.Context, q Querier) Result
}

// RunAll runs every check in order and returns all results, even after a failure,
// so one report shows everything that is wrong.
func RunAll(ctx context.Context, q Querier, checks []Check) []Result {
	results := make([]Result, 0, len(checks))
	for _, c := range checks {
		start := time.Now()
		r := c.Run(ctx, q)
		r.Name = c.Name()
		r.Duration = time.Since(start)
		results = append(results, r)
	}
	return results
}

// AllPassed reports whether every result is a pass. An empty list is not a pass.
func AllPassed(results []Result) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if r.Status != Pass {
			return false
		}
	}
	return true
}

// AnyErrored reports whether at least one check could not run.
func AnyErrored(results []Result) bool {
	for _, r := range results {
		if r.Status == Error {
			return true
		}
	}
	return false
}

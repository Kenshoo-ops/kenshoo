package check

import (
	"context"
	"testing"
	"time"
)

type slowCheck struct{ delay time.Duration }

func (s slowCheck) Name() string { return "slow" }
func (s slowCheck) Run(ctx context.Context, q Querier) Result {
	time.Sleep(s.delay)
	return Result{Status: Pass}
}

func TestRunAllRecordsDuration(t *testing.T) {
	const delay = 20 * time.Millisecond
	results := RunAll(context.Background(), nil, []Check{slowCheck{delay}})
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Duration < delay {
		t.Errorf("Duration = %v, want at least %v", results[0].Duration, delay)
	}
}

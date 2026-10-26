package check

import (
	"context"
	"testing"
)

type fakeCheck struct {
	name   string
	status Status
}

func (f fakeCheck) Name() string { return f.name }
func (f fakeCheck) Run(ctx context.Context, q Querier) Result {
	return Result{Status: f.status}
}

func TestRunAllRunsEveryCheckAfterAFailure(t *testing.T) {
	checks := []Check{
		fakeCheck{"a", Fail},
		fakeCheck{"b", Pass},
	}
	results := RunAll(context.Background(), nil, checks)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Name != "a" || results[1].Name != "b" {
		t.Errorf("names = %q, %q", results[0].Name, results[1].Name)
	}
}

func TestAllPassed(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    bool
	}{
		{"all pass", []Result{{Status: Pass}, {Status: Pass}}, true},
		{"one fails", []Result{{Status: Pass}, {Status: Fail}}, false},
		{"one errors", []Result{{Status: Error}}, false},
		{"empty is not a pass", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllPassed(tt.results); got != tt.want {
				t.Errorf("AllPassed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnyErrored(t *testing.T) {
	if AnyErrored([]Result{{Status: Pass}, {Status: Fail}}) {
		t.Error("a failed check is not an errored check")
	}
	if !AnyErrored([]Result{{Status: Pass}, {Status: Error}}) {
		t.Error("expected AnyErrored to be true")
	}
}

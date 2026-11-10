package check

import (
	"context"
	"fmt"
)

type sqlCheck struct {
	name   string
	sql    string
	expect Expectation
}

// NewSQL builds a check that runs one query and compares its number with an expectation. The
// expectation is parsed here, so a typo fails when the configuration loads, not mid-run.
func NewSQL(name, sql, expect string) (Check, error) {
	e, err := ParseExpect(expect)
	if err != nil {
		return nil, fmt.Errorf("check %q: %w", name, err)
	}
	return sqlCheck{name: name, sql: sql, expect: e}, nil
}

func (c sqlCheck) Name() string { return c.name }

// Run reports Fail only when the data is provably not as expected. A query that errors, or
// returns something that is not a number, is Error: the check could not judge, and it must
// not claim the backup is bad.
func (c sqlCheck) Run(ctx context.Context, q Querier) Result {
	res := Result{Name: c.name}
	raw, err := q.QueryScalar(ctx, c.sql)
	if err != nil {
		res.Status = Error
		res.Detail = "query failed: " + err.Error()
		return res
	}
	got, err := parseNumber(raw)
	if err != nil {
		res.Status = Error
		res.Detail = err.Error()
		return res
	}
	if c.expect.Holds(got) {
		res.Status = Pass
	} else {
		res.Status = Fail
	}
	res.Detail = "expected " + c.expect.String()
	return res
}

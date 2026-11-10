package check

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseExpectAcceptsComparisons(t *testing.T) {
	tests := []struct {
		in    string
		op    string
		value float64
	}{
		{"> 0", ">", 0}, {">0", ">", 0}, {">= 10", ">=", 10}, {"<= 2.5", "<=", 2.5},
		{"= 3", "=", 3}, {"!= 0", "!=", 0}, {"  > 1  ", ">", 1}, {"> -5", ">", -5},
		{"< 26h", "<", 93600}, {"< 90m", "<", 5400}, {"<= 30s", "<=", 30},
		{"< 2d", "<", 172800}, {"< 1.5h", "<", 5400},
	}
	for _, tt := range tests {
		e, err := ParseExpect(tt.in)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.in, err)
			continue
		}
		if e.op != tt.op || e.value != tt.value {
			t.Errorf("%q parsed as (%s, %v), want (%s, %v)", tt.in, e.op, e.value, tt.op, tt.value)
		}
	}
}

func TestParseExpectRejectsEverythingElse(t *testing.T) {
	for _, in := range []string{
		"", "   ", "0", ">", ">>0", "== 1", "<> 1", "> abc", "> 1x", "> 1e5", "> NaN", "> Inf",
		"> 1 2", "> 0.", "> .5", "> --1", ">= =1", "> 1h30m", "> " + strings.Repeat("9", 400),
	} {
		if _, err := ParseExpect(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
	if _, err := ParseExpect(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("empty expect: %v, want it to say expect is required", err)
	}
}

func TestExpectationHolds(t *testing.T) {
	tests := []struct {
		expect string
		got    float64
		want   bool
	}{
		{"> 0", 1, true}, {"> 0", 0, false},
		{">= 0", 0, true}, {">= 0", -1, false},
		{"< 5", 4, true}, {"< 5", 5, false},
		{"<= 5", 5, true}, {"<= 5", 6, false},
		{"= 3", 3, true}, {"= 3", 4, false},
		{"!= 3", 4, true}, {"!= 3", 3, false},
		{"< 26h", 93599, true}, {"< 26h", 93600, false},
	}
	for _, tt := range tests {
		e, err := ParseExpect(tt.expect)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Holds(tt.got); got != tt.want {
			t.Errorf("%q holds for %v = %v, want %v", tt.expect, tt.got, got, tt.want)
		}
	}
	if (Expectation{}).Holds(0) {
		t.Error("a zero Expectation must hold for nothing")
	}
}

func TestParseNumber(t *testing.T) {
	good := map[string]float64{"3": 3, "-3": -3, "1.50": 1.5, ".5": 0.5, "5.": 5, "1e+15": 1e15, "1.5E3": 1500, " 42 ": 42, "+7": 7}
	for in, want := range good {
		if got, err := parseNumber(in); err != nil || got != want {
			t.Errorf("%q: got (%v, %v), want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "NaN", "Infinity", "-Infinity", "0x10", "1_000", "1,5", "1 2", "t", "2026-01-01", "1 day 02:00:00", "--1", "1e999"} {
		if _, err := parseNumber(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
	if _, err := parseNumber("secret@example.com"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("error must not echo the value: %v", err)
	}
}

type scalarQuerier struct {
	value  string
	err    error
	gotSQL string
}

func (s *scalarQuerier) QueryScalar(_ context.Context, sql string) (string, error) {
	s.gotSQL = sql
	return s.value, s.err
}

func runSQL(t *testing.T, expect string, q *scalarQuerier) Result {
	t.Helper()
	c, err := NewSQL("users", "SELECT count(*) FROM users", expect)
	if err != nil {
		t.Fatal(err)
	}
	return c.Run(context.Background(), q)
}

func TestSQLCheckPassAndFail(t *testing.T) {
	if r := runSQL(t, "> 0", &scalarQuerier{value: "3"}); r.Status != Pass {
		t.Errorf("3 > 0: %+v, want Pass", r)
	}
	if r := runSQL(t, "> 0", &scalarQuerier{value: "0"}); r.Status != Fail {
		t.Errorf("0 > 0: %+v, want Fail", r)
	}
	if r := runSQL(t, "< 26h", &scalarQuerier{value: "93599"}); r.Status != Pass {
		t.Errorf("93599s < 26h: %+v, want Pass", r)
	}
	if r := runSQL(t, "< 26h", &scalarQuerier{value: "93600"}); r.Status != Fail {
		t.Errorf("93600s < 26h: %+v, want Fail", r)
	}
}

func TestSQLCheckErrorsAreNotFailures(t *testing.T) {
	if r := runSQL(t, "> 0", &scalarQuerier{err: errors.New("relation users does not exist")}); r.Status != Error || !strings.Contains(r.Detail, "query failed") {
		t.Errorf("failing query: %+v, want Error", r)
	}
	for _, v := range []string{"abc", "NaN", "1 day 02:00:00", ""} {
		if r := runSQL(t, "> 0", &scalarQuerier{value: v}); r.Status != Error {
			t.Errorf("result %q: status %q, want Error: the check could not judge the data", v, r.Status)
		}
	}
}

func TestSQLCheckRunsTheQueryUnchanged(t *testing.T) {
	q := &scalarQuerier{value: "1"}
	runSQL(t, "> 0", q)
	if q.gotSQL != "SELECT count(*) FROM users" {
		t.Errorf("query = %q", q.gotSQL)
	}
}

func TestNewSQLRejectsABadExpectWithTheCheckName(t *testing.T) {
	c, err := NewSQL("freshness", "SELECT 1", ">> 1")
	if err == nil || c != nil || !strings.Contains(err.Error(), "freshness") {
		t.Errorf("got (%v, %v), want a nil check and an error naming the check", c, err)
	}
}

func TestParseExpectRejectsOverflow(t *testing.T) {
	rep := func(c string, n int) string {
		out := ""
		for i := 0; i < n; i++ {
			out += c
		}
		return out
	}
	// 9e307 fits in a float64, but is +Inf once the d unit multiplies it by 86400.
	fits := "9" + rep("0", 307)
	if _, err := ParseExpect("< " + fits); err != nil {
		t.Fatalf("a number that fits must parse: %v", err)
	}
	if _, err := ParseExpect("< " + fits + "d"); err == nil {
		t.Fatal("a value that overflows once the unit is applied must be rejected")
	}
	if _, err := ParseExpect("< " + rep("9", 400)); err == nil {
		t.Fatal("a number beyond float64 must be rejected")
	}
}

func TestSQLCheckNeverEchoesTheValue(t *testing.T) {
	const secret = "123456789"
	cases := []struct {
		name, expect string
		want         Status
	}{
		{"pass", "> 0", Pass},
		{"fail", "< 5", Fail},
	}
	for _, c := range cases {
		r := runSQL(t, c.expect, &scalarQuerier{value: secret + ".5"})
		if r.Status != c.want {
			t.Fatalf("%s: status %q, want %q", c.name, r.Status, c.want)
		}
		if want := "expected " + c.expect; r.Detail != want {
			t.Errorf("%s: Detail = %q, want exactly %q (the result must not appear in any format)", c.name, r.Detail, want)
		}
	}
}

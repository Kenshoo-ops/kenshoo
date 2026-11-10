package check

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Expectation is what a check requires of the number its query returns: an operator and a
// value. A unit (s, m, h, d) on the expected value is shorthand for seconds, so "< 26h" means
// "< 93600" and the query must return seconds, for example extract(epoch FROM now() - max(ts)).
// The zero value holds for nothing, so a missing expectation can never make a check pass.
type Expectation struct {
	op    string
	value float64
	text  string
}

var expectPattern = regexp.MustCompile(`^(>=|<=|!=|=|>|<)\s*(-?[0-9]+(?:\.[0-9]+)?)\s*([smhd])?$`)

// numberPattern is strict on purpose: strconv.ParseFloat also accepts NaN, Infinity, hex
// floats and underscores, none of which a database count or age should ever be.
var numberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

var unitSeconds = map[string]float64{"s": 1, "m": 60, "h": 3600, "d": 86400}

// ParseExpect parses an expect string such as "> 0" or "< 26h".
func ParseExpect(s string) (Expectation, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Expectation{}, errors.New(`expect is required, for example "> 0" or "< 26h"`)
	}
	m := expectPattern.FindStringSubmatch(s)
	if m == nil {
		return Expectation{}, fmt.Errorf("cannot parse expect %q: use an operator (=, !=, >, >=, <, <=) and a number, optionally with a unit s, m, h or d", s)
	}
	v, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return Expectation{}, fmt.Errorf("the number in expect %q is out of range", s)
	}
	if unit := m[3]; unit != "" {
		v *= unitSeconds[unit]
	}
	if math.IsInf(v, 0) {
		return Expectation{}, fmt.Errorf("the number in expect %q is out of range", s)
	}
	return Expectation{op: m[1], value: v, text: s}, nil
}

// Holds reports whether got satisfies the expectation.
func (e Expectation) Holds(got float64) bool {
	switch e.op {
	case "=":
		return got == e.value
	case "!=":
		return got != e.value
	case ">":
		return got > e.value
	case ">=":
		return got >= e.value
	case "<":
		return got < e.value
	case "<=":
		return got <= e.value
	}
	return false
}

func (e Expectation) String() string { return e.text }

// parseNumber reads the text of a query's result. Errors never include the value: a check's
// query may return any column, and its content must not end up in logs.
func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !numberPattern.MatchString(s) {
		return 0, errors.New("query returned a value that is not a number (for an interval, use extract(epoch FROM ...) to get seconds)")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, errors.New("query returned a number that is out of range")
	}
	return v, nil
}

package pgdb

import (
	"context"
	"testing"
)

// Each query makes the server quote the string in its error message, and each fails at a
// different moment, so each one leaves QueryScalar through a different return.
func TestQueryScalarErrorsNeverLeakData(t *testing.T) {
	const leaked = "jane@example.com"
	db := mustProvision(t)
	tests := []struct{ name, sql string }{
		{"fails when the statement is prepared", "SELECT '" + leaked + "'::int"},
		{"fails on the first row", "SELECT ('" + leaked + "' || g::text)::int FROM generate_series(1, 1) g"},
		{"fails on the second row", "SELECT CASE WHEN g = 1 THEN '1' ELSE '" + leaked + "' END::int FROM generate_series(1, 2) g"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.QueryScalar(context.Background(), tt.sql)
			if err == nil {
				t.Fatal("expected the query to fail")
			}
			if got, want := err.Error(), "SQLSTATE 22P02"; got != want {
				t.Errorf("error = %q, want exactly %q", got, want)
			}
		})
	}
}

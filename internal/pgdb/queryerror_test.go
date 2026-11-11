package pgdb

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSafeQueryError(t *testing.T) {
	const leaked = "jane@example.com"
	server := func(code string) error {
		return &pgconn.PgError{
			Severity: "ERROR",
			Code:     code,
			Message:  `invalid input syntax for type integer: "` + leaked + `"`,
			Detail:   "Key (email)=(" + leaked + ") already exists.",
			Hint:     leaked,
			Where:    "SQL function " + leaked,
		}
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"server error keeps only the SQLSTATE", server("22P02"), "SQLSTATE 22P02"},
		{"wrapped server error", fmt.Errorf("query %s: %w", leaked, server("42703")), "SQLSTATE 42703"},
		{"a malformed SQLSTATE is not trusted", server(leaked), "unexpected database error"},
		{"unknown error", errors.New("write tcp " + leaked + ": broken pipe"), "unexpected database error"},
		{"deadline", fmt.Errorf("%s: %w", leaked, context.DeadlineExceeded), "timed out"},
		{"cancelled", fmt.Errorf("%s: %w", leaked, context.Canceled), "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeQueryError(tt.err).Error(); got != tt.want {
				t.Errorf("safeQueryError = %q, want exactly %q", got, tt.want)
			}
		})
	}
}

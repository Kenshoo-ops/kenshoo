package pgdb

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqlstatePattern is the shape of a PostgreSQL SQLSTATE: five digits or capital letters.
var sqlstatePattern = regexp.MustCompile(`^[0-9A-Z]{5}$`)

// safeQueryError reduces an error from a check's query to text that is safe to log. A server
// message can quote data ("invalid input syntax for type integer: \"jane@example.com\""), so
// only an allow-list gets through: the SQLSTATE, or a fixed phrase. The original error is not
// wrapped, so its text cannot come back through Unwrap.
func safeQueryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && sqlstatePattern.MatchString(pgErr.Code) {
		return errors.New("SQLSTATE " + pgErr.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timed out")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("cancelled")
	}
	return errors.New("unexpected database error")
}

// Package dsnquery is the one place that decides which URL query parameters a database
// DSN may carry. The database client and pg_restore must see the same target, so the
// allow-list is shared.
package dsnquery

import (
	"errors"
	"net/url"
)

// Check accepts only a single sslmode parameter. Any other key (dbname, host, options,
// search_path, ...) could point the checks at a different database than the one that was
// restored, so it is rejected. Errors never include the DSN or the key: a DSN holds a password.
func Check(u *url.URL) error {
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return errors.New("DSN query string is malformed")
	}
	for key, vals := range q {
		if key != "sslmode" {
			return errors.New("DSN query may only contain sslmode")
		}
		if len(vals) != 1 {
			return errors.New("DSN query may give sslmode only once")
		}
	}
	return nil
}

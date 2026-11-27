package pgdb

import (
	"strings"
	"testing"
)

var redirectingQueries = []string{
	"dbname=prod", "database=prod", "host=other", "port=1", "user=x", "password=x",
	"options=-csearch_path%3Dx", "search_path=x", "DBNAME=prod",
	"sslmode=disable&sslmode=require",
}

func TestWithDatabaseRejectsRedirectingQueryKeys(t *testing.T) {
	for _, q := range redirectingQueries {
		_, err := withDatabase("postgres://u:secret@localhost:5432/postgres?"+q, "kenshoo_verify_ab12")
		if err == nil {
			t.Errorf("query %q: expected an error", q)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("query %q: error leaks the password: %v", q, err)
		}
	}
}

func TestWithDatabaseKeepsSSLMode(t *testing.T) {
	got, err := withDatabase("postgres://u:secret@localhost:5432/postgres?sslmode=disable", "kenshoo_verify_ab12")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/kenshoo_verify_ab12") || !strings.Contains(got, "sslmode=disable") {
		t.Errorf("got %q", got)
	}
}

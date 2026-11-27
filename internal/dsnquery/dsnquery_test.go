package dsnquery

import (
	"net/url"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"sslmode=disable", true},
		{"sslmode=disable&sslmode=require", false},
		{"dbname=prod", false},
		{"DBNAME=prod", false},
		{"SSLMODE=disable", false},
		{"sslmode=disable&dbname=prod", false},
		{"options=-csearch_path%3Dx", false},
		{"sslmode=a;dbname=prod", false},
	} {
		u, err := url.Parse("postgres://u:secret@h/db?" + tc.query)
		if err != nil {
			t.Fatal(err)
		}
		err = Check(u)
		if tc.ok && err != nil {
			t.Errorf("query %q: unexpected error %v", tc.query, err)
		}
		if !tc.ok {
			if err == nil {
				t.Errorf("query %q: expected an error", tc.query)
			} else if strings.Contains(err.Error(), "secret") {
				t.Errorf("query %q: error leaks the password: %v", tc.query, err)
			}
		}
	}
}

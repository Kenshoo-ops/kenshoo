package restorer

import (
	"strings"
	"testing"
)

func TestParseTargetRejectsRedirectingQueryKeys(t *testing.T) {
	for _, q := range []string{
		"dbname=prod", "database=prod", "host=other", "port=1", "user=x", "password=x",
		"options=-csearch_path%3Dx", "search_path=x", "DBNAME=prod",
		"sslmode=disable&sslmode=require",
	} {
		_, _, err := parseTarget(nil, "postgres://u:secret@localhost/kenshoo_verify_ab12?"+q)
		if err == nil {
			t.Errorf("query %q: expected an error", q)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("query %q: error leaks the password: %v", q, err)
		}
	}
}

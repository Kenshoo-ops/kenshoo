package restorer

import (
	"strings"
	"testing"
)

func TestParseTargetChildEnvIsAnAllowList(t *testing.T) {
	base := []string{
		"PATH=/usr/bin", "TMPDIR=/tmp/x", "LANG=en_US.UTF-8",
		"AWS_SECRET_ACCESS_KEY=topsecret", "AWS_ACCESS_KEY_ID=akid",
		"KENSHOO_ADMIN_DSN=postgres://admin:adminpw@db/postgres",
		"HOME=/home/x", "LANGUAGE=fr", "PATHEXT=.exe", "TMPDIRX=1",
		"PGHOST=prod.example.com", "NOEQUALS",
	}
	env, _, err := parseTarget(base, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PATH=/usr/bin", "TMPDIR=/tmp/x", "LANG=en_US.UTF-8", "PGHOST=localhost", "PGUSER=verifier"} {
		if !hasItem(env, want) {
			t.Errorf("env is missing %q", want)
		}
	}
	allowed := map[string]bool{
		"PATH": true, "TMPDIR": true, "LANG": true,
		"PGHOST": true, "PGPORT": true, "PGUSER": true, "PGPASSWORD": true, "PGSSLMODE": true,
	}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !allowed[key] {
			t.Errorf("child env carries %q, which pg_restore does not need", key)
		}
	}
}

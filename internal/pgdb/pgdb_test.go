package pgdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewNameFormatAndUniqueness(t *testing.T) {
	re := regexp.MustCompile(`^kenshoo_verify_[0-9a-f]{16}$`)
	a, err := newName()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newName()
	if !re.MatchString(a) || !re.MatchString(b) {
		t.Errorf("names %q and %q do not match %s", a, b, re)
	}
	if a == b {
		t.Error("two generated names are equal")
	}
}

func TestWithDatabaseKeepsEverythingButTheName(t *testing.T) {
	got, err := withDatabase("postgres://admin:pw@db.example:5433/postgres?sslmode=require", "kenshoo_verify_ab")
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://admin:pw@db.example:5433/kenshoo_verify_ab?sslmode=require"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithDatabaseRejectsBadDSNsWithoutLeakingThePassword(t *testing.T) {
	for _, dsn := range []string{"", "mysql://u:pw@host/db", "postgres://u:pw@/db", "not a url pw"} {
		_, err := withDatabase(dsn, "kenshoo_verify_ab")
		if err == nil {
			t.Errorf("DSN %q: expected an error", dsn)
		} else if strings.Contains(err.Error(), "pw") {
			t.Errorf("DSN %q: error leaks the password: %v", dsn, err)
		}
	}
}

func TestDropRefusesDatabasesItDidNotCreate(t *testing.T) {
	for _, d := range []*Database{{}, {name: "production"}, {name: "postgres"}} {
		err := d.Drop(context.Background())
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("Drop on %q: %v, want a refusal", d.name, err)
		}
	}
}

func TestDatabaseNeverPrintsItsDSN(t *testing.T) {
	d := Database{name: "kenshoo_verify_x", adminDSN: "postgres://a:secret@h/p", dsn: "postgres://a:secret@h/kenshoo_verify_x"}
	for _, s := range []string{fmt.Sprint(d), fmt.Sprintf("%v", &d), fmt.Sprintf("%+v", d)} {
		if strings.Contains(s, "secret") {
			t.Errorf("printing a Database leaks the password: %q", s)
		}
	}
}

func TestConnectErrorsNeverLeakThePassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := connect(ctx, "postgres://u:secret@host:notaport/db"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("unparseable DSN: %v, want an error without the password", err)
	}
	_, err := Provision(ctx, "postgres://u:secret@127.0.0.1:1/db?sslmode=disable")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("unreachable server: %v, want an error without the password", err)
	}
}

func TestServerMajor(t *testing.T) {
	for num, want := range map[string]int{"160002": 16, "130000": 13, "170001": 17} {
		got, err := serverMajor(num)
		if err != nil || got != want {
			t.Errorf("serverMajor(%q) = (%d, %v), want %d", num, got, err, want)
		}
	}
	for _, num := range []string{"120009", "90624", "", "abc"} {
		if _, err := serverMajor(num); err == nil {
			t.Errorf("serverMajor(%q): expected an error", num)
		}
	}
}

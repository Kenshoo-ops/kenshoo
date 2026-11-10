package pgdb

import (
	"context"
	"os"
	"strings"
	"testing"
)

// adminDSN skips the test unless a throwaway PostgreSQL server is available.
func adminDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KENSHOO_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set KENSHOO_TEST_PG_DSN to a throwaway PostgreSQL server to run this test")
	}
	return dsn
}

func mustProvision(t *testing.T) Database {
	t.Helper()
	db, err := Provision(context.Background(), adminDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	return db
}

func databaseExists(t *testing.T, name string) bool {
	t.Helper()
	ctx := context.Background()
	conn, err := connect(ctx, adminDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn(conn)
	var n string
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datname = $1", name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n != "0"
}

func TestProvisionCreatesAndDropsADatabase(t *testing.T) {
	db := mustProvision(t)
	if db.ServerMajor() < MinServerMajor {
		t.Errorf("server major = %d", db.ServerMajor())
	}
	if !databaseExists(t, db.Name()) {
		t.Fatal("the database was not created")
	}
	if err := db.Drop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if databaseExists(t, db.Name()) {
		t.Error("the database still exists after Drop")
	}
	if err := db.Drop(context.Background()); err != nil {
		t.Errorf("a second Drop should be harmless: %v", err)
	}
}

func TestDropStillWorksAfterTheContextEnded(t *testing.T) {
	db := mustProvision(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.Drop(ctx); err != nil {
		t.Fatalf("Drop with a cancelled context: %v", err)
	}
	if databaseExists(t, db.Name()) {
		t.Error("the database still exists: cleanup must survive an expired deadline")
	}
}

func TestQueryScalar(t *testing.T) {
	db := mustProvision(t)
	ctx := context.Background()

	conn, err := connect(ctx, db.DSN())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "CREATE TABLE users (id int); INSERT INTO users VALUES (1),(2),(3)"); err != nil {
		t.Fatal(err)
	}
	closeConn(conn)

	good := map[string]string{
		"SELECT count(*) FROM users": "3",
		"SELECT true":                "t",
		"SELECT 1.50::numeric":       "1.50",
		"SELECT ''":                  "",
	}
	for sql, want := range good {
		got, err := db.QueryScalar(ctx, sql)
		if err != nil || got != want {
			t.Errorf("%s: got (%q, %v), want %q", sql, got, err, want)
		}
	}

	bad := map[string]string{
		"SELECT id FROM users WHERE false": "no rows",
		"SELECT id FROM users":             "more than one row",
		"SELECT 1, 2":                      "exactly one column",
		"SELECT NULL":                      "NULL",
		"INSERT INTO users VALUES (9)":     "read-only",
		"SELECT * FROM missing_table":      "does not exist",
	}
	for sql, want := range bad {
		_, err := db.QueryScalar(ctx, sql)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to contain %q", sql, err, want)
		}
	}

	if got, _ := db.QueryScalar(ctx, "SELECT count(*) FROM users"); got != "3" {
		t.Errorf("a rejected write changed the data: count = %q", got)
	}
}

func TestDropTerminatesLingeringConnections(t *testing.T) {
	db := mustProvision(t)
	ctx := context.Background()

	conn, err := connect(ctx, db.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn(conn)
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}

	if err := db.Drop(ctx); err != nil {
		t.Fatalf("Drop with a session still connected: %v", err)
	}
	if databaseExists(t, db.Name()) {
		t.Error("the database still exists: a lingering session must not block the cleanup")
	}
}

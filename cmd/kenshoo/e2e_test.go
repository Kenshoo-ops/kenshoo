package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kenshoo-ops/kenshoo/internal/pgdb"
	"github.com/Kenshoo-ops/kenshoo/internal/restorer"
	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

// requireE2E returns the admin DSN of the throwaway server. Without the server or the PostgreSQL
// client tools the test skips on a laptop but fails on CI, because a skipped test reports as ok.
func requireE2E(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KENSHOO_TEST_PG_DSN")
	missing := ""
	if dsn == "" {
		missing = "KENSHOO_TEST_PG_DSN is not set"
	}
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing = tool + " is not on PATH"
		}
	}
	if missing != "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("end-to-end prerequisites missing on CI: %s", missing)
		}
		t.Skipf("end-to-end test needs a throwaway server and the PostgreSQL client tools: %s", missing)
	}
	return dsn
}

// sourceDump creates a database with three users, dumps it with the real pg_dump and returns the archive.
func sourceDump(t *testing.T, ctx context.Context, adminDSN string) []byte {
	t.Helper()
	src, err := pgdb.Provision(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Drop(context.Background()) })

	conn, err := pgx.Connect(ctx, src.DSN())
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, "CREATE TABLE users (id int); INSERT INTO users VALUES (1),(2),(3)")
	_ = conn.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "src.dump")
	if out, err := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--file="+path, src.DSN()).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func serverMajorOf(t *testing.T, ctx context.Context, adminDSN string) int {
	t.Helper()
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var num string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&num); err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(num)
	return n / 10000
}

func dbExists(t *testing.T, ctx context.Context, adminDSN, name string) bool {
	t.Helper()
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var ok bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestEndToEndWithRealPostgres(t *testing.T) {
	adminDSN := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	major := serverMajorOf(t, ctx, adminDSN)
	requireDumpToolMatches(t, ctx, major)
	good := sourceDump(t, ctx, adminDSN)
	corrupt := append([]byte("PGDMP"), bytes.Repeat([]byte{0xFF}, 128)...)

	tests := []struct {
		name   string
		dump   []byte
		expect string
		code   int
		stdout []string
		stderr []string
	}{
		{"a good backup is verified", good, "> 0", 0, []string{"restored backup into", "users-not-empty: expected > 0", "verdict: verified"}, nil},
		{"data that fails the check is a bad backup", good, "> 100", 6, []string{"verdict: check_failed"}, nil},
		{"a corrupt archive is a bad backup", corrupt, "> 0", 5, nil, []string{"backup could not be restored"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemStore()
			store.Put("my-app/backup.dump", tt.dump, time.Now())
			cfg := strings.Replace(wiringConfig, `version: "16"`, fmt.Sprintf(`version: "%d"`, major), 1)
			cfg = strings.Replace(cfg, `expect: "> 0"`, `expect: "`+tt.expect+`"`, 1)
			path := filepath.Join(t.TempDir(), "kenshoo.yaml")
			if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
				t.Fatal(err)
			}
			d := deps{
				newStore:    memFactory(store),
				newRestorer: restorer.New,
				provision:   provisionPostgres,
				getenv: func(k string) string {
					if k == adminDSNEnv {
						return adminDSN
					}
					return ""
				},
			}
			var out, errOut bytes.Buffer
			code := runWith(ctx, []string{"--config", path}, &out, &errOut, d)
			if code != tt.code {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.code, out.String(), errOut.String())
			}
			contains(t, "stdout", out.String(), tt.stdout...)
			contains(t, "stderr", errOut.String(), tt.stderr...)
			found := regexp.MustCompile(`provisioned (\S+) on PostgreSQL`).FindStringSubmatch(out.String())
			if found == nil {
				t.Fatalf("no database was provisioned:\n%s", out.String())
			}
			if dbExists(t, ctx, adminDSN, found[1]) {
				t.Errorf("database %s still exists after the run", found[1])
			}
		})
	}
}

var pgVersionPattern = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

// checkDumpTool compares the major version in `pg_dump --version` with the server's. A newer
// pg_dump can write statements an older server refuses (pg_dump 18 writes SET transaction_timeout,
// which a 16 server rejects), and that would look like a bad backup instead of a setup problem.
func checkDumpTool(versionOutput string, serverMajor int) error {
	m := pgVersionPattern.FindStringSubmatch(versionOutput)
	if m == nil {
		return fmt.Errorf("cannot read a version from %q", strings.TrimSpace(versionOutput))
	}
	got, err := strconv.Atoi(m[1])
	if err != nil {
		return fmt.Errorf("cannot read a version from %q: %w", strings.TrimSpace(versionOutput), err)
	}
	if got != serverMajor {
		return fmt.Errorf("pg_dump is version %d but the throwaway server runs %d: put a version %d client first on PATH", got, serverMajor, serverMajor)
	}
	return nil
}

func requireDumpToolMatches(t *testing.T, ctx context.Context, serverMajor int) {
	t.Helper()
	out, err := exec.CommandContext(ctx, "pg_dump", "--version").Output()
	if err != nil {
		t.Fatalf("pg_dump --version: %v", err)
	}
	if err := checkDumpTool(string(out), serverMajor); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDumpTool(t *testing.T) {
	tests := []struct{ name, output, want string }{
		{"same major", "pg_dump (PostgreSQL) 16.16 (Homebrew)\n", ""},
		{"newer client", "pg_dump (PostgreSQL) 18.7\n", "pg_dump is version 18 but the throwaway server runs 16"},
		{"older client", "pg_dump (PostgreSQL) 15.4\n", "pg_dump is version 15 but"},
		{"not a version", "command not found", "cannot read a version"},
		{"empty", "", "cannot read a version"},
		{"absurd version", "pg_dump (PostgreSQL) 99999999999999999999.1", "cannot read a version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDumpTool(tt.output, 16)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

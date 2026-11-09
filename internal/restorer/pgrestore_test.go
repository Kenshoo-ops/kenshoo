package restorer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testDSN = "postgres://verifier:s3cret@localhost:5432/kenshoo_verify_ab12?sslmode=disable"

// realExitError returns the *exec.ExitError of a command that really exited with code.
func realExitError(t *testing.T, code string) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+code).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected an ExitError, got %v", err)
	}
	return err
}

func writeTestArchive(t *testing.T, content string) Artifact {
	t.Helper()
	p := filepath.Join(t.TempDir(), "backup.dump")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return Artifact{Name: "backup.dump", Path: p}
}

type recorded struct {
	calls int
	name  string
	args  []string
	env   []string
}

func recordingRunner(r *recorded, stderr string, err error) commandRunner {
	return func(_ context.Context, name string, args, env []string) (string, error) {
		r.calls++
		r.name, r.args, r.env = name, args, env
		return stderr, err
	}
}

func hasItem(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestParseTargetBuildsEnvAndStripsInheritedPGVars(t *testing.T) {
	base := []string{"HOME=/home/x", "PGHOST=prod.example.com", "PGSERVICE=prod", "PGPASSFILE=/etc/prod"}
	env, db, err := parseTarget(base, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	if db != "kenshoo_verify_ab12" {
		t.Errorf("database = %q", db)
	}
	for _, want := range []string{"HOME=/home/x", "PGHOST=localhost", "PGPORT=5432", "PGUSER=verifier", "PGPASSWORD=s3cret", "PGSSLMODE=disable"} {
		if !hasItem(env, want) {
			t.Errorf("env is missing %q", want)
		}
	}
	for _, bad := range []string{"PGHOST=prod.example.com", "PGSERVICE=prod", "PGPASSFILE=/etc/prod"} {
		if hasItem(env, bad) {
			t.Errorf("env kept the inherited %q", bad)
		}
	}
}

func TestParseTargetRejectsBadDSNsWithoutLeakingThePassword(t *testing.T) {
	for _, dsn := range []string{
		"",
		"::bad::secret",
		"mysql://u:secret@localhost/db",
		"postgres://u:secret@localhost",
		"postgres://u:secret@/db",
		"postgres://u:secret@localhost/x=y",
		"postgres://u:secret@localhost/-flag",
		"postgres://u:secret@localhost/db name",
	} {
		_, _, err := parseTarget(nil, dsn)
		if err == nil {
			t.Errorf("DSN %q: expected an error", dsn)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("DSN %q: error leaks the password: %v", dsn, err)
		}
	}
}

func TestClassifyRestore(t *testing.T) {
	exit := realExitError(t, "3")
	expired, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-expired.Done()
	notFound := &exec.Error{Name: "pg_restore", Err: exec.ErrNotFound}

	if err := classifyRestore(context.Background(), nil, ""); err != nil {
		t.Errorf("success classified as %v", err)
	}

	err := classifyRestore(context.Background(), exit, "boom")
	if !errors.Is(err, ErrRestoreFailed) {
		t.Errorf("non-zero exit: %v, want it to wrap ErrRestoreFailed", err)
	}
	if !strings.Contains(err.Error(), "status 3") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("message = %q, want the exit status and stderr", err)
	}

	err = classifyRestore(expired, exit, "")
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRestoreFailed) {
		t.Errorf("killed by the deadline: %v, want DeadlineExceeded and not ErrRestoreFailed", err)
	}

	err = classifyRestore(context.Background(), notFound, "")
	if errors.Is(err, ErrRestoreFailed) || !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("missing binary: %v, want ErrNotFound and not ErrRestoreFailed", err)
	}
}

func TestRestoreRunsPgRestoreWithSafeArguments(t *testing.T) {
	a := writeTestArchive(t, "PGDMP-rest-of-archive")
	var rec recorded
	if err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, "", nil), a, Target{DSN: testDSN}); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 || rec.name != "pg_restore" {
		t.Fatalf("calls = %d, name = %q", rec.calls, rec.name)
	}
	for _, want := range []string{"--no-owner", "--no-acl", "--exit-on-error", "--dbname=kenshoo_verify_ab12", a.Path} {
		if !hasItem(rec.args, want) {
			t.Errorf("args are missing %q: %v", want, rec.args)
		}
	}
	for _, arg := range rec.args {
		if strings.Contains(arg, "s3cret") || strings.Contains(arg, "localhost") {
			t.Errorf("argument %q leaks connection details; they belong in the environment", arg)
		}
	}
	if !hasItem(rec.env, "PGPASSWORD=s3cret") {
		t.Error("the password did not reach the child's environment")
	}
}

func TestRestoreRejectsNonArchives(t *testing.T) {
	for name, content := range map[string]string{"plain SQL": "SELECT 1;\n", "empty": "", "too short": "PGD"} {
		var rec recorded
		err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, "", nil), writeTestArchive(t, content), Target{DSN: testDSN})
		if !errors.Is(err, ErrRestoreFailed) {
			t.Errorf("%s: %v, want ErrRestoreFailed", name, err)
		}
		if rec.calls != 0 {
			t.Errorf("%s: pg_restore ran on a file that is not an archive", name)
		}
	}
}

func TestRestoreBadDSNIsNotABackupProblem(t *testing.T) {
	var rec recorded
	err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, "", nil), writeTestArchive(t, "PGDMP"), Target{DSN: "postgres://u:secret@localhost"})
	if err == nil || errors.Is(err, ErrRestoreFailed) {
		t.Errorf("bad DSN: %v, want an error that is not ErrRestoreFailed", err)
	}
	if rec.calls != 0 {
		t.Error("pg_restore ran with an invalid DSN")
	}
}

func TestRestoreFailureAndMissingBinary(t *testing.T) {
	a := writeTestArchive(t, "PGDMP")

	var rec recorded
	err := (pgDump{}).restore(context.Background(), recordingRunner(&rec, "relation exists", realExitError(t, "1")), a, Target{DSN: testDSN})
	if !errors.Is(err, ErrRestoreFailed) {
		t.Errorf("pg_restore failure: %v, want ErrRestoreFailed", err)
	}

	err = (pgDump{}).restore(context.Background(), recordingRunner(&rec, "", &exec.Error{Name: "pg_restore", Err: exec.ErrNotFound}), a, Target{DSN: testDSN})
	if err == nil || errors.Is(err, ErrRestoreFailed) {
		t.Errorf("missing binary: %v, want an error that is not ErrRestoreFailed", err)
	}
}

func TestRestoreTimeoutIsNotABadBackup(t *testing.T) {
	a := writeTestArchive(t, "PGDMP")
	exit := realExitError(t, "1")
	blocking := func(ctx context.Context, _ string, _, _ []string) (string, error) {
		<-ctx.Done()
		return "", exit // a killed process reports an ExitError, like a failed restore does
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := (pgDump{}).restore(ctx, blocking, a, Target{DSN: testDSN})
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrRestoreFailed) {
		t.Errorf("error = %v, want DeadlineExceeded and not ErrRestoreFailed", err)
	}
}

func TestExecRunnerReturnsStderrAndExitStatus(t *testing.T) {
	stderr, err := execRunner(context.Background(), "sh", []string{"-c", "echo oops >&2; exit 4"}, os.Environ())
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 4 {
		t.Fatalf("error = %v, want exit status 4", err)
	}
	if stderr != "oops" {
		t.Errorf("stderr = %q, want %q", stderr, "oops")
	}
}

func TestExecRunnerBoundsStderr(t *testing.T) {
	stderr, _ := execRunner(context.Background(), "sh", []string{"-c", "yes x | head -c 10000 >&2; exit 1"}, os.Environ())
	if len(stderr) == 0 || len(stderr) > stderrTailBytes {
		t.Errorf("stderr length = %d, want between 1 and %d", len(stderr), stderrTailBytes)
	}
}

func TestExecRunnerStopsCommandWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := execRunner(ctx, "sleep", []string{"30"}, os.Environ())
	if err == nil {
		t.Fatal("expected an error when the command is stopped")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("command ran for %s after a 100ms deadline", elapsed)
	}
}

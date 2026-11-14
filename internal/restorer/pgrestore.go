package restorer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// archiveMagic is how every custom-format pg_dump archive starts.
const archiveMagic = "PGDMP"

// stderrTailBytes bounds how much of pg_restore's stderr is kept for error messages.
const stderrTailBytes = 2048

// dbNamePattern is deliberately strict. kenshoo creates the throwaway database itself,
// and a name that doubles as a connection string or a flag could send pg_restore elsewhere.
var dbNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.$-]*$`)

// commandRunner runs an external command and returns the tail of its stderr together
// with the raw error from running it. Tests replace it with a fake.
type commandRunner func(ctx context.Context, name string, args, env []string) (stderrTail string, err error)

// tailBuffer keeps only the last stderrTailBytes bytes written to it.
type tailBuffer struct{ buf []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTailBytes {
		t.buf = t.buf[len(t.buf)-stderrTailBytes:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return strings.TrimSpace(string(t.buf)) }

// execRunner is the real commandRunner. On cancellation it asks the process to stop
// with an interrupt, and kills it if it is still running five seconds later.
func execRunner(ctx context.Context, name string, args, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	var stderr tailBuffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// parseTarget turns a postgres:// URL into the environment pg_restore runs with and the
// database name to restore into. Inherited PG* variables are dropped so nothing from the
// caller's shell can redirect the restore. Errors never include the DSN: it holds a password.
func parseTarget(base []string, dsn string) (env []string, dbname string, err error) {
	u, perr := url.Parse(dsn)
	if perr != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		return nil, "", errors.New("target DSN must be a postgres:// URL with a host")
	}
	dbname = strings.TrimPrefix(u.Path, "/")
	if !dbNamePattern.MatchString(dbname) {
		return nil, "", errors.New("target DSN must name a database using only letters, digits, _ . $ -")
	}

	env = make([]string, 0, len(base)+6)
	for _, kv := range base {
		if !strings.HasPrefix(kv, "PG") {
			env = append(env, kv)
		}
	}
	env = append(env, "PGHOST="+u.Hostname())
	if port := u.Port(); port != "" {
		env = append(env, "PGPORT="+port)
	}
	if user := u.User.Username(); user != "" {
		env = append(env, "PGUSER="+user)
	}
	if pw, ok := u.User.Password(); ok {
		env = append(env, "PGPASSWORD="+pw)
	}
	if ssl := u.Query().Get("sslmode"); ssl != "" {
		env = append(env, "PGSSLMODE="+ssl)
	}
	return env, dbname, nil
}

// checkArchive rejects files that are not custom-format pg_dump archives. A wrong file is
// a backup problem, so those errors wrap ErrRestoreFailed.
func checkArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer f.Close()

	magic := make([]byte, len(archiveMagic))
	if _, err := io.ReadFull(f, magic); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("%w: file is too short to be a pg_dump archive", ErrRestoreFailed)
		}
		return fmt.Errorf("read backup: %w", err)
	}
	if string(magic) != archiveMagic {
		return fmt.Errorf("%w: not a custom-format pg_dump archive (missing %s header)", ErrRestoreFailed, archiveMagic)
	}
	return nil
}

// classifyRestore decides whose fault a failed pg_restore run was. The order matters: a
// process killed by the deadline reports an ExitError, which must not be mistaken for a
// bad backup.
func classifyRestore(ctx context.Context, err error, stderr string) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("pg_restore: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		msg := fmt.Errorf("pg_restore exited with status %d: %w", exitErr.ExitCode(), ErrRestoreFailed)
		if stderr != "" {
			return fmt.Errorf("%w: %s", msg, stderr)
		}
		return msg
	}
	return fmt.Errorf("run pg_restore: %w", err)
}

// restore loads the archive into the database the DSN names, starting pg_restore through run.
func (pgDump) restore(ctx context.Context, run commandRunner, a Artifact, t Target) error {
	if a.Path == "" {
		return errors.New("pg_restore: artifact has no file")
	}
	env, dbname, err := parseTarget(os.Environ(), t.DSN)
	if err != nil {
		return fmt.Errorf("pg_restore: %w", err)
	}
	if err := checkArchive(a.Path); err != nil {
		return err
	}
	args := []string{"--no-owner", "--no-acl", "--exit-on-error", "--dbname=" + dbname, a.Path}
	stderr, err := run(ctx, "pg_restore", args, env)
	return classifyRestore(ctx, err, stderr)
}

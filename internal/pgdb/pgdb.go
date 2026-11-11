// Package pgdb creates and drops the throwaway database a backup is restored into, and
// runs the checks' queries against it. kenshoo only ever restores into a database it
// created itself, so a mistaken DSN can never make it overwrite a real one.
package pgdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Kenshoo-ops/kenshoo/internal/check"
)

// MinServerMajor is the oldest PostgreSQL major version supported as the throwaway server:
// DROP DATABASE ... WITH (FORCE) needs 13.
const MinServerMajor = 13

const namePrefix = "kenshoo_verify_"

var _ check.Querier = Database{}

// Database is a throwaway database that kenshoo created and must drop.
type Database struct {
	name        string
	adminDSN    string
	dsn         string
	serverMajor int
}

// Name is the database's name.
func (d Database) Name() string { return d.name }

// DSN is the connection URL of the throwaway database. It contains the password: never log it.
func (d Database) DSN() string { return d.dsn }

// ServerMajor is the PostgreSQL major version of the throwaway server.
func (d Database) ServerMajor() int { return d.serverMajor }

// String never includes a DSN, so printing a Database cannot leak the password.
func (d Database) String() string { return "database " + d.name }

func newName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return namePrefix + hex.EncodeToString(b), nil
}

// withDatabase returns dsn with its database replaced by name.
func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" {
		return "", errors.New("admin DSN must be a postgres:// URL with a host")
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}

// connect opens one connection. Errors from parsing never include the DSN.
func connect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid DSN")
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.RuntimeParams["application_name"] = "kenshoo"
	return pgx.ConnectConfig(ctx, cfg)
}

func cleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func closeConn(c *pgx.Conn) {
	ctx, cancel := cleanupCtx()
	defer cancel()
	_ = c.Close(ctx)
}

func rollback(tx pgx.Tx) {
	ctx, cancel := cleanupCtx()
	defer cancel()
	_ = tx.Rollback(ctx)
}

// serverMajor converts server_version_num (for example "160002") into the major version and
// rejects servers older than MinServerMajor.
func serverMajor(versionNum string) (int, error) {
	num, err := strconv.Atoi(versionNum)
	if err != nil {
		return 0, fmt.Errorf("unexpected server_version_num %q", versionNum)
	}
	major := num / 10000
	if major < MinServerMajor {
		return 0, fmt.Errorf("the throwaway server runs PostgreSQL %d, kenshoo needs %d or newer", major, MinServerMajor)
	}
	return major, nil
}

// Provision creates an empty, randomly named database on the server adminDSN points at.
// Every failure here is a problem with kenshoo's setup, never with the backup.
func Provision(ctx context.Context, adminDSN string) (Database, error) {
	name, err := newName()
	if err != nil {
		return Database{}, fmt.Errorf("generate database name: %w", err)
	}
	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		return Database{}, err
	}

	conn, err := connect(ctx, adminDSN)
	if err != nil {
		return Database{}, fmt.Errorf("connect to the throwaway server: %w", err)
	}
	defer closeConn(conn)

	var versionNum string
	if err := conn.QueryRow(ctx, "SHOW server_version_num").Scan(&versionNum); err != nil {
		return Database{}, fmt.Errorf("read server version: %w", err)
	}
	major, err := serverMajor(versionNum)
	if err != nil {
		return Database{}, err
	}

	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return Database{}, fmt.Errorf("create database %s (the admin role needs the CREATEDB privilege): %w", name, err)
	}
	return Database{name: name, adminDSN: adminDSN, dsn: dsn, serverMajor: major}, nil
}

// Drop removes the database. It refuses to touch any database kenshoo did not create, and it
// uses its own deadline so it still runs after the run's context has expired. Calling it
// twice is safe.
func (d Database) Drop(ctx context.Context) error {
	if !strings.HasPrefix(d.name, namePrefix) {
		return errors.New("refusing to drop a database kenshoo did not create")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	conn, err := connect(ctx, d.adminDSN)
	if err != nil {
		return fmt.Errorf("connect to drop %s: %w", d.name, err)
	}
	defer closeConn(conn)

	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{d.name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop database %s: %w", d.name, err)
	}
	return nil
}

// QueryScalar runs one query in a read-only transaction and returns its single value in
// PostgreSQL's text form (so a boolean is "t" or "f"). Zero rows, several rows, several
// columns and NULL are all errors: a check cannot judge a value that is not there.
func (d Database) QueryScalar(ctx context.Context, sql string) (string, error) {
	conn, err := connect(ctx, d.dsn)
	if err != nil {
		return "", fmt.Errorf("connect to %s: %w", d.name, err)
	}
	defer closeConn(conn)

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return "", fmt.Errorf("begin read-only transaction: %w", err)
	}
	defer rollback(tx)

	rows, err := tx.Query(ctx, sql)
	if err != nil {
		return "", safeQueryError(err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", safeQueryError(err)
		}
		return "", errors.New("query returned no rows")
	}
	if n := len(rows.FieldDescriptions()); n != 1 {
		return "", fmt.Errorf("query must return exactly one column, got %d", n)
	}
	raw := rows.RawValues()[0]
	isNull := raw == nil
	value := string(raw)
	if rows.Next() {
		return "", errors.New("query returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return "", safeQueryError(err)
	}
	if isNull {
		return "", errors.New("query returned NULL")
	}
	return value, nil
}

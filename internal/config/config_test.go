package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		Source: Source{
			Type:    "pg_dump",
			Storage: Storage{Type: "s3", Bucket: "backups"},
		},
		Target: Target{Engine: "postgres", Version: "16"},
		Checks: []Check{{Name: "users", SQL: "SELECT count(*) FROM users", Expect: "> 0"}},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // empty means valid
	}{
		{"valid", func(c *Config) {}, ""},
		{"wrong source type", func(c *Config) { c.Source.Type = "wal-g" }, "source.type"},
		{"missing bucket", func(c *Config) { c.Source.Storage.Bucket = "" }, "bucket"},
		{"wrong engine", func(c *Config) { c.Target.Engine = "mysql" }, "target.engine"},
		{"no checks", func(c *Config) { c.Checks = nil }, "at least one check"},
		{"unnamed check", func(c *Config) { c.Checks[0].Name = "" }, "name is required"},
		{"check without sql", func(c *Config) { c.Checks[0].SQL = "" }, "sql is required"},
		{"duplicate check", func(c *Config) { c.Checks = append(c.Checks, c.Checks[0]) }, "duplicate"},
		{"negative timeout", func(c *Config) { c.Limits.RestoreTimeout = -time.Second }, "restore_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAppliesDefaultTimeout(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.RestoreTimeout != DefaultRestoreTimeout {
		t.Errorf("timeout = %v, want %v", cfg.Limits.RestoreTimeout, DefaultRestoreTimeout)
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	content := `
source:
  type: pg_dump
  storage:
    type: s3
    bucket: backups
target:
  engine: postgres
  version: "16"
checks:
  - name: users-not-empty
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
limits:
  restore_timeout: 5m
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Limits.RestoreTimeout != 5*time.Minute {
		t.Errorf("timeout = %v, want 5m", cfg.Limits.RestoreTimeout)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte("surce:\n  type: pg_dump\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

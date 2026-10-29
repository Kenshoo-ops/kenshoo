package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validYAML is a complete, valid configuration. Tests add exactly one problem
// to it, so a failure can only come from that problem.
const validYAML = `
source:
  type: pg_dump
  storage:
    type: s3
    bucket: backups
    region: af-casablanca-1
target:
  engine: postgres
  version: "16"
checks:
  - name: users-not-empty
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
`

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsUnknownFieldInValidConfig(t *testing.T) {
	if _, err := Load(writeYAML(t, validYAML)); err != nil {
		t.Fatalf("control config should load, got: %v", err)
	}

	typo := strings.Replace(validYAML, "    region: af-casablanca-1\n", "    region: af-casablanca-1\n    buket: typo\n", 1)
	_, err := Load(writeYAML(t, typo))
	if err == nil {
		t.Fatal("expected an error for an unknown field in an otherwise valid config")
	}
	if !strings.Contains(err.Error(), "buket") {
		t.Errorf("error = %v, want it to name the unknown field %q", err, "buket")
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := validConfig()
	cfg.Source.Storage.Bucket = ""
	cfg.Source.Storage.Region = ""
	cfg.Checks = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"bucket", "region", "at least one check"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestValidateRejectsWrongStorageType(t *testing.T) {
	for _, typ := range []string{"gcs", ""} {
		cfg := validConfig()
		cfg.Source.Storage.Type = typ
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "storage.type") {
			t.Errorf("storage type %q: error = %v, want it to mention storage.type", typ, err)
		}
	}
}

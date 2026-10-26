package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("output = %q, want %q", out.String(), version)
	}
}

func TestRunMissingConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--config", filepath.Join(t.TempDir(), "nope.yaml")}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunValidateExampleConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	path := filepath.Join("..", "..", "examples", "kenshoo.example.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("example config not found: %v", err)
	}
	if code := run([]string{"--config", path, "--validate"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, errOut.String())
	}
}

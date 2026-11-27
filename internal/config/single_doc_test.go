package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kenshoo.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadAcceptsOneDocument(t *testing.T) {
	cfg := validConfig()
	cfg.Checks = append(cfg.Checks, Check{Name: "dashes", SQL: "SELECT count(*)\n---\nFROM users", Expect: "> 0"})
	one, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"plain":                   string(one),
		"leading marker":          "---\n" + string(one),
		"trailing marker":         string(one) + "---\n",
		"--- inside a block text": string(one),
	} {
		if _, err := loadYAML(t, body); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}

func TestLoadRejectsASecondDocument(t *testing.T) {
	one, err := yaml.Marshal(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	for name, second := range map[string]string{
		"valid second config": string(one),
		"broken second":       "source: [secretvalue",
		"unknown content":     "secretvalue: 1\n",
	} {
		_, err := loadYAML(t, string(one)+"---\n"+second)
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if !strings.Contains(err.Error(), "single YAML document") {
			t.Errorf("%s: error = %q, want the fixed message", name, err)
		}
		if strings.Contains(err.Error(), "secretvalue") {
			t.Errorf("%s: error quotes the file content: %q", name, err)
		}
	}
}

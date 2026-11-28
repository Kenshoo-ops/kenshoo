package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateSuffix(t *testing.T) {
	for suffix, ok := range map[string]bool{
		"": true, ".dump": true, ".pg_dump": true,
		"a/b": false, "/": false, "x\n": false, "x\x00": false, "\x7f": false,
	} {
		cfg := validConfig()
		cfg.Source.Storage.Suffix = suffix
		if err := cfg.Validate(); (err == nil) != ok {
			t.Errorf("suffix %q: error = %v, want ok=%v", suffix, err, ok)
		}
	}
}

func TestLoadReadsSuffix(t *testing.T) {
	cfg := validConfig()
	cfg.Source.Storage.Suffix = ".dump"
	body, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadYAML(t, string(body))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source.Storage.Suffix != ".dump" {
		t.Errorf("suffix = %q, want .dump", got.Source.Storage.Suffix)
	}
}

// Package config loads and validates the kenshoo configuration file.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultRestoreTimeout applies when limits.restore_timeout is not set.
const DefaultRestoreTimeout = 15 * time.Minute

// Config is the full configuration of one verification run.
type Config struct {
	Source Source  `yaml:"source"`
	Target Target  `yaml:"target"`
	Checks []Check `yaml:"checks"`
	Limits Limits  `yaml:"limits"`
}

// Source says where the backup comes from and in which format.
type Source struct {
	Type    string  `yaml:"type"`
	Storage Storage `yaml:"storage"`
}

// Storage describes S3-compatible object storage.
type Storage struct {
	Type     string `yaml:"type"`
	Endpoint string `yaml:"endpoint"`
	Bucket   string `yaml:"bucket"`
	Prefix   string `yaml:"prefix"`
}

// Target is the throwaway database the backup is restored into.
type Target struct {
	Engine  string `yaml:"engine"`
	Version string `yaml:"version"`
}

// Check is one assertion run against the restored database.
type Check struct {
	Name   string `yaml:"name"`
	SQL    string `yaml:"sql"`
	Expect string `yaml:"expect"`
}

// Limits bounds a run.
type Limits struct {
	RestoreTimeout time.Duration `yaml:"restore_timeout"`
}

// Load reads, parses and validates the configuration file at path.
// Unknown fields are rejected, so a typo in a key is an error and not silently ignored.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg Config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

// Validate checks the configuration and fills in defaults. It reports every problem at once.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if c.Source.Type != "pg_dump" {
		add("source.type must be %q (got %q)", "pg_dump", c.Source.Type)
	}
	if c.Source.Storage.Type != "s3" {
		add("source.storage.type must be %q (got %q)", "s3", c.Source.Storage.Type)
	}
	if c.Source.Storage.Bucket == "" {
		add("source.storage.bucket is required")
	}
	if c.Target.Engine != "postgres" {
		add("target.engine must be %q (got %q)", "postgres", c.Target.Engine)
	}
	if len(c.Checks) == 0 {
		add("at least one check is required")
	}

	seen := make(map[string]bool)
	for i, ch := range c.Checks {
		switch {
		case ch.Name == "":
			add("checks[%d].name is required", i)
		case seen[ch.Name]:
			add("checks[%d]: duplicate check name %q", i, ch.Name)
		default:
			seen[ch.Name] = true
		}
		if ch.SQL == "" {
			add("checks[%d].sql is required", i)
		}
	}

	if c.Limits.RestoreTimeout < 0 {
		add("limits.restore_timeout must not be negative")
	}
	if c.Limits.RestoreTimeout == 0 {
		c.Limits.RestoreTimeout = DefaultRestoreTimeout
	}

	return errors.Join(errs...)
}

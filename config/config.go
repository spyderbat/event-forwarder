// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	defaultAPIHost = "api.prod.spyderbat.com"
	defaultLogPath = "./"
)

type Config struct {
	APIHost               string   `yaml:"api_host"`
	LogPath               string   `yaml:"log_path"`
	OrgUID                string   `yaml:"spyderbat_org_uid"`
	APIKey                string   `yaml:"spyderbat_secret_api_key"`
	LocalSyslogForwarding bool     `yaml:"local_syslog_forwarding"`
	StdOut                bool     `yaml:"stdout"`
	Webhook               *Webhook `yaml:"webhook"`
}

const iteratorFile = "iterator"

func (c *Config) iteratorFile() string {
	return filepath.Join(c.LogPath, iteratorFile)
}

// GetIterator returns the last stored iterator, or the fallback if it doesn't exist.
func (c *Config) GetIterator(fallback string) (string, error) {
	iteratorBytes, err := os.ReadFile(c.iteratorFile())
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return "", fmt.Errorf("failed to read iterator file: %w", err)
	}
	iterator := strings.TrimSpace(string(iteratorBytes))
	// If the file is empty, return the fallback
	if len(iterator) == 0 {
		return fallback, nil
	}
	// If the file is not empty, return the iterator
	return string(iterator), nil
}

func (c *Config) WriteIterator(iterator string) error {
	tmpFile := c.iteratorFile() + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(iterator), 0600); err != nil {
		return fmt.Errorf("failed to write iterator file: %w", err)
	}
	if err := os.Rename(tmpFile, c.iteratorFile()); err != nil {
		return fmt.Errorf("failed to rename iterator file: %w", err)
	}
	return nil
}

// configItem validation
type configItem struct {
	Value     *string                   // Value to check
	Key       string                    // Config key name, should match struct tag
	Default   string                    // Default value if one is not provided
	Required  bool                      // Die if value is not provided? (No default will be used)
	Validator func(c *configItem) error // If set, validate further
}

// ensure the log path is a valid, writeable directory
func validateLogPath(c *configItem) error {
	st, err := os.Stat(*c.Value)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s: not a directory", *c.Value)
	}
	f, err := os.CreateTemp(*c.Value, "")
	if err != nil {
		return fmt.Errorf("unable to write to %s: %w", *c.Value, os.ErrPermission)
	}
	return os.Remove(f.Name())
}

// PrepareAndValidate validates the config and compiles expressions. It is called
// automatically by LoadConfig, and provided here for testing.
func (c *Config) PrepareAndValidate() error {
	validation := []configItem{
		{
			Value:   &c.APIHost,
			Key:     "api_host",
			Default: defaultAPIHost,
		},
		{
			Value:     &c.LogPath,
			Key:       "log_path",
			Default:   defaultLogPath,
			Validator: validateLogPath,
		},
		{
			Value:    &c.OrgUID,
			Key:      "spyderbat_org_uid",
			Required: true,
		},
		{
			Value:    &c.APIKey,
			Key:      "spyderbat_secret_api_key",
			Required: true,
		},
	}

	for _, v := range validation {
		if *v.Value == "" {
			if v.Required {
				return fmt.Errorf("no value for required config key '%s'", v.Key)
			}
			*v.Value = v.Default
		}
		if v.Validator != nil {
			err := v.Validator(&v)
			if err != nil {
				return fmt.Errorf("failed to validate config key '%s': %w", v.Key, err)
			}
		}
	}

	return ValidateWebhook(c.Webhook)
}

// LoadConfig loads and parses a yaml config
func LoadConfig(filename string) (*Config, error) {
	log.Printf("loading config from %s", filename)
	c := &Config{}

	d, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	err = yaml.Unmarshal(d, c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	err = c.PrepareAndValidate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return c, nil
}

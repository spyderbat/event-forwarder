// Spyderbat Event Forwarder
// Copyright (C) 2022-2023 Spyderbat, Inc.
// Use according to license terms.

package config

import (
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
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
	MatchRegex            []string `yaml:"matching_filters"`
	Expr                  string   `yaml:"expr"`

	exprProgram *vm.Program
	reg         []*regexp.Regexp
}

func (c *Config) GetExprProgram() *vm.Program {
	return c.exprProgram
}

func (c *Config) GetRegexes() []*regexp.Regexp {
	return c.reg
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
		{
			Value: &c.Expr,
			Key:   "expr",
			Validator: func(i *configItem) error {
				if len(*i.Value) > 0 {
					program, err := expr.Compile(*i.Value, expr.AsBool())
					c.exprProgram = program
					return err
				} else {
					return nil
				}
			},
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

	if len(c.MatchRegex) > 0 && len(c.Expr) > 0 {
		return fmt.Errorf("cannot use both 'expr' and 'matching_filters'")
	}

	for _, regex := range c.MatchRegex {
		regex, err := regexp.Compile(regex)
		if err != nil {
			return fmt.Errorf("failed to compile regex '%s': %w", regex, err)
		}
		c.reg = append(c.reg, regex)
	}

	return nil
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

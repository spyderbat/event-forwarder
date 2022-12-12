package config

import (
	"fmt"
	"log"
	"os"
	"sort"
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
	DataTypes             string   `yaml:"data_types"`
	dataTypes             []string `yaml:"-"` // parsed data types from DataTypes, loaded on validation
}

// configItem validation
type configItem struct {
	Value     *string                   // Value to check
	Key       string                    // Config key name, should match struct tag
	Default   string                    // Default value if one is not provided
	Required  bool                      // Die if value is not provided? (No default will be used)
	Validator func(i *configItem) error // If set, validate further
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

// return comma-separated, de-duplicated values as a slice
func getCommaSeparatedValues(c *configItem) []string {
	dupMap := make(map[string]struct{})
	types := strings.Split(*c.Value, ",")
	for _, k := range types {
		dupMap[k] = struct{}{}
	}
	types = make([]string, 0, len(dupMap))
	for k := range dupMap {
		types = append(types, k)
	}
	sort.Strings(types)
	return types
}

// ensure the data types are valid
func (c *Config) validateDataTypes(i *configItem) error {
	validDataTypes := map[string]struct{}{
		"redflags":     {},
		"spydertraces": {},
	}

	types := getCommaSeparatedValues(i)
	if len(types) == 0 {
		return fmt.Errorf("no data types provided")
	}

	for _, t := range types {
		if _, ok := validDataTypes[t]; !ok {
			return fmt.Errorf("invalid data type %s", t)
		}
	}

	c.dataTypes = types
	return nil
}

// LoadConfig loads and parses a yaml config
func LoadConfig(filename string) (*Config, error) {
	log.Printf("loading config from %s", filename)
	c := &Config{}

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
			Value:     &c.DataTypes,
			Key:       "data_types",
			Default:   "redflags",
			Validator: c.validateDataTypes,
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

	d, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	err = yaml.Unmarshal(d, c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	for _, v := range validation {
		if *v.Value == "" {
			if v.Required {
				return nil, fmt.Errorf("no value for required config key '%s' in %s", v.Key, filename)
			}
			*v.Value = v.Default
		}
		if v.Validator != nil {
			err := v.Validator(&v)
			if err != nil {
				return nil, fmt.Errorf("failed to validate config key '%s': %w", v.Key, err)
			}
		}
	}

	return c, nil
}

func (c *Config) GetDataTypes() []string {
	return c.dataTypes
}

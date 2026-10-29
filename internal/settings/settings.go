// Package settings stores user-owned scan roots and protected paths.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Version    int      `json:"version"`
	Roots      []string `json:"roots,omitempty"`
	Exclusions []string `json:"exclusions,omitempty"`
}

func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "handbyte"), nil
}

func Load() (Config, error) {
	c := Config{Version: 1}
	dir, err := Dir()
	if err != nil {
		return c, err
	}
	path := filepath.Join(dir, "config.json")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return c, errors.New("config must be a regular file")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid cleanup config: %w", err)
	}
	if c.Version != 1 {
		return c, fmt.Errorf("unsupported config version %d", c.Version)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, errors.New("unexpected config content")
	}
	for _, path := range append(append([]string{}, c.Roots...), c.Exclusions...) {
		if path == "" {
			return c, errors.New("config paths must not be empty")
		}
	}
	return c, nil
}

func Save(c Config) error {
	if c.Version != 1 {
		return errors.New("unsupported config version")
	}
	// Refuse to overwrite unreadable or unrecognized user configuration.
	if _, err := Load(); err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, "config.json"))
}

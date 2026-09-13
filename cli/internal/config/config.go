// Package config reads and writes ckit's config file, ~/.ckit/config.json.
//
// The directory is derived from os.UserHomeDir, never os.UserConfigDir: on
// Windows the latter is %APPDATA%, which a process running under an MSIX
// package identity (e.g. a terminal inside the Claude desktop app) gets
// silently redirected to a private per-package copy, splitting state between
// launchers.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirName is the per-user directory under the home directory.
const DirName = ".ckit"

// FileName is the config file inside DirName.
const FileName = "config.json"

// Config is the whole file. Empty string means "unset, use the default".
type Config struct {
	ProjectsDir  string `json:"projectsDir,omitempty"`
	SmartgitPath string `json:"smartgitPath,omitempty"`
	CodePath     string `json:"codePath,omitempty"`
}

// Key describes one settable key.
type Key struct {
	Name        string
	Description string
	get         func(*Config) *string
}

// Keys lists every settable key, in display order.
var Keys = []Key{
	{"projectsDir", "directory holding your projects (default ~/Projects)", func(c *Config) *string { return &c.ProjectsDir }},
	{"smartgitPath", "SmartGit executable/app override (default: per-OS location)", func(c *Config) *string { return &c.SmartgitPath }},
	{"codePath", "VS Code `code` CLI override (default: `code` on PATH)", func(c *Config) *string { return &c.CodePath }},
}

// KeyNames returns the settable key names.
func KeyNames() []string {
	out := make([]string, len(Keys))
	for i, k := range Keys {
		out[i] = k.Name
	}
	return out
}

func findKey(name string) (Key, error) {
	for _, k := range Keys {
		if strings.EqualFold(k.Name, name) {
			return k, nil
		}
	}
	names := KeyNames()
	sort.Strings(names)
	return Key{}, fmt.Errorf("unknown config key %q; valid keys: %s", name, strings.Join(names, ", "))
}

// Get returns the raw stored value of a key ("" if unset).
func (c *Config) Get(name string) (string, error) {
	k, err := findKey(name)
	if err != nil {
		return "", err
	}
	return *k.get(c), nil
}

// Set stores a value; an empty value unsets the key.
func (c *Config) Set(name, value string) error {
	k, err := findKey(name)
	if err != nil {
		return err
	}
	*k.get(c) = strings.TrimSpace(value)
	return nil
}

// Dir is the config directory for a home directory.
func Dir(home string) string { return filepath.Join(home, DirName) }

// Path is the config file path for a home directory.
func Path(home string) string { return filepath.Join(Dir(home), FileName) }

// Home returns the user's home directory (USERPROFILE on Windows, HOME
// elsewhere), which is also how tests point ckit at a temp directory.
func Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return h, nil
}

// Load reads the config file. A missing file is not an error: it is the zero
// Config, i.e. all defaults.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s is not valid JSON: %w (fix or delete it)", path, err)
	}
	return c, nil
}

// Save writes the config file atomically (temp file + rename), creating the
// directory if needed.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// ExpandHome turns a leading "~" or "~/" into the home directory.
func ExpandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// ProjectsDir is the effective projects directory: the configured one with
// "~" expanded, or <home>/Projects.
func (c *Config) ResolvedProjectsDir(home string) string {
	if c.ProjectsDir == "" {
		return filepath.Join(home, "Projects")
	}
	return filepath.Clean(ExpandHome(c.ProjectsDir, home))
}

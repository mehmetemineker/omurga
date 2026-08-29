package remote

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Address    string `yaml:"address" json:"address"`
	User       string `yaml:"user,omitempty" json:"user,omitempty"`
	Port       int    `yaml:"port,omitempty" json:"port,omitempty"`
	Identity   string `yaml:"identity,omitempty" json:"identity,omitempty"`
	OmurgaPath string `yaml:"omurgaPath,omitempty" json:"omurgaPath,omitempty"`
	Sudo       bool   `yaml:"sudo,omitempty" json:"sudo,omitempty"`
}

type File struct {
	Version int                `yaml:"version" json:"version"`
	Hosts   map[string]Profile `yaml:"hosts" json:"hosts"`
}

type Store struct {
	Path string
}

func DefaultPath() (string, error) {
	if override := os.Getenv("OMURGA_CONFIG_HOME"); override != "" {
		return filepath.Join(override, "hosts.yaml"), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve user configuration directory: %w", err)
	}
	return filepath.Join(root, "omurga", "hosts.yaml"), nil
}

func DefaultStore() (Store, error) {
	path, err := DefaultPath()
	return Store{Path: path}, err
}

func (s Store) Load() (File, error) {
	content, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return File{Version: 1, Hosts: map[string]Profile{}}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("could not read host profiles: %w", err)
	}
	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("could not decode host profiles: %w", err)
	}
	if file.Version != 1 {
		return File{}, fmt.Errorf("unsupported host profile version %d", file.Version)
	}
	if file.Hosts == nil {
		file.Hosts = map[string]Profile{}
	}
	return file, nil
}

func (s Store) Put(name string, profile Profile) error {
	if err := Validate(name, profile); err != nil {
		return err
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	file.Hosts[name] = profile
	return s.save(file)
}

func (s Store) Get(name string) (Profile, bool, error) {
	file, err := s.Load()
	if err != nil {
		return Profile{}, false, err
	}
	profile, exists := file.Hosts[name]
	return profile, exists, nil
}

func (s Store) List() ([]string, map[string]Profile, error) {
	file, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(file.Hosts))
	for name := range file.Hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, file.Hosts, nil
}

func (s Store) Remove(name string) (bool, error) {
	file, err := s.Load()
	if err != nil {
		return false, err
	}
	if _, exists := file.Hosts[name]; !exists {
		return false, nil
	}
	delete(file.Hosts, name)
	return true, s.save(file)
}

func Validate(name string, profile Profile) error {
	if name == "" || name == "local" || strings.ContainsAny(name, " /\\\t\r\n") {
		return fmt.Errorf("invalid host profile name %q", name)
	}
	if profile.Address == "" || strings.ContainsAny(profile.Address, "\r\n") {
		return fmt.Errorf("host address is required")
	}
	if profile.Port < 0 || profile.Port > 65535 {
		return fmt.Errorf("host port must be between 1 and 65535")
	}
	return nil
}

func (s Store) save(file File) error {
	content, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("could not encode host profiles: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("could not create host profile directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".omurga-hosts-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(s.Path)
	}
	if err := os.Rename(temporaryPath, s.Path); err != nil {
		return fmt.Errorf("could not replace host profiles: %w", err)
	}
	return nil
}

package registry

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
	Address  string `yaml:"address" json:"address"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
}

type File struct {
	Version    int                `yaml:"version" json:"version"`
	Registries map[string]Profile `yaml:"registries" json:"registries"`
}

type Store struct{ Path string }

func DefaultStore() (Store, error) {
	root := os.Getenv("OMURGA_CONFIG_HOME")
	if root == "" {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return Store{}, err
		}
		root = filepath.Join(root, "omurga")
	}
	return Store{Path: filepath.Join(root, "registries.yaml")}, nil
}

func (s Store) Load() (File, error) {
	content, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return File{Version: 1, Registries: map[string]Profile{}}, nil
	}
	if err != nil {
		return File{}, err
	}
	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("could not decode registry profiles: %w", err)
	}
	if file.Version != 1 {
		return File{}, fmt.Errorf("unsupported registry profile version %d", file.Version)
	}
	if file.Registries == nil {
		file.Registries = map[string]Profile{}
	}
	return file, nil
}

func (s Store) Put(name string, profile Profile) error {
	if name == "" || strings.ContainsAny(name, " /\\\t\r\n") || profile.Address == "" || strings.ContainsAny(profile.Address, "\r\n") {
		return fmt.Errorf("registry name and address are invalid")
	}
	file, err := s.Load()
	if err != nil {
		return err
	}
	file.Registries[name] = profile
	return s.save(file)
}

func (s Store) Get(name string) (Profile, bool, error) {
	file, err := s.Load()
	if err != nil {
		return Profile{}, false, err
	}
	profile, exists := file.Registries[name]
	return profile, exists, nil
}

func (s Store) List() ([]string, map[string]Profile, error) {
	file, err := s.Load()
	if err != nil {
		return nil, nil, err
	}
	var names []string
	for name := range file.Registries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, file.Registries, nil
}

func (s Store) Remove(name string) (Profile, bool, error) {
	file, err := s.Load()
	if err != nil {
		return Profile{}, false, err
	}
	profile, exists := file.Registries[name]
	if !exists {
		return Profile{}, false, nil
	}
	delete(file.Registries, name)
	return profile, true, s.save(file)
}

func (s Store) save(file File) error {
	content, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".omurga-registries-*")
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
	return os.Rename(temporaryPath, s.Path)
}

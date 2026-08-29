package secret

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"filippo.io/age"

	"omurga/internal/host"
)

const storeVersion = 1

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Store struct {
	Version int               `json:"version"`
	Values  map[string]string `json:"values"`
}

type Manager struct {
	Paths host.Paths
}

type MaterializeSpec struct {
	Name string
	Mode fs.FileMode
	UID  int
	GID  int
}

func NewManager(paths host.Paths) Manager {
	return Manager{Paths: paths}
}

func (m Manager) IdentityPath() string {
	return filepath.Join(m.Paths.Keys, "identity.agekey")
}

func (m Manager) StorePath(project, environment string) (string, error) {
	if !namePattern.MatchString(project) {
		return "", fmt.Errorf("invalid project name %q", project)
	}
	if !namePattern.MatchString(environment) {
		return "", fmt.Errorf("invalid environment name %q", environment)
	}
	return filepath.Join(m.Paths.Secrets, project, environment+".age"), nil
}

func (m Manager) Set(project, environment, name string, value []byte) error {
	if err := validateName(name); err != nil {
		return err
	}
	store, _, err := m.load(project, environment, true)
	if err != nil {
		return err
	}
	store.Values[name] = base64.StdEncoding.EncodeToString(value)
	return m.save(project, environment, store)
}

func (m Manager) List(project, environment string) ([]string, error) {
	store, exists, err := m.load(project, environment, false)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	names := make([]string, 0, len(store.Values))
	for name := range store.Values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (m Manager) Remove(project, environment, name string) (bool, error) {
	if err := validateName(name); err != nil {
		return false, err
	}
	store, exists, err := m.load(project, environment, false)
	if err != nil || !exists {
		return false, err
	}
	if _, exists := store.Values[name]; !exists {
		return false, nil
	}
	delete(store.Values, name)
	if len(store.Values) == 0 {
		path, pathErr := m.StorePath(project, environment)
		if pathErr != nil {
			return false, pathErr
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("could not remove empty secret store: %w", err)
		}
		return true, nil
	}
	return true, m.save(project, environment, store)
}

func (m Manager) Materialize(project, environment, root string, specs []MaterializeSpec) (bool, error) {
	store, exists, err := m.load(project, environment, false)
	if err != nil || !exists {
		return exists, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return true, fmt.Errorf("could not create runtime secret directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return true, fmt.Errorf("could not secure runtime secret directory: %w", err)
	}
	required := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if err := validateName(spec.Name); err != nil {
			return true, err
		}
		encoded, ok := store.Values[spec.Name]
		if !ok {
			return true, fmt.Errorf("secret %s is missing from the encrypted store", spec.Name)
		}
		value, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return true, fmt.Errorf("secret %s has invalid encrypted store data: %w", spec.Name, err)
		}
		mode := spec.Mode
		if mode == 0 {
			mode = 0o400
		}
		path := filepath.Join(root, spec.Name)
		if err := atomicWrite(path, value, mode); err != nil {
			return true, fmt.Errorf("could not materialize secret %s: %w", spec.Name, err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chown(path, spec.UID, spec.GID); err != nil {
				return true, fmt.Errorf("could not set ownership for secret %s: %w", spec.Name, err)
			}
		}
		required[spec.Name] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return true, fmt.Errorf("could not inspect runtime secrets: %w", err)
	}
	for _, entry := range entries {
		if !required[entry.Name()] {
			path := filepath.Join(root, entry.Name())
			if entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
				if err := os.Remove(path); err != nil {
					return true, fmt.Errorf("could not remove stale runtime secret %s: %w", entry.Name(), err)
				}
			}
		}
	}
	return true, nil
}

func ParseMode(value string) (fs.FileMode, error) {
	if value == "" {
		return 0o400, nil
	}
	parsed, err := strconv.ParseUint(value, 8, 9)
	if err != nil || parsed > 0o777 {
		return 0, fmt.Errorf("secret mode %q must be an octal permission between 000 and 777", value)
	}
	return fs.FileMode(parsed), nil
}

func (m Manager) load(project, environment string, create bool) (Store, bool, error) {
	path, err := m.StorePath(project, environment)
	if err != nil {
		return Store{}, false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if create {
			return Store{Version: storeVersion, Values: map[string]string{}}, false, nil
		}
		return Store{}, false, nil
	}
	if err != nil {
		return Store{}, false, fmt.Errorf("could not read encrypted secret store: %w", err)
	}
	identity, err := m.readIdentity()
	if err != nil {
		return Store{}, false, err
	}
	reader, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		return Store{}, false, fmt.Errorf("could not decrypt secret store: %w", err)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return Store{}, false, fmt.Errorf("could not read decrypted secret store: %w", err)
	}
	var store Store
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store); err != nil {
		return Store{}, false, fmt.Errorf("could not decode secret store: %w", err)
	}
	if store.Version != storeVersion || store.Values == nil {
		return Store{}, false, fmt.Errorf("unsupported secret store version %d", store.Version)
	}
	return store, true, nil
}

func (m Manager) save(project, environment string, store Store) error {
	identity, err := m.ensureIdentity()
	if err != nil {
		return err
	}
	plaintext, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("could not encode secret store: %w", err)
	}
	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, identity.Recipient())
	if err != nil {
		return fmt.Errorf("could not initialize secret encryption: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return fmt.Errorf("could not encrypt secret store: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("could not finalize secret encryption: %w", err)
	}
	path, err := m.StorePath(project, environment)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create secret store directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not secure secret store directory: %w", err)
	}
	if err := atomicWrite(path, encrypted.Bytes(), 0o600); err != nil {
		return fmt.Errorf("could not write encrypted secret store: %w", err)
	}
	return nil
}

func (m Manager) ensureIdentity() (*age.X25519Identity, error) {
	identity, err := m.readIdentity()
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	identity, err = age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("could not generate age identity: %w", err)
	}
	if err := os.MkdirAll(m.Paths.Keys, 0o700); err != nil {
		return nil, fmt.Errorf("could not create key directory: %w", err)
	}
	if err := os.Chmod(m.Paths.Keys, 0o700); err != nil {
		return nil, fmt.Errorf("could not secure key directory: %w", err)
	}
	content := []byte("# created by Omurga; keep this file private\n" + identity.String() + "\n")
	if err := atomicWrite(m.IdentityPath(), content, 0o600); err != nil {
		return nil, fmt.Errorf("could not write age identity: %w", err)
	}
	return identity, nil
}

func (m Manager) readIdentity() (*age.X25519Identity, error) {
	data, err := os.ReadFile(m.IdentityPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("could not read age identity: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		identity, err := age.ParseX25519Identity(line)
		if err != nil {
			return nil, fmt.Errorf("could not parse age identity: %w", err)
		}
		return identity, nil
	}
	return nil, fmt.Errorf("age identity file does not contain a private key")
}

func validateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("secret name %q must contain only lowercase letters, digits, and hyphens", name)
	}
	return nil
}

func atomicWrite(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".omurga-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(temporaryPath, path)
}

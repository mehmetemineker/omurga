package host

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const defaultConfig = `version: 1

gateway:
  portRange:
    start: 20000
    end: 29999
`

type InitAction struct {
	Type    string      `json:"type"`
	Path    string      `json:"path"`
	Mode    fs.FileMode `json:"-"`
	ModeOct string      `json:"mode"`
	Changed bool        `json:"changed"`
	Planned bool        `json:"planned"`
}

type InitResult struct {
	OS       OSRelease    `json:"os"`
	Platform PlatformInfo `json:"platform"`
	Actions  []InitAction `json:"actions"`
	DryRun   bool         `json:"dryRun"`
}

func Initialize(paths Paths, dryRun bool) (InitResult, error) {
	release, _, platform, err := DetectPlatform(paths.OSRelease)
	if err != nil {
		return InitResult{}, err
	}

	result := InitResult{OS: release, Platform: platform, DryRun: dryRun}
	for _, directory := range paths.ManagedDirectories() {
		action := InitAction{
			Type:    "directory",
			Path:    directory.Path,
			Mode:    directory.Mode,
			ModeOct: fmt.Sprintf("%04o", directory.Mode.Perm()),
			Planned: dryRun,
		}

		info, statErr := os.Stat(directory.Path)
		switch {
		case statErr == nil && !info.IsDir():
			return InitResult{}, fmt.Errorf("managed path exists but is not a directory: %s", directory.Path)
		case statErr == nil:
			action.Changed = info.Mode().Perm() != directory.Mode.Perm()
		case os.IsNotExist(statErr):
			action.Changed = true
		default:
			return InitResult{}, fmt.Errorf("could not inspect directory %s: %w", directory.Path, statErr)
		}

		if !dryRun && action.Changed {
			if err := os.MkdirAll(directory.Path, directory.Mode); err != nil {
				return InitResult{}, fmt.Errorf("could not create directory %s: %w", directory.Path, err)
			}
			if err := os.Chmod(directory.Path, directory.Mode); err != nil {
				return InitResult{}, fmt.Errorf("could not set permissions on %s: %w", directory.Path, err)
			}
		}
		result.Actions = append(result.Actions, action)
	}

	configAction, err := initializeConfig(paths.ConfigFile, dryRun)
	if err != nil {
		return InitResult{}, err
	}
	result.Actions = append(result.Actions, configAction)
	return result, nil
}

func initializeConfig(path string, dryRun bool) (InitAction, error) {
	action := InitAction{
		Type:    "file",
		Path:    path,
		Mode:    0o640,
		ModeOct: "0640",
		Planned: dryRun,
	}

	if _, err := os.Stat(path); err == nil {
		return action, nil
	} else if !os.IsNotExist(err) {
		return InitAction{}, fmt.Errorf("could not inspect config file: %w", err)
	}
	action.Changed = true
	if dryRun {
		return action, nil
	}

	if err := writeFileAtomically(path, []byte(defaultConfig), action.Mode); err != nil {
		return InitAction{}, fmt.Errorf("could not create config file: %w", err)
	}
	return action, nil
}

func writeFileAtomically(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".omurga-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
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
	return os.Rename(temporaryPath, path)
}

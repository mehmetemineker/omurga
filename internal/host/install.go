package host

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	dockerKeyURL   = "https://download.docker.com/linux/ubuntu/gpg"
	caddyKeyURL    = "https://dl.cloudsmith.io/public/caddy/stable/gpg.key"
	caddySourceURL = "https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt"
)

type InstallOptions struct {
	DryRun                 bool
	ReplaceDockerConflicts bool
}

type InstallStep struct {
	Name    string   `json:"name"`
	Command []string `json:"command,omitempty"`
	Path    string   `json:"path,omitempty"`
	Status  string   `json:"status"`
}

type InstallResult struct {
	Component        string        `json:"component"`
	AlreadyInstalled bool          `json:"alreadyInstalled"`
	DryRun           bool          `json:"dryRun"`
	Steps            []InstallStep `json:"steps"`
}

type Installer struct {
	Paths      Paths
	Runner     Runner
	Downloader Downloader
}

func NewInstaller(paths Paths) Installer {
	return Installer{
		Paths:      paths,
		Runner:     ExecRunner{},
		Downloader: HTTPDownloader{},
	}
}

func (i Installer) validate() error {
	if i.Runner == nil {
		return fmt.Errorf("installer runner is required")
	}
	if i.Downloader == nil {
		return fmt.Errorf("installer downloader is required")
	}
	return nil
}

func (i Installer) runStep(ctx context.Context, result *InstallResult, options InstallOptions, name, command string, args ...string) error {
	step := InstallStep{Name: name, Command: append([]string{command}, args...), Status: "planned"}
	if !options.DryRun {
		if _, err := i.Runner.Run(ctx, command, args...); err != nil {
			step.Status = "failed"
			result.Steps = append(result.Steps, step)
			return err
		}
		step.Status = "completed"
	}
	result.Steps = append(result.Steps, step)
	return nil
}

func (i Installer) fileStep(result *InstallResult, options InstallOptions, name, path string, changed bool) {
	status := "unchanged"
	if changed && options.DryRun {
		status = "planned"
	} else if changed {
		status = "completed"
	}
	result.Steps = append(result.Steps, InstallStep{Name: name, Path: path, Status: status})
}

func ensureDirectory(path string, mode fs.FileMode, dryRun bool) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("path exists but is not a directory: %s", path)
		}
		changed := info.Mode().Perm() != mode.Perm()
		if changed && !dryRun {
			if err := os.Chmod(path, mode); err != nil {
				return false, err
			}
		}
		return changed, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if !dryRun {
		if err := os.MkdirAll(path, mode); err != nil {
			return false, err
		}
		if err := os.Chmod(path, mode); err != nil {
			return false, err
		}
	}
	return true, nil
}

func ensureFile(path string, data []byte, mode fs.FileMode, dryRun bool) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return false, statErr
		}
		changed := !bytes.Equal(existing, data) || info.Mode().Perm() != mode.Perm()
		if changed && !dryRun {
			if bytes.Equal(existing, data) {
				return true, os.Chmod(path, mode)
			}
			return true, writeFileAtomically(path, data, mode)
		}
		return changed, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if dryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, writeFileAtomically(path, data, mode)
}

func packageInstalled(ctx context.Context, runner Runner, name string) bool {
	output, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${Status}", name)
	return err == nil && strings.Contains(output, "install ok installed")
}

func commandHealthy(ctx context.Context, runner Runner, name string, args ...string) bool {
	if _, err := runner.LookPath(name); err != nil {
		return false
	}
	_, err := runner.Run(ctx, name, args...)
	return err == nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

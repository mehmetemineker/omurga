package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (i Installer) configureRepositories(ctx context.Context, result *InstallResult, options InstallOptions, files []RepositoryFile) error {
	preparedDirectories := map[string]bool{}
	for _, file := range files {
		target, err := i.repositoryTarget(file.Path)
		if err != nil {
			return err
		}
		directory := filepath.Dir(target)
		if !preparedDirectories[directory] {
			changed, err := ensureDirectory(directory, 0o755, options.DryRun)
			if err != nil {
				return fmt.Errorf("could not prepare repository directory: %w", err)
			}
			i.fileStep(result, options, "prepare repository directory", directory, changed)
			preparedDirectories[directory] = true
		}
		name := "install " + strings.ReplaceAll(file.Path, "-", " ")
		if options.DryRun && file.URL != "" {
			i.fileStep(result, options, name, target, true)
			continue
		}
		data := append([]byte(nil), file.Content...)
		if file.URL != "" {
			task := i.Progress.Start("Download " + filepath.Base(file.Path))
			data, err = i.Downloader.Download(ctx, file.URL)
			if err != nil {
				task.Fail(err)
				return fmt.Errorf("could not download %s: %w", file.Path, err)
			}
			task.Complete()
		}
		if len(data) == 0 {
			return fmt.Errorf("repository artifact %s is empty", file.Path)
		}
		if file.RequiredFragment != "" && !strings.Contains(string(data), file.RequiredFragment) {
			return fmt.Errorf("repository artifact %s did not contain the required repository reference", file.Path)
		}
		if file.Dearmor {
			data, err = i.dearmorRepositoryKey(ctx, result, options, file.Path, data)
			if err != nil {
				return err
			}
		}
		mode := file.Mode
		if mode == 0 {
			mode = 0o644
		}
		changed, err := ensureFile(target, data, mode, options.DryRun)
		if err != nil {
			return fmt.Errorf("could not install %s: %w", file.Path, err)
		}
		i.fileStep(result, options, name, target, changed)
	}
	return nil
}

func (i Installer) repositoryTarget(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("repository artifact path is required")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || strings.Contains(name, `\`) || strings.Contains(name, ":") {
		return "", fmt.Errorf("repository artifact path must be a portable relative path: %q", name)
	}
	relative := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repository artifact path must stay within the host root: %q", name)
	}
	root := filepath.Clean(i.Paths.Root)
	if root == "." || root == "" {
		return "", fmt.Errorf("host root is required for repository artifacts")
	}
	target := filepath.Join(root, relative)
	resolved, err := filepath.Rel(root, target)
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repository artifact path must stay within the host root: %q", name)
	}
	return target, nil
}

func (i Installer) dearmorRepositoryKey(ctx context.Context, result *InstallResult, options InstallOptions, name string, data []byte) ([]byte, error) {
	rawFile, err := os.CreateTemp("", "omurga-repository-key-*.asc")
	if err != nil {
		return nil, err
	}
	rawPath := rawFile.Name()
	defer os.Remove(rawPath)
	if _, err := rawFile.Write(data); err != nil {
		rawFile.Close()
		return nil, err
	}
	if err := rawFile.Close(); err != nil {
		return nil, err
	}
	binaryFile, err := os.CreateTemp("", "omurga-repository-key-*.gpg")
	if err != nil {
		return nil, err
	}
	binaryPath := binaryFile.Name()
	if err := binaryFile.Close(); err != nil {
		return nil, err
	}
	defer os.Remove(binaryPath)
	if err := i.runStep(ctx, result, options, "dearmor "+strings.ReplaceAll(name, "-", " "), "gpg", "--batch", "--yes", "--dearmor", "--output", binaryPath, rawPath); err != nil {
		return nil, err
	}
	dearmored, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("could not read dearmored repository key: %w", err)
	}
	if len(dearmored) == 0 {
		return nil, fmt.Errorf("gpg produced an empty repository key")
	}
	return dearmored, nil
}

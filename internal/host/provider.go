package host

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

const SupportOfficial = "official"

type PlatformInfo struct {
	Distribution   string   `json:"distribution"`
	Family         string   `json:"family"`
	Version        string   `json:"version"`
	Codename       string   `json:"codename,omitempty"`
	PackageManager string   `json:"packageManager"`
	ServiceManager string   `json:"serviceManager"`
	SupportLevel   string   `json:"supportLevel"`
	Supported      []string `json:"supportedVersions"`
}

type RepositoryFile struct {
	URL              string
	Path             string
	Content          []byte
	Mode             fs.FileMode
	Dearmor          bool
	RequiredFragment string
}

type ComponentSpec struct {
	Repository    []RepositoryFile
	Prerequisites []string
	Conflicts     []string
	Packages      []string
}

type PackageManager interface {
	Name() string
	VersionCommand() PackageCommand
	RefreshCommand() PackageCommand
	UpgradeCommand(full, simulate bool) PackageCommand
	InstallCommand(packages ...string) PackageCommand
	RemoveCommand(packages ...string) PackageCommand
	IsInstalled(ctx context.Context, runner Runner, name string) bool
	Architecture(ctx context.Context, runner Runner) (string, error)
}

type ServiceManager interface {
	Name() string
	VersionCommand() PackageCommand
	EnableNowCommand(service string) PackageCommand
	RestartCommand(service string) PackageCommand
	DisableNowCommand(service string) PackageCommand
	IsActiveCommand(service string) PackageCommand
	ReloadCommand(service string) PackageCommand
	DaemonReloadCommand() PackageCommand
	ListTimersCommand(pattern string) PackageCommand
}

type DistributionProvider interface {
	ID() string
	Family() string
	SupportLevel() string
	SupportedVersions() []string
	Validate(OSRelease) error
	PackageManager() PackageManager
	ServiceManager() ServiceManager
	DockerSpec(OSRelease, string) (ComponentSpec, error)
	CaddySpec(OSRelease) (ComponentSpec, error)
}

type ProviderRegistry struct {
	providers map[string]DistributionProvider
}

func NewProviderRegistry(providers ...DistributionProvider) ProviderRegistry {
	registry := ProviderRegistry{providers: make(map[string]DistributionProvider, len(providers))}
	for _, provider := range providers {
		if provider != nil {
			registry.providers[provider.ID()] = provider
		}
	}
	return registry
}

func DefaultProviderRegistry() ProviderRegistry {
	return NewProviderRegistry(NewUbuntuProvider(), NewDebianProvider())
}

func NewUbuntuProvider() DistributionProvider {
	return newAPTSystemdProvider("ubuntu", []string{"22.04", "24.04", "26.04"})
}

func NewDebianProvider() DistributionProvider {
	return newAPTSystemdProvider("debian", []string{"11", "12", "13"})
}

func (r ProviderRegistry) Resolve(release OSRelease) (DistributionProvider, error) {
	provider, exists := r.providers[release.ID]
	if !exists {
		ids := make([]string, 0, len(r.providers))
		for id := range r.providers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("unsupported Linux distribution %q: supported distributions are %s", release.ID, strings.Join(ids, " and "))
	}
	if err := provider.Validate(release); err != nil {
		return nil, err
	}
	return provider, nil
}

func DetectPlatform(path string) (OSRelease, DistributionProvider, PlatformInfo, error) {
	release, err := LoadOSRelease(path)
	if err != nil {
		return OSRelease{}, nil, PlatformInfo{}, fmt.Errorf("could not read os-release: %w", err)
	}
	provider, err := DefaultProviderRegistry().Resolve(release)
	if err != nil {
		return release, nil, PlatformInfo{}, err
	}
	return release, provider, ProviderInfo(provider, release), nil
}

func DetectServiceManager(path string) (ServiceManager, error) {
	_, provider, _, err := DetectPlatform(path)
	if err != nil {
		return nil, err
	}
	return provider.ServiceManager(), nil
}

func NewSystemdServiceManager() ServiceManager {
	return systemdServiceManager{}
}

func ProviderInfo(provider DistributionProvider, release OSRelease) PlatformInfo {
	return PlatformInfo{
		Distribution: provider.ID(), Family: provider.Family(), Version: release.VersionID,
		Codename: release.Codename, PackageManager: provider.PackageManager().Name(),
		ServiceManager: provider.ServiceManager().Name(), SupportLevel: provider.SupportLevel(),
		Supported: append([]string(nil), provider.SupportedVersions()...),
	}
}

type aptSystemdProvider struct {
	id       string
	versions []string
}

func newAPTSystemdProvider(id string, versions []string) DistributionProvider {
	return aptSystemdProvider{id: id, versions: append([]string(nil), versions...)}
}

func (p aptSystemdProvider) ID() string                     { return p.id }
func (p aptSystemdProvider) Family() string                 { return "debian" }
func (p aptSystemdProvider) SupportLevel() string           { return SupportOfficial }
func (p aptSystemdProvider) SupportedVersions() []string    { return append([]string(nil), p.versions...) }
func (p aptSystemdProvider) PackageManager() PackageManager { return aptPackageManager{} }
func (p aptSystemdProvider) ServiceManager() ServiceManager { return systemdServiceManager{} }

func (p aptSystemdProvider) Validate(release OSRelease) error {
	if release.ID != p.id {
		return fmt.Errorf("provider %s cannot manage distribution %s", p.id, release.ID)
	}
	for _, version := range p.versions {
		if release.VersionID == version {
			if release.Codename == "" {
				return fmt.Errorf("%s VERSION_CODENAME is required", distributionLabel(p.id))
			}
			return nil
		}
	}
	return fmt.Errorf("unsupported %s version %q: supported versions are %s", distributionLabel(p.id), release.VersionID, strings.Join(p.versions, ", "))
}

func distributionLabel(id string) string {
	if id == "" {
		return "Linux"
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func (p aptSystemdProvider) DockerSpec(release OSRelease, architecture string) (ComponentSpec, error) {
	if err := p.Validate(release); err != nil {
		return ComponentSpec{}, err
	}
	if strings.TrimSpace(architecture) == "" {
		return ComponentSpec{}, fmt.Errorf("package architecture is required")
	}
	baseURL := "https://download.docker.com/linux/" + p.id
	source := fmt.Sprintf("Types: deb\nURIs: %s\nSuites: %s\nComponents: stable\nArchitectures: %s\nSigned-By: /etc/apt/keyrings/docker.asc\n", baseURL, release.Codename, architecture)
	return ComponentSpec{
		Repository: []RepositoryFile{
			{URL: baseURL + "/gpg", Path: "etc/apt/keyrings/docker.asc", Mode: 0o644},
			{Content: []byte(source), Path: "etc/apt/sources.list.d/docker.sources", Mode: 0o644},
		},
		Prerequisites: []string{"ca-certificates", "curl"},
		Conflicts:     []string{"docker.io", "docker-compose", "docker-compose-v2", "docker-doc", "docker-buildx", "podman-docker", "containerd", "runc"},
		Packages:      []string{"docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"},
	}, nil
}

func (p aptSystemdProvider) CaddySpec(release OSRelease) (ComponentSpec, error) {
	if err := p.Validate(release); err != nil {
		return ComponentSpec{}, err
	}
	return ComponentSpec{
		Repository: []RepositoryFile{
			{URL: caddyKeyURL, Path: "usr/share/keyrings/caddy-stable-archive-keyring.gpg", Mode: 0o644, Dearmor: true},
			{URL: caddySourceURL, Path: "etc/apt/sources.list.d/caddy-stable.list", Mode: 0o644, RequiredFragment: "dl.cloudsmith.io/public/caddy/stable"},
		},
		Prerequisites: []string{"debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl", "gnupg"},
		Packages:      []string{"caddy"},
	}, nil
}

type aptPackageManager struct{}

func (aptPackageManager) Name() string { return "apt" }
func (aptPackageManager) VersionCommand() PackageCommand {
	return PackageCommand{Name: "apt-get", Args: []string{"--version"}}
}
func (aptPackageManager) RefreshCommand() PackageCommand {
	return PackageCommand{Name: "apt-get", Args: []string{"update"}}
}
func (aptPackageManager) UpgradeCommand(full, simulate bool) PackageCommand {
	operation := "upgrade"
	if full {
		operation = "full-upgrade"
	}
	args := []string{"-y", "-o", "Dpkg::Options::=--force-confold", operation}
	if simulate {
		args = []string{"-s", "-o", "Dpkg::Options::=--force-confold", operation}
	}
	return PackageCommand{Name: "apt-get", Args: args}
}
func (aptPackageManager) InstallCommand(packages ...string) PackageCommand {
	return PackageCommand{Name: "apt-get", Args: append([]string{"install", "-y"}, packages...)}
}
func (aptPackageManager) RemoveCommand(packages ...string) PackageCommand {
	return PackageCommand{Name: "apt-get", Args: append([]string{"remove", "-y"}, packages...)}
}
func (aptPackageManager) IsInstalled(ctx context.Context, runner Runner, name string) bool {
	output, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${Status}", name)
	return err == nil && strings.Contains(output, "install ok installed")
}
func (aptPackageManager) Architecture(ctx context.Context, runner Runner) (string, error) {
	output, err := runner.Run(ctx, "dpkg", "--print-architecture")
	if err != nil {
		return "", fmt.Errorf("could not determine the Debian architecture: %w", err)
	}
	architecture := strings.TrimSpace(output)
	if architecture == "" {
		return "", fmt.Errorf("dpkg returned an empty architecture")
	}
	return architecture, nil
}

type systemdServiceManager struct{}

func (systemdServiceManager) Name() string { return "systemd" }
func (systemdServiceManager) VersionCommand() PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"--version"}}
}
func (systemdServiceManager) EnableNowCommand(service string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"enable", "--now", service}}
}
func (systemdServiceManager) RestartCommand(service string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"restart", service}}
}
func (systemdServiceManager) DisableNowCommand(service string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"disable", "--now", service}}
}
func (systemdServiceManager) IsActiveCommand(service string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"is-active", "--quiet", service}}
}
func (systemdServiceManager) ReloadCommand(service string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"reload", service}}
}
func (systemdServiceManager) DaemonReloadCommand() PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"daemon-reload"}}
}
func (systemdServiceManager) ListTimersCommand(pattern string) PackageCommand {
	return PackageCommand{Name: "systemctl", Args: []string{"list-timers", "--all", "--no-legend", pattern}}
}

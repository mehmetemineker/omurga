package host

import (
	"io/fs"
	"path/filepath"
	"strings"
)

type Paths struct {
	Root           string
	OSRelease      string
	ConfigRoot     string
	ConfigFile     string
	AlertConfig    string
	ProjectsConfig string
	Secrets        string
	Keys           string
	CaddyProjects  string
	StateRoot      string
	StateDB        string
	ProjectsState  string
	SharedServices string
	BackupStaging  string
	BackupConfig   string
	SystemdUnits   string
	LogRoot        string
	RuntimeRoot    string
	RuntimeSecrets string
	RebootRequired string
	APTKeyrings    string
	APTSources     string
	DockerKey      string
	DockerSource   string
	CaddyKey       string
	CaddySource    string
	CaddyFile      string
}

type Directory struct {
	Path string
	Mode fs.FileMode
}

func DefaultPaths(root string) Paths {
	if root == "" {
		root = string(filepath.Separator)
	}
	join := func(path string) string {
		path = strings.TrimLeft(path, `/\`)
		return filepath.Join(root, filepath.FromSlash(path))
	}

	return Paths{
		Root:           root,
		OSRelease:      join("etc/os-release"),
		ConfigRoot:     join("etc/omurga"),
		ConfigFile:     join("etc/omurga/config.yaml"),
		AlertConfig:    join("etc/omurga/alerts.yaml"),
		ProjectsConfig: join("etc/omurga/projects"),
		Secrets:        join("etc/omurga/secrets"),
		Keys:           join("etc/omurga/keys"),
		CaddyProjects:  join("etc/caddy"),
		StateRoot:      join("var/lib/omurga"),
		StateDB:        join("var/lib/omurga/state.db"),
		ProjectsState:  join("var/lib/omurga/projects"),
		SharedServices: join("var/lib/omurga/services"),
		BackupStaging:  join("var/backups/omurga/staging"),
		BackupConfig:   join("etc/omurga/backup"),
		SystemdUnits:   join("etc/systemd/system"),
		LogRoot:        join("var/log/omurga"),
		RuntimeRoot:    join("run/omurga"),
		RuntimeSecrets: join("run/omurga/secrets"),
		RebootRequired: join("var/run/reboot-required"),
		APTKeyrings:    join("etc/apt/keyrings"),
		APTSources:     join("etc/apt/sources.list.d"),
		DockerKey:      join("etc/apt/keyrings/docker.asc"),
		DockerSource:   join("etc/apt/sources.list.d/docker.sources"),
		CaddyKey:       join("usr/share/keyrings/caddy-stable-archive-keyring.gpg"),
		CaddySource:    join("etc/apt/sources.list.d/caddy-stable.list"),
		CaddyFile:      join("etc/caddy/Caddyfile"),
	}
}

func (p Paths) ManagedDirectories() []Directory {
	return []Directory{
		{Path: p.ConfigRoot, Mode: 0o750},
		{Path: p.ProjectsConfig, Mode: 0o750},
		{Path: p.Secrets, Mode: 0o700},
		{Path: p.Keys, Mode: 0o700},
		{Path: p.CaddyProjects, Mode: 0o755},
		{Path: p.StateRoot, Mode: 0o750},
		{Path: p.ProjectsState, Mode: 0o750},
		{Path: p.SharedServices, Mode: 0o750},
		{Path: p.BackupStaging, Mode: 0o700},
		{Path: p.BackupConfig, Mode: 0o700},
		{Path: p.LogRoot, Mode: 0o750},
		{Path: p.RuntimeRoot, Mode: 0o700},
		{Path: p.RuntimeSecrets, Mode: 0o700},
	}
}

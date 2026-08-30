package manifest

type Project struct {
	Version      int                   `yaml:"version" json:"version"`
	Name         string                `yaml:"name" json:"name"`
	Host         string                `yaml:"host,omitempty" json:"host,omitempty"`
	Services     map[string]Service    `yaml:"services" json:"services"`
	Gateway      Gateway               `yaml:"gateway,omitempty" json:"gateway,omitempty"`
	Dependencies map[string]Dependency `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Backup       Backup                `yaml:"backup,omitempty" json:"backup,omitempty"`
	Alerts       Alerts                `yaml:"alerts,omitempty" json:"alerts,omitempty"`
}

type Service struct {
	Image       string            `yaml:"image" json:"image"`
	PullPolicy  string            `yaml:"pullPolicy,omitempty" json:"pullPolicy,omitempty"`
	Restart     string            `yaml:"restart,omitempty" json:"restart,omitempty"`
	Command     []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Expose      []int             `yaml:"expose,omitempty" json:"expose,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Secrets     []SecretMount     `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Volumes     []VolumeMount     `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Resources   Resources         `yaml:"resources,omitempty" json:"resources,omitempty"`
	Logging     Logging           `yaml:"logging,omitempty" json:"logging,omitempty"`
	Healthcheck Healthcheck       `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`
}

type SecretMount struct {
	Name   string `yaml:"name" json:"name"`
	Target string `yaml:"target" json:"target"`
	Mode   string `yaml:"mode,omitempty" json:"mode,omitempty"`
	UID    int    `yaml:"uid,omitempty" json:"uid,omitempty"`
	GID    int    `yaml:"gid,omitempty" json:"gid,omitempty"`
}

type VolumeMount struct {
	Name   string `yaml:"name" json:"name"`
	Target string `yaml:"target" json:"target"`
}

type Resources struct {
	CPUs   string `yaml:"cpus,omitempty" json:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
	PIDs   int    `yaml:"pids,omitempty" json:"pids,omitempty"`
}

type Logging struct {
	Driver   string `yaml:"driver,omitempty" json:"driver,omitempty"`
	MaxSize  string `yaml:"maxSize,omitempty" json:"maxSize,omitempty"`
	MaxFiles int    `yaml:"maxFiles,omitempty" json:"maxFiles,omitempty"`
}

type Healthcheck struct {
	Command     []string `yaml:"command,omitempty" json:"command,omitempty"`
	Interval    string   `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty" json:"retries,omitempty"`
	StartPeriod string   `yaml:"startPeriod,omitempty" json:"startPeriod,omitempty"`
}

type Gateway struct {
	Email  string  `yaml:"email,omitempty" json:"email,omitempty"`
	Routes []Route `yaml:"routes,omitempty" json:"routes,omitempty"`
}

type Route struct {
	Domain  string `yaml:"domain" json:"domain"`
	Service string `yaml:"service" json:"service"`
	Port    int    `yaml:"port" json:"port"`
	HTTPS   *bool  `yaml:"https,omitempty" json:"https,omitempty"`
}

type Dependency struct {
	Type           string `yaml:"type" json:"type"`
	Version        string `yaml:"version" json:"version"`
	Mode           string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Instance       string `yaml:"instance,omitempty" json:"instance,omitempty"`
	Database       string `yaml:"database,omitempty" json:"database,omitempty"`
	User           string `yaml:"user,omitempty" json:"user,omitempty"`
	PasswordSecret string `yaml:"passwordSecret,omitempty" json:"passwordSecret,omitempty"`
	Persistence    string `yaml:"persistence,omitempty" json:"persistence,omitempty"`
	MaxMemory      string `yaml:"maxMemory,omitempty" json:"maxMemory,omitempty"`
	EvictionPolicy string `yaml:"evictionPolicy,omitempty" json:"evictionPolicy,omitempty"`
}

type Backup struct {
	Enabled     bool            `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Schedule    string          `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Destination string          `yaml:"destination,omitempty" json:"destination,omitempty"`
	Retention   Retention       `yaml:"retention,omitempty" json:"retention,omitempty"`
	Include     BackupSelection `yaml:"include,omitempty" json:"include,omitempty"`
}

type Retention struct {
	Daily   int `yaml:"daily,omitempty" json:"daily,omitempty"`
	Weekly  int `yaml:"weekly,omitempty" json:"weekly,omitempty"`
	Monthly int `yaml:"monthly,omitempty" json:"monthly,omitempty"`
}

type BackupSelection struct {
	Volumes  []string `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Postgres []string `yaml:"postgres,omitempty" json:"postgres,omitempty"`
	Redis    []string `yaml:"redis,omitempty" json:"redis,omitempty"`
}

type Alerts struct {
	On []string `yaml:"on,omitempty" json:"on,omitempty"`
}

type LoadedProject struct {
	Project     Project
	Path        string
	Environment string
}

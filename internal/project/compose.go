package project

type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Secrets  map[string]ComposeSecret  `yaml:"secrets,omitempty"`
}

type ComposeService struct {
	Image       string                       `yaml:"image"`
	PullPolicy  string                       `yaml:"pull_policy,omitempty"`
	Restart     string                       `yaml:"restart,omitempty"`
	Command     []string                     `yaml:"command,omitempty"`
	Expose      []int                        `yaml:"expose,omitempty"`
	Environment map[string]string            `yaml:"environment,omitempty"`
	Secrets     []ComposeServiceSecret       `yaml:"secrets,omitempty"`
	Volumes     []ComposeVolume              `yaml:"volumes,omitempty"`
	Ports       []ComposePort                `yaml:"ports,omitempty"`
	CPUs        string                       `yaml:"cpus,omitempty"`
	Memory      string                       `yaml:"mem_limit,omitempty"`
	PIDs        int                          `yaml:"pids_limit,omitempty"`
	Logging     *ComposeLogging              `yaml:"logging,omitempty"`
	Healthcheck *ComposeHealthcheck          `yaml:"healthcheck,omitempty"`
	DependsOn   map[string]ComposeDependency `yaml:"depends_on,omitempty"`
	Labels      map[string]string            `yaml:"labels,omitempty"`
}

type ComposeSecret struct {
	File string `yaml:"file"`
}

type ComposeServiceSecret struct {
	Source string `yaml:"source"`
	Target string `yaml:"target,omitempty"`
}

type ComposeVolume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
}

type ComposePort struct {
	Name      string `yaml:"name,omitempty"`
	Target    int    `yaml:"target"`
	Published string `yaml:"published"`
	HostIP    string `yaml:"host_ip"`
	Protocol  string `yaml:"protocol,omitempty"`
}

type ComposeLogging struct {
	Driver  string            `yaml:"driver"`
	Options map[string]string `yaml:"options,omitempty"`
}

type ComposeHealthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

type ComposeDependency struct {
	Condition string `yaml:"condition"`
	Restart   bool   `yaml:"restart,omitempty"`
}

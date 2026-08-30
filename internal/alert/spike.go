package alert

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"omurga/internal/host"
)

type ResourceSpikeConfig struct {
	Enabled                        *bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	BaselineSamples                int     `yaml:"baselineSamples,omitempty" json:"baselineSamples,omitempty"`
	ConsecutiveSamples             int     `yaml:"consecutiveSamples,omitempty" json:"consecutiveSamples,omitempty"`
	CooldownMinutes                int     `yaml:"cooldownMinutes,omitempty" json:"cooldownMinutes,omitempty"`
	CPUIncreasePercent             float64 `yaml:"cpuIncreasePercent,omitempty" json:"cpuIncreasePercent,omitempty"`
	MemoryIncreasePercent          float64 `yaml:"memoryIncreasePercent,omitempty" json:"memoryIncreasePercent,omitempty"`
	DiskIncreasePercent            float64 `yaml:"diskIncreasePercent,omitempty" json:"diskIncreasePercent,omitempty"`
	ContainerCPUIncreasePercent    float64 `yaml:"containerCPUIncreasePercent,omitempty" json:"containerCPUIncreasePercent,omitempty"`
	ContainerMemoryIncreasePercent float64 `yaml:"containerMemoryIncreasePercent,omitempty" json:"containerMemoryIncreasePercent,omitempty"`
	CPUMinimumPercent              float64 `yaml:"cpuMinimumPercent,omitempty" json:"cpuMinimumPercent,omitempty"`
	MemoryMinimumPercent           float64 `yaml:"memoryMinimumPercent,omitempty" json:"memoryMinimumPercent,omitempty"`
	DiskMinimumPercent             float64 `yaml:"diskMinimumPercent,omitempty" json:"diskMinimumPercent,omitempty"`
}

type ResourceSample struct {
	At      time.Time          `json:"at"`
	Metrics map[string]float64 `json:"metrics"`
}

type ResourceSpikeState struct {
	Samples   []ResourceSample     `json:"samples,omitempty"`
	Baselines map[string][]float64 `json:"baselines,omitempty"`
	Breaches  map[string]int       `json:"breaches,omitempty"`
	Active    map[string]time.Time `json:"active,omitempty"`
}

type ResourceSpikeDelta struct {
	NewIssues []MonitorIssue
	Resolved  []string
	NextState ResourceSpikeState
}

func withResourceSpikeDefaults(config ResourceSpikeConfig) ResourceSpikeConfig {
	if config.Enabled == nil {
		enabled := true
		config.Enabled = &enabled
	}
	if config.BaselineSamples == 0 {
		config.BaselineSamples = 3
	}
	if config.ConsecutiveSamples == 0 {
		config.ConsecutiveSamples = 2
	}
	if config.CooldownMinutes == 0 {
		config.CooldownMinutes = 30
	}
	if config.CPUIncreasePercent == 0 {
		config.CPUIncreasePercent = 30
	}
	if config.MemoryIncreasePercent == 0 {
		config.MemoryIncreasePercent = 20
	}
	if config.DiskIncreasePercent == 0 {
		config.DiskIncreasePercent = 5
	}
	if config.ContainerCPUIncreasePercent == 0 {
		config.ContainerCPUIncreasePercent = 30
	}
	if config.ContainerMemoryIncreasePercent == 0 {
		config.ContainerMemoryIncreasePercent = 20
	}
	if config.CPUMinimumPercent == 0 {
		config.CPUMinimumPercent = 70
	}
	if config.MemoryMinimumPercent == 0 {
		config.MemoryMinimumPercent = 70
	}
	if config.DiskMinimumPercent == 0 {
		config.DiskMinimumPercent = 80
	}
	return config
}

func ValidateResourceSpikeConfig(config ResourceSpikeConfig) error {
	config = withResourceSpikeDefaults(config)
	if config.BaselineSamples < 1 || config.BaselineSamples > 100 {
		return fmt.Errorf("monitor spike baselineSamples must be between 1 and 100")
	}
	if config.ConsecutiveSamples < 1 || config.ConsecutiveSamples > 20 {
		return fmt.Errorf("monitor spike consecutiveSamples must be between 1 and 20")
	}
	if config.CooldownMinutes < 1 || config.CooldownMinutes > 10080 {
		return fmt.Errorf("monitor spike cooldownMinutes must be between 1 and 10080")
	}
	for name, value := range map[string]float64{
		"cpuIncreasePercent":             config.CPUIncreasePercent,
		"memoryIncreasePercent":          config.MemoryIncreasePercent,
		"diskIncreasePercent":            config.DiskIncreasePercent,
		"containerCPUIncreasePercent":    config.ContainerCPUIncreasePercent,
		"containerMemoryIncreasePercent": config.ContainerMemoryIncreasePercent,
	} {
		if value <= 0 || value > 100 {
			return fmt.Errorf("monitor spike %s must be between 0 and 100", name)
		}
	}
	for name, value := range map[string]float64{
		"cpuMinimumPercent":    config.CPUMinimumPercent,
		"memoryMinimumPercent": config.MemoryMinimumPercent,
		"diskMinimumPercent":   config.DiskMinimumPercent,
	} {
		if value < 0 || value > 100 {
			return fmt.Errorf("monitor spike %s must be between 0 and 100", name)
		}
	}
	return nil
}

func EvaluateResourceSpikes(config ResourceSpikeConfig, previous MonitorState, metrics []host.ResourceMetric, now time.Time) ResourceSpikeDelta {
	config = withResourceSpikeDefaults(config)
	next := ResourceSpikeState{
		Samples:   append([]ResourceSample(nil), previous.Spikes.Samples...),
		Baselines: cloneFloatSlices(previous.Spikes.Baselines),
		Breaches:  cloneIntMap(previous.Spikes.Breaches),
		Active:    cloneTimeMap(previous.Spikes.Active),
	}
	if next.Baselines == nil {
		next.Baselines = map[string][]float64{}
	}
	if next.Breaches == nil {
		next.Breaches = map[string]int{}
	}
	if next.Active == nil {
		next.Active = map[string]time.Time{}
	}
	if !resourceSpikesEnabled(config) {
		return ResourceSpikeDelta{NextState: next}
	}

	values := make(map[string]float64, len(metrics))
	for _, metric := range metrics {
		values[metric.Name] = metric.Value
	}
	next.Samples = append(next.Samples, ResourceSample{At: now, Metrics: values})
	maxSamples := config.BaselineSamples + config.ConsecutiveSamples + 10
	if len(next.Samples) > maxSamples {
		next.Samples = next.Samples[len(next.Samples)-maxSamples:]
	}

	delta := ResourceSpikeDelta{NextState: next}
	for _, metric := range metrics {
		rule, ok := getResourceSpikeRule(config, metric.Name)
		if !ok {
			continue
		}
		baselineValues := append([]float64(nil), next.Baselines[metric.Name]...)
		if len(baselineValues) == 0 {
			baselineValues = resourceHistoryValues(previous.Spikes.Samples, metric.Name, config.BaselineSamples)
		}
		if len(baselineValues) < config.BaselineSamples {
			next.Baselines[metric.Name] = appendBaseline(baselineValues, metric.Value, config.BaselineSamples)
			continue
		}
		baseline := average(baselineValues)
		increase := metric.Value - baseline
		breached := metric.Value >= rule.minimum && increase >= rule.increase
		if breached {
			next.Breaches[metric.Name]++
		} else {
			next.Breaches[metric.Name] = 0
			next.Baselines[metric.Name] = appendBaseline(baselineValues, metric.Value, config.BaselineSamples)
		}

		issueName := "resource-spike/" + metric.Name
		lastAlert, active := next.Active[metric.Name]
		if breached && next.Breaches[metric.Name] >= config.ConsecutiveSamples {
			if !active || now.Sub(lastAlert) >= time.Duration(config.CooldownMinutes)*time.Minute {
				delta.NewIssues = append(delta.NewIssues, MonitorIssue{
					Name:    issueName,
					Status:  resourceSpikeStatus(metric.Value),
					Message: fmt.Sprintf("%s is %.1f%%, baseline %.1f%% (+%.1f percentage points)", metric.Name, metric.Value, baseline, increase),
				})
				next.Active[metric.Name] = now
			}
		} else if !breached && active {
			delete(next.Active, metric.Name)
			delta.Resolved = append(delta.Resolved, issueName)
		}
	}
	sort.Strings(delta.Resolved)
	sort.Slice(delta.NewIssues, func(i, j int) bool { return delta.NewIssues[i].Name < delta.NewIssues[j].Name })
	delta.NextState = next
	return delta
}

func resourceSpikesEnabled(config ResourceSpikeConfig) bool {
	return config.Enabled == nil || *config.Enabled
}

type spikeRule struct {
	increase float64
	minimum  float64
}

func getResourceSpikeRule(config ResourceSpikeConfig, name string) (spikeRule, bool) {
	switch name {
	case "host.cpu":
		return spikeRule{increase: config.CPUIncreasePercent, minimum: config.CPUMinimumPercent}, true
	case "host.memory":
		return spikeRule{increase: config.MemoryIncreasePercent, minimum: config.MemoryMinimumPercent}, true
	case "host.disk":
		return spikeRule{increase: config.DiskIncreasePercent, minimum: config.DiskMinimumPercent}, true
	default:
		if strings.HasPrefix(name, "container.") && strings.HasSuffix(name, ".cpu") {
			return spikeRule{increase: config.ContainerCPUIncreasePercent, minimum: config.CPUMinimumPercent}, true
		}
		if strings.HasPrefix(name, "container.") && strings.HasSuffix(name, ".memory") {
			return spikeRule{increase: config.ContainerMemoryIncreasePercent, minimum: config.MemoryMinimumPercent}, true
		}
		return spikeRule{}, false
	}
}

func resourceHistoryValues(samples []ResourceSample, name string, limit int) []float64 {
	values := make([]float64, 0, limit)
	for index := len(samples) - 1; index >= 0 && len(values) < limit; index-- {
		if value, ok := samples[index].Metrics[name]; ok {
			values = append(values, value)
		}
	}
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
	return values
}

func average(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneTimeMap(values map[string]time.Time) map[string]time.Time {
	if values == nil {
		return nil
	}
	clone := make(map[string]time.Time, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneFloatSlices(values map[string][]float64) map[string][]float64 {
	if values == nil {
		return nil
	}
	clone := make(map[string][]float64, len(values))
	for key, value := range values {
		clone[key] = append([]float64(nil), value...)
	}
	return clone
}

func appendBaseline(values []float64, value float64, limit int) []float64 {
	values = append(append([]float64(nil), values...), value)
	if len(values) > limit {
		values = values[len(values)-limit:]
	}
	return values
}

func resourceSpikeStatus(value float64) string {
	if value >= 95 {
		return "critical"
	}
	return "warning"
}

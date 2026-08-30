package alert

import (
	"testing"
	"time"

	"omurga/internal/host"
)

func TestEvaluateResourceSpikesRequiresConsecutiveSamplesAndRecovers(t *testing.T) {
	enabled := true
	config := ResourceSpikeConfig{
		Enabled:            &enabled,
		BaselineSamples:    3,
		ConsecutiveSamples: 2,
		CooldownMinutes:    30,
		CPUIncreasePercent: 20,
		CPUMinimumPercent:  70,
	}
	state := MonitorState{}
	start := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	for index := 0; index < 3; index++ {
		delta := EvaluateResourceSpikes(config, state, []host.ResourceMetric{{Name: "host.cpu", Value: 40, Unit: "percent"}}, start.Add(time.Duration(index)*time.Minute))
		state.Spikes = delta.NextState
	}

	delta := EvaluateResourceSpikes(config, state, []host.ResourceMetric{{Name: "host.cpu", Value: 80, Unit: "percent"}}, start.Add(3*time.Minute))
	if len(delta.NewIssues) != 0 {
		t.Fatalf("spike alert fired before consecutive sample requirement: %#v", delta.NewIssues)
	}
	state.Spikes = delta.NextState

	delta = EvaluateResourceSpikes(config, state, []host.ResourceMetric{{Name: "host.cpu", Value: 80, Unit: "percent"}}, start.Add(4*time.Minute))
	if len(delta.NewIssues) != 1 || delta.NewIssues[0].Name != "resource-spike/host.cpu" {
		t.Fatalf("unexpected spike alert: %#v", delta.NewIssues)
	}
	if delta.NewIssues[0].Status != "warning" || delta.NewIssues[0].Message != "host.cpu is 80.0%, baseline 40.0% (+40.0 percentage points)" {
		t.Fatalf("unexpected spike alert details: %#v", delta.NewIssues[0])
	}
	state.Spikes = delta.NextState

	delta = EvaluateResourceSpikes(config, state, []host.ResourceMetric{{Name: "host.cpu", Value: 80, Unit: "percent"}}, start.Add(5*time.Minute))
	if len(delta.NewIssues) != 0 {
		t.Fatalf("cooldown did not suppress repeated alert: %#v", delta.NewIssues)
	}
	state.Spikes = delta.NextState

	delta = EvaluateResourceSpikes(config, state, []host.ResourceMetric{{Name: "host.cpu", Value: 40, Unit: "percent"}}, start.Add(6*time.Minute))
	if len(delta.Resolved) != 1 || delta.Resolved[0] != "resource-spike/host.cpu" {
		t.Fatalf("unexpected spike recovery: %#v", delta.Resolved)
	}
}

func TestValidateResourceSpikeConfigAcceptsExplicitDisable(t *testing.T) {
	disabled := false
	if err := ValidateResourceSpikeConfig(ResourceSpikeConfig{Enabled: &disabled}); err != nil {
		t.Fatalf("disabled resource spikes should be valid: %v", err)
	}
}

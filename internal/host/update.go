package host

import (
	"context"
	"fmt"
	"strings"
)

type PackageCommand struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

type UpdateResult struct {
	Mode           string           `json:"mode"`
	PackageManager string           `json:"packageManager"`
	Commands       []PackageCommand `json:"commands"`
	DryRun         bool             `json:"dryRun"`
}

func UpdatePackages(ctx context.Context, runner Runner, provider DistributionProvider, full, dryRun bool) (UpdateResult, error) {
	if provider == nil {
		return UpdateResult{}, fmt.Errorf("distribution provider is required")
	}
	mode := "safe"
	if full {
		mode = "full"
	}
	packages := provider.PackageManager()

	result := UpdateResult{
		Mode:           mode,
		PackageManager: packages.Name(),
		DryRun:         dryRun,
		Commands: []PackageCommand{
			packages.RefreshCommand(),
			packages.UpgradeCommand(full, dryRun),
		},
	}
	if dryRun {
		return result, nil
	}

	for _, command := range result.Commands {
		if _, err := runner.Run(ctx, command.Name, command.Args...); err != nil {
			return result, fmt.Errorf("package update failed while running %s %s: %w", command.Name, strings.Join(command.Args, " "), err)
		}
	}
	return result, nil
}

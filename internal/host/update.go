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
	Mode     string           `json:"mode"`
	Commands []PackageCommand `json:"commands"`
	DryRun   bool             `json:"dryRun"`
}

func UpdatePackages(ctx context.Context, runner Runner, full, dryRun bool) (UpdateResult, error) {
	mode := "safe"
	upgradeCommand := "upgrade"
	if full {
		mode = "full"
		upgradeCommand = "full-upgrade"
	}

	result := UpdateResult{
		Mode:   mode,
		DryRun: dryRun,
		Commands: []PackageCommand{
			{Name: "apt-get", Args: []string{"update"}},
			{Name: "apt-get", Args: []string{"-y", "-o", "Dpkg::Options::=--force-confold", upgradeCommand}},
		},
	}
	if dryRun {
		result.Commands[1].Args = []string{"-s", "-o", "Dpkg::Options::=--force-confold", upgradeCommand}
		return result, nil
	}

	for _, command := range result.Commands {
		if _, err := runner.Run(ctx, command.Name, command.Args...); err != nil {
			return result, fmt.Errorf("package update failed while running %s %s: %w", command.Name, strings.Join(command.Args, " "), err)
		}
	}
	return result, nil
}

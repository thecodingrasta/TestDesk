package manager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

// A file-only catalog entry cannot distinguish the same file in Jest projects.
func checkJestProjectAmbiguity(ctx context.Context, proc runner.Runner, command domain.CommandSpec) error {
	return checkJestProjectSelection(ctx, proc, command, "")
}

type jestProjectConfig struct {
	DisplayName *struct {
		Name string `json:"name"`
	} `json:"displayName"`
}

func readJestProjects(ctx context.Context, proc runner.Runner, command domain.CommandSpec) ([]jestProjectConfig, error) {
	probe := domain.CommandSpec{Exe: command.Exe, Args: []string{"jest", "--showConfig"}, WorkingDir: command.WorkingDir, TimeoutSeconds: 30}
	result, err := proc.Run(ctx, probe, nil)
	if err != nil || result.ExitCode != 0 || result.TimedOut || ctx.Err() != nil {
		return nil, fmt.Errorf("could not inspect Jest projects")
	}
	var configuration struct {
		Configs []jestProjectConfig `json:"configs"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &configuration); err != nil || len(configuration.Configs) == 0 {
		return nil, fmt.Errorf("could not identify Jest projects")
	}
	return configuration.Configs, nil
}

func checkJestProjectSelection(ctx context.Context, proc runner.Runner, command domain.CommandSpec, project string) error {
	projects, err := readJestProjects(ctx, proc, command)
	if err != nil {
		return err
	}
	if project == "" {
		if len(projects) != 1 {
			return fmt.Errorf("targeted Jest runs require project identity; rediscover tests")
		}
		return nil
	}
	matches := 0
	for _, candidate := range projects {
		if candidate.DisplayName != nil && candidate.DisplayName.Name == project {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("Jest project %q is missing or ambiguous; rediscover tests", project)
	}
	return nil
}

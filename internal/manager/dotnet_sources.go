package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/discovery"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func dotNetMixedAdapters(tests []domain.TestCase) bool {
	nunit, other := false, false
	for _, test := range tests {
		if test.DotNetFilterProperty == "FullyQualifiedName" {
			nunit = true
		} else {
			other = true
		}
	}
	return nunit && other
}

// Mixed solutions must run individual projects: NUnit rejects xUnit's
// DisplayName filter even when no NUnit case matches it.
func (m *Manager) identifyDotNetProjects(ctx context.Context, provider domain.Provider, discoveryCmd domain.CommandSpec, tests []domain.TestCase) error {
	if len(discoveryCmd.Args) > 1 {
		target := discoveryCmd.Args[1]
		switch strings.ToLower(filepath.Ext(target)) {
		case ".csproj", ".fsproj", ".vbproj":
			if !filepath.IsAbs(target) {
				target = filepath.Join(discoveryCmd.WorkingDir, target)
			}
			for i := range tests {
				tests[i].DotNetProject = filepath.Clean(target)
				tests[i].DotNetNUnitLegacy = true
			}
			return nil
		}
	}
	var solutions []string
	for _, pattern := range []string{"*.sln", "*.slnx"} {
		matches, err := filepath.Glob(filepath.Join(discoveryCmd.WorkingDir, pattern))
		if err != nil {
			return err
		}
		solutions = append(solutions, matches...)
	}
	if len(solutions) != 1 {
		if len(solutions) == 0 {
			var projects []string
			for _, pattern := range []string{"*.csproj", "*.fsproj", "*.vbproj"} {
				matches, err := filepath.Glob(filepath.Join(discoveryCmd.WorkingDir, pattern))
				if err != nil {
					return err
				}
				projects = append(projects, matches...)
			}
			if len(projects) == 1 {
				for i := range tests {
					tests[i].DotNetProject = projects[0]
					tests[i].DotNetNUnitLegacy = true
				}
				return nil
			}
		}
		return fmt.Errorf("mixed .NET adapters require a single solution to identify test projects")
	}
	listCmd := discoveryCmd
	listCmd.Args = []string{"sln", solutions[0], "list"}
	list, err := m.runDotNetIdentityCommand(ctx, listCmd)
	if err != nil {
		return fmt.Errorf("list .NET solution projects: %w", err)
	}
	var projects []string
	for _, line := range strings.Split(list, "\n") {
		path := strings.TrimSpace(line)
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csproj", ".fsproj", ".vbproj":
		default:
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(solutions[0]), filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
			return fmt.Errorf("solution project %q is unavailable", path)
		}
		projects = append(projects, path)
	}
	sort.Strings(projects)
	if len(projects) == 0 {
		return fmt.Errorf("solution listed no supported .NET projects")
	}
	owners := make(map[string][]string)
	for _, project := range projects {
		projectCmd := discoveryCmd
		projectCmd.Args = []string{"test", project, "--list-tests", "--nologo", "--no-build", "--no-restore", "--", "NUnit.DisplayName=FullName"}
		output, runErr := m.runDotNetIdentityCommand(ctx, projectCmd)
		if runErr != nil {
			return fmt.Errorf("discover project %q: %w", project, runErr)
		}
		projectTests, parseErr := discovery.Parse(provider, output)
		if parseErr != nil {
			return fmt.Errorf("discover project %q: %w", project, parseErr)
		}
		for _, test := range projectTests {
			key := strings.ToLower(test.FullName)
			owners[key] = append(owners[key], project)
		}
	}
	projectAdapters := make(map[string]uint8)
	for i := range tests {
		key := strings.ToLower(tests[i].FullName)
		if len(owners[key]) == 0 {
			return fmt.Errorf("could not identify the project for .NET test %q", tests[i].FullName)
		}
		tests[i].DotNetProject, owners[key] = owners[key][0], owners[key][1:]
		if tests[i].DotNetFilterProperty == "FullyQualifiedName" {
			projectAdapters[tests[i].DotNetProject] |= 1
		} else {
			projectAdapters[tests[i].DotNetProject] |= 2
		}
	}
	for i := range tests {
		tests[i].DotNetNUnitLegacy = projectAdapters[tests[i].DotNetProject] == 3
	}
	for name, remaining := range owners {
		if len(remaining) != 0 {
			return fmt.Errorf("inconsistent project identities for .NET test %q", name)
		}
	}
	return nil
}

func (m *Manager) runDotNetIdentityCommand(ctx context.Context, cmd domain.CommandSpec) (string, error) {
	result, err := m.runner.Run(ctx, cmd, func(event runner.Event) {
		if event.Type == runner.EventStdout || event.Type == runner.EventStderr {
			now := time.Now().UTC()
			m.mu.Lock()
			m.discoveryStatus.LastOutputAt = &now
			m.mu.Unlock()
		}
	})
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if result.ExitCode != 0 || result.TimedOut {
		return "", fmt.Errorf("exit code %d, timed out: %t", result.ExitCode, result.TimedOut)
	}
	return result.Stdout, nil
}

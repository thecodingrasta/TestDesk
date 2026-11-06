package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/discovery"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func isBuiltInJestDiscovery(provider domain.Provider) bool {
	return reflect.DeepEqual(provider.Discover, config.JestProvider().Discover)
}

func (m *Manager) discoverJestProjects(ctx context.Context, provider domain.Provider, command domain.CommandSpec, initial []domain.TestCase) ([]domain.TestCase, error) {
	projects, err := readJestProjects(ctx, m.runner, command)
	if err != nil {
		return nil, err
	}
	if len(projects) == 1 {
		return initial, nil
	}
	seen := make(map[string]bool)
	var cases []domain.TestCase
	for _, project := range projects {
		if project.DisplayName == nil || strings.TrimSpace(project.DisplayName.Name) == "" || seen[project.DisplayName.Name] {
			return nil, fmt.Errorf("multi-project Jest discovery requires unique non-empty display names")
		}
		name := project.DisplayName.Name
		seen[name] = true
		listing := command
		listing.Args = append(append([]string(nil), command.Args...), "--json", "--selectProjects="+name)
		result, err := m.runner.Run(ctx, listing, nil)
		if err != nil || result.ExitCode != 0 || result.TimedOut || ctx.Err() != nil {
			return nil, fmt.Errorf("Jest discovery for project %q failed", name)
		}
		var files []string
		if err := json.Unmarshal([]byte(result.Stdout), &files); err != nil || files == nil {
			return nil, fmt.Errorf("invalid Jest file listing for project %q", name)
		}
		for _, file := range files {
			if strings.ContainsAny(file, "\r\n") || strings.TrimSpace(file) == "" {
				return nil, fmt.Errorf("invalid Jest file identity for project %q", name)
			}
		}
		found, err := discovery.Parse(provider, strings.Join(files, "\n"))
		if err != nil {
			return nil, err
		}
		for _, test := range found {
			hash := sha256.Sum256([]byte(test.ID + "\x00" + name))
			test.ID = hex.EncodeToString(hash[:8])
			test.JestProject = name
			test.Name = name + " · " + test.Name
			test.Module = name
			cases = append(cases, test)
		}
	}
	return cases, nil
}

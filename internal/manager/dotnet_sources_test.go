package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type sourceFixtureRunner struct{ projects map[string]string }

func (r sourceFixtureRunner) Run(_ context.Context, cmd domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if cmd.Args[0] == "sln" {
		var paths []string
		for path := range r.projects {
			paths = append(paths, path)
		}
		return runner.Result{Stdout: strings.Join(paths, "\n")}, nil
	}
	return runner.Result{Stdout: "The following Tests are available:\n" + r.projects[cmd.Args[1]]}, nil
}

func TestDotNetProjectOwnershipRequiresCompleteUnambiguousDiscovery(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"fixture.slnx", "first.csproj", "second.csproj"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, second := filepath.Join(dir, "first.csproj"), filepath.Join(dir, "second.csproj")
	tests := []domain.TestCase{{FullName: "Fixture.NUnit.Case", DotNetFilterProperty: "FullyQualifiedName"}, {FullName: "Fixture.Xunit.Case"}}
	provider := config.DotNetProvider()
	cmd := domain.CommandSpec{Exe: "dotnet", WorkingDir: dir}
	m := &Manager{runner: sourceFixtureRunner{projects: map[string]string{first: "  Fixture.NUnit.Case\n", second: "  Fixture.Xunit.Case\n"}}}
	if err := m.identifyDotNetProjects(context.Background(), provider, cmd, tests); err != nil {
		t.Fatal(err)
	}
	if tests[0].DotNetProject != first || tests[1].DotNetProject != second {
		t.Fatalf("wrong project ownership: %+v", tests)
	}
	m.runner = sourceFixtureRunner{projects: map[string]string{first: "  Fixture.NUnit.Case\n"}}
	if err := m.identifyDotNetProjects(context.Background(), provider, cmd, tests); err == nil {
		t.Fatal("incomplete project listing was accepted")
	}
	m.runner = sourceFixtureRunner{projects: map[string]string{first: "  Fixture.NUnit.Case\n  Fixture.Xunit.Case\n"}}
	if err := m.identifyDotNetProjects(context.Background(), provider, cmd, tests); err != nil || !tests[0].DotNetNUnitLegacy || !tests[1].DotNetNUnitLegacy {
		t.Fatalf("mixed project did not use NUnit legacy mode: %+v %v", tests, err)
	}
}

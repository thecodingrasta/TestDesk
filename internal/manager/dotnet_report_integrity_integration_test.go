package manager_test

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

// reportDamageRunner runs the actual provider, then damages only its disposable
// output before the manager consumes it. Discovery is left untouched.
type reportDamageRunner struct {
	damage  string
	applied chan struct{}
}

func (r reportDamageRunner) Run(ctx context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	result, err := runner.NewProcessRunner().Run(ctx, cmd, onEvent)
	if err != nil || result.ExitCode != 0 {
		return result, err
	}
	for i, arg := range cmd.Args {
		if arg != "--results-directory" || i+1 >= len(cmd.Args) {
			continue
		}
		reports, globErr := filepath.Glob(filepath.Join(cmd.Args[i+1], "*.trx"))
		if globErr != nil || len(reports) != 1 {
			return result, fmt.Errorf("expected one real TRX, got %v: %v", reports, globErr)
		}
		data, readErr := os.ReadFile(reports[0])
		if readErr != nil || !strings.Contains(string(data), `outcome="Passed"`) {
			return result, fmt.Errorf("real TRX did not establish passing execution: %v", readErr)
		}
		switch r.damage {
		case "missing":
			err = os.Remove(reports[0])
		case "empty":
			err = os.WriteFile(reports[0], nil, 0o600)
		case "malformed":
			err = os.WriteFile(reports[0], []byte("<TestRun>"), 0o600)
		case "partial":
			err = os.WriteFile(filepath.Join(cmd.Args[i+1], "incomplete.trx"), nil, 0o600)
		default:
			err = fmt.Errorf("unknown damage %q", r.damage)
		}
		if err == nil {
			r.applied <- struct{}{}
		}
		return result, err
	}
	return result, nil
}

func TestRealDotNetRejectsDamagedReports(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("requires cached .NET test packages")
	}
	project := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUGET_PACKAGES", filepath.Join(project, "packages"))
	t.Setenv("DOTNET_CLI_HOME", filepath.Join(project, "home"))
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	files := map[string]string{
		"fixture.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`,
		"NuGet.Config":   `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Fact] public void Passes() { Assert.True(true); } }`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider := config.DotNetProvider()
	provider.ID = "fixture"
	cfg := config.Normalise(project, domain.ProjectConfig{Version: 1, Name: "real-report-integrity", Providers: []domain.Provider{provider}})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	for _, damage := range []string{"missing", "empty", "malformed", "partial"} {
		t.Run(damage, func(t *testing.T) {
			applied := make(chan struct{}, 1)
			m, err := manager.New(project, cfg, reportDamageRunner{damage: damage, applied: applied})
			if err != nil {
				t.Fatal(err)
			}
			tests, err := m.Discover(ctx)
			if err != nil || len(tests) != 1 {
				t.Fatalf("discovery: %+v, %v", tests, err)
			}
			run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tests[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-applied:
			default:
				t.Fatal("the real passing TRX was not damaged")
			}
			diag, err := m.Diagnostics(run.ID, 4096)
			if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
				t.Fatalf("damaged report accepted: %+v, %v", diag.Run, err)
			}
			reopened, err := manager.New(project, cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snap.Tests) != 1 || snap.Tests[0].LastStatus != domain.StatusUnknown {
				t.Fatalf("persisted result after damaged report: %+v, %v", snap.Tests, err)
			}
		})
	}
}

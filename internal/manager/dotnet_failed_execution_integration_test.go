package manager_test

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealDotNetSelectedTestAndBuildFailure(t *testing.T) {
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
		"Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Fact] public void Fails() { Assert.True(false); } }`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	provider := config.DotNetProvider()
	provider.ID = "fixture"
	cfg := config.Normalise(project, domain.ProjectConfig{Version: 1, Name: "failure-evidence", Workspace: ".testdesk", Providers: []domain.Provider{provider}})
	m, err := manager.New(project, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tests, err := m.Discover(ctx)
	if err != nil || len(tests) != 1 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	for _, tc := range []struct {
		name, replacement string
		want              domain.Status
	}{
		{name: "test failure", want: domain.StatusFailed},
		{name: "build failure", replacement: "this is not valid C#", want: domain.StatusUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.replacement != "" {
				if err := os.WriteFile(filepath.Join(project, "Cases.cs"), []byte(tc.replacement), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tests[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			diag, err := m.Diagnostics(run.ID, 4096)
			if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed {
				t.Fatalf("run: %+v, %v", diag.Run, err)
			}
			reopened, err := manager.New(project, cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snap.Tests) != 1 || snap.Tests[0].LastStatus != tc.want {
				t.Fatalf("catalog: %+v, %v; want %s", snap.Tests, err, tc.want)
			}
		})
	}
}

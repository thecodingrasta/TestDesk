package manager_test

import (
	"context"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealDotNetParameterizedSelection(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("requires cached .NET test packages")
	}
	dir := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUGET_PACKAGES", filepath.Join(dir, "packages"))
	t.Setenv("DOTNET_CLI_HOME", filepath.Join(dir, "home"))
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	files := map[string]string{
		"fixture.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`,
		"NuGet.Config":   `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Theory] [InlineData("json {\"k\":[1,2]} & A|B = C! (~) 100%;")] [InlineData("other")] public void Select(string value) { Assert.NotNull(value); } }`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	cfg := config.Normalise(dir, domain.ProjectConfig{Version: 1, Name: "selector", Workspace: ".testdesk", Providers: []domain.Provider{p}})
	m, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery: %+v %v", cases, err)
	}
	for _, tc := range cases {
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run.Status != domain.StatusPassed {
			t.Fatalf("%s: %+v %v", tc.FullName, diag, err)
		}
		path := m.Store().ArtifactPath(run.ID, "fixture", "001", ".trx") + ".d"
		reports, err := filepath.Glob(filepath.Join(path, "*.trx"))
		if err != nil || len(reports) != 1 {
			t.Fatalf("reports: %v %v", reports, err)
		}
		report, err := os.ReadFile(reports[0])
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(report), "<UnitTestResult "); n != 1 {
			t.Fatalf("%s: expected one result, got %d", tc.FullName, n)
		}
	}
}

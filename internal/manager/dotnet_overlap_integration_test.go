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

func TestRealDotNetSolutionProjectOverlapPlan(t *testing.T) {
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
	projectDir := filepath.Join(dir, "tests", "Fixture")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Fixture.slnx":                 `<Solution><Project Path="tests/Fixture/Fixture.csproj" /></Solution>`,
		"NuGet.Config":                 `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"tests/Fixture/Fixture.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`,
		"tests/Fixture/Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Fact] public void Passes() { Assert.True(true); } }`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	solution := config.DotNetProvider()
	project := config.DotNetProvider()
	project.ID = "dotnet-fixture"
	project.Root = "tests/Fixture"
	for _, spec := range []*domain.CommandSpec{&project.Discover.Command, &project.Run.All, &project.Run.Test} {
		for i, arg := range spec.Args {
			if arg == "{{provider.root}}" {
				spec.Args[i] = "."
			}
		}
	}
	cfg := domain.ProjectConfig{Version: 1, Name: "overlap", Providers: []domain.Provider{solution, project}}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	m, err := manager.NewFromProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery: %+v %v", cases, err)
	}
	if cases[0].FullName != cases[1].FullName || cases[0].ProviderID == cases[1].ProviderID {
		t.Fatalf("unexpected cases: %+v", cases)
	}
	plan, err := m.Plan(manager.RunRequest{TestIDs: []string{cases[0].ID, cases[1].ID}})
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	for _, item := range plan {
		if len(item.Warnings) != 1 || !strings.Contains(item.Warnings[0], "overlap") {
			t.Fatalf("missing overlap warning: %+v", item)
		}
	}
}

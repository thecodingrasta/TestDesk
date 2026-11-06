package manager_test

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealDotNetProjectsWithSameFilename(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("requires cached .NET test packages")
	}
	root := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUGET_PACKAGES", filepath.Join(root, "packages"))
	t.Setenv("DOTNET_CLI_HOME", filepath.Join(root, "home"))
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	nuget := `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`
	if err := os.WriteFile(filepath.Join(root, "NuGet.Config"), []byte(nuget), 0600); err != nil {
		t.Fatal(err)
	}
	project := `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`
	for _, name := range []string{"First", "Second"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for file, content := range map[string]string{
			"Shared.Tests.csproj": project,
			"Cases.cs":            fmt.Sprintf(`using Xunit; namespace %s; public class Cases { [Fact] public void Passes() { Assert.True(true); } }`, name),
		} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := config.Detect(root)
	m, err := manager.New(root, cfg, mixedRecordingRunner{t})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 || cases[0].ProviderID == cases[1].ProviderID {
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
		if err != nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
			t.Fatalf("%s: %+v %v", tc.FullName, diag.Run, err)
		}
	}
}

func TestRealDotNetProjectsInSameDirectory(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("requires cached .NET test packages")
	}
	root := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUGET_PACKAGES", filepath.Join(root, "packages"))
	t.Setenv("DOTNET_CLI_HOME", filepath.Join(root, "home"))
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	files := map[string]string{
		"NuGet.Config":          `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"Directory.Build.props": `<Project><PropertyGroup><BaseIntermediateOutputPath>obj/$(MSBuildProjectName)/</BaseIntermediateOutputPath></PropertyGroup></Project>`,
	}
	for _, name := range []string{"First", "Second"} {
		packages := `<PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/>`
		files[name+".cs"] = fmt.Sprintf(`using Xunit; namespace %s; public class Cases { [Fact] public void Passes() { Assert.True(true); } }`, name)
		if name == "Second" {
			packages = `<PackageReference Include="NUnit" Version="4.1.0"/><PackageReference Include="NUnit3TestAdapter" Version="4.5.0"/>`
			files[name+".cs"] = `using NUnit.Framework; namespace Second; public class Cases { [TestCase("a.b:c")] public void Passes(string value) { Assert.That(value, Is.Not.Null); } }`
		}
		files[name+".Tests.csproj"] = `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation><EnableDefaultCompileItems>false</EnableDefaultCompileItems><OutputPath>bin/$(MSBuildProjectName)/</OutputPath></PropertyGroup><ItemGroup><Compile Include="` + name + `.cs"/><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/>` + packages + `</ItemGroup></Project>`
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), mixedRecordingRunner{t})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 || cases[0].ProviderID == cases[1].ProviderID {
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
		if err != nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
			t.Fatalf("%s: %+v %v", tc.FullName, diag.Run, err)
		}
	}
}

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
	"github.com/thecodingrasta/testdesk/internal/results"
)

func TestRealDotNetMixedProjectInsideSolution(t *testing.T) {
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
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	if err := os.Mkdir(filepath.Join(root, "tests"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"NuGet.Config":             `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"fixture.slnx":             `<Solution><Project Path="tests/Mixed.Tests.csproj"/></Solution>`,
		"tests/Mixed.Tests.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/><PackageReference Include="NUnit" Version="4.1.0"/><PackageReference Include="NUnit3TestAdapter" Version="4.5.0"/></ItemGroup></Project>`,
		"tests/Cases.cs":           `namespace Mixed; public class X { [Xunit.Theory] [Xunit.InlineData("a,b(c)")] public void Case(string value) { Xunit.Assert.NotNull(value); } } public class N { [NUnit.Framework.TestCase("a,b(c)")] public void Case(string value) { NUnit.Framework.Assert.That(value, NUnit.Framework.Is.Not.Null); } }`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	provider := config.DotNetProvider()
	provider.ID = "fixture"
	cfg := config.Normalise(root, domain.ProjectConfig{Providers: []domain.Provider{provider}})
	m, err := manager.New(root, cfg, mixedRecordingRunner{t})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("mixed solution discovery: %+v %v", cases, err)
	}
	for _, tc := range cases {
		if !tc.DotNetNUnitLegacy || tc.DotNetProject != filepath.Join(root, "tests", "Mixed.Tests.csproj") {
			t.Fatalf("incorrect mixed-project ownership: %+v", tc)
		}
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{tc.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run.Status != domain.StatusPassed {
			t.Fatalf("%s: %+v %v", tc.FullName, diag.Run, err)
		}
		parsed, err := results.ParseTRXDir(m.Store().ArtifactPath(run.ID, provider.ID, "001", ".trx") + ".d")
		if err != nil || len(parsed) != 1 || parsed[0].Name != tc.FullName {
			t.Fatalf("%s: %+v %v", tc.FullName, parsed, err)
		}
	}
}

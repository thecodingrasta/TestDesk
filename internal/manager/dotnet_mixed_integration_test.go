package manager_test

import (
	"context"
	"encoding/xml"
	"html"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type mixedRecordingRunner struct{ t *testing.T }

func (r mixedRecordingRunner) Run(ctx context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	result, err := runner.NewProcessRunner().Run(ctx, cmd, onEvent)
	if err != nil {
		r.t.Logf("command failure: %s\n%s\n%s", runner.Preview(cmd), result.Stdout, result.Stderr)
	}
	return result, err
}

func TestRealMixedDotNetAdaptersSelection(t *testing.T) {
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
	for _, name := range []string{"nunit", "xunit"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	base := `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><AssemblyName>fixture.$(MSBuildProjectName)</AssemblyName><PackageId>fixture.$(MSBuildProjectName)</PackageId><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/>`
	files := map[string]string{
		"fixture.slnx":       `<Solution><Project Path="nunit/nunit.csproj"/><Project Path="xunit/xunit.csproj"/></Solution>`,
		"NuGet.Config":       `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"nunit/nunit.csproj": base + `<PackageReference Include="NUnit" Version="4.1.0"/><PackageReference Include="NUnit3TestAdapter" Version="4.5.0"/></ItemGroup></Project>`,
		"xunit/xunit.csproj": base + `<PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`,
		"nunit/Cases.cs":     `using NUnit.Framework; namespace Mixed; public class NUnitCases { [TestCase("a.b:c")] public void Select(string value) { Assert.That(value, Is.Not.Null); } }`,
		"xunit/Cases.cs":     `using Xunit; namespace Mixed; public class XunitCases { [Theory] [InlineData("a.b:c")] public void Select(string value) { Assert.NotNull(value); } }`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	m, err := manager.New(dir, config.Normalise(dir, domain.ProjectConfig{Version: 1, Name: "mixed", Workspace: ".testdesk", Providers: []domain.Provider{p}}), mixedRecordingRunner{t})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("mixed discovery: %+v %v", cases, err)
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
		paths, err := filepath.Glob(filepath.Join(m.Store().ArtifactPath(run.ID, "fixture", "001", ".trx")+".d", "*.trx"))
		if err != nil {
			t.Fatal(err)
		}
		var results []struct {
			Name string `xml:"testName,attr"`
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var trx struct {
				Results []struct {
					Name string `xml:"testName,attr"`
				} `xml:"Results>UnitTestResult"`
			}
			if err := xml.Unmarshal(data, &trx); err != nil {
				t.Fatal(err)
			}
			results = append(results, trx.Results...)
		}
		if len(results) != 1 || results[0].Name != tc.FullName {
			t.Fatalf("selected %q, got %+v", tc.FullName, results)
		}
	}
}

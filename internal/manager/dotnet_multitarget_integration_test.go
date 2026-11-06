package manager_test

import (
	"context"
	"encoding/xml"
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

func TestRealDotNetMultiTargetArtifacts(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("requires cached .NET test packages and .NET 8 and 10 runtimes")
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
		"fixture.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFrameworks>net8.0;net10.0</TargetFrameworks><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`,
		"NuGet.Config":   `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Fact] public void Passes() { Assert.True(true); } }`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	cfg := config.Normalise(dir, domain.ProjectConfig{Version: 1, Name: "multi-target", Workspace: ".testdesk", Providers: []domain.Provider{p}})
	m, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery: %+v %v", cases, err)
	}
	if _, err := m.Plan(manager.RunRequest{TestIDs: []string{cases[0].ID}}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("identical target-framework names should not be selected individually: %v", err)
	}
	run, err := m.Run(ctx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed {
		t.Fatalf("run: %+v %v", diag, err)
	}
	if diag.Run.Passed != 2 {
		t.Fatalf("two target-framework results must both count: %+v", diag.Run)
	}
	snapshot, err := m.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 2 {
		t.Fatalf("catalog: %+v %v", snapshot.Tests, err)
	}
	for _, test := range snapshot.Tests {
		if test.LastStatus != domain.StatusUnknown {
			t.Fatalf("ambiguous target-framework result was assigned to one case: %+v", test)
		}
	}
	paths, err := filepath.Glob(filepath.Join(m.Store().ArtifactPath(run.ID, "fixture", "001", ".trx")+".d", "*.trx"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("reports: %v %v", paths, err)
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
		if err := xml.Unmarshal(data, &trx); err != nil || len(trx.Results) != 1 || trx.Results[0].Name != "Fixture.Cases.Passes" {
			t.Fatalf("report %s: %+v %v", path, trx, err)
		}
	}
	failingSource := `using Xunit; namespace Fixture; public class Cases { [Fact] public void Passes() {
#if NET8_0
Assert.True(false);
#else
Assert.True(true);
#endif
} }`
	if err := os.WriteFile(filepath.Join(dir, "Cases.cs"), []byte(failingSource), 0600); err != nil {
		t.Fatal(err)
	}
	failingRun, err := m.Run(ctx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	failingDiag, err := m.Diagnostics(failingRun.ID, 4096)
	if err != nil || failingDiag.Run == nil || failingDiag.Run.Status != domain.StatusFailed || failingDiag.Run.Passed != 1 || failingDiag.Run.Failed != 1 {
		t.Fatalf("mixed target-framework outcomes: %+v %v", failingDiag.Run, err)
	}
	snapshot, err = m.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 2 {
		t.Fatalf("catalog: %+v %v", snapshot.Tests, err)
	}
	for _, test := range snapshot.Tests {
		if test.LastStatus != domain.StatusUnknown || test.LastRunID != failingRun.ID {
			t.Fatalf("mixed result appeared conclusive: %+v", test)
		}
	}
}

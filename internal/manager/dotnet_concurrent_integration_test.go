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

func TestRealDotNetConcurrentRunArtifacts(t *testing.T) {
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
		"Cases.cs":       `using Xunit; namespace Fixture; public class Cases { [Theory] [InlineData(1)] [InlineData(2)] public void Select(int value) { System.Threading.Thread.Sleep(1000); Assert.True(value > 0); } }`,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	cfg := config.Normalise(dir, domain.ProjectConfig{Version: 1, Name: "concurrent", Workspace: ".testdesk", Providers: []domain.Provider{p}})
	a, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cases, err := a.Discover(ctx)
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery: %+v %v", cases, err)
	}
	b, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	runA, err := a.Run(ctx, manager.RunRequest{TestIDs: []string{cases[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	runB, err := b.Run(ctx, manager.RunRequest{TestIDs: []string{cases[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		mgr *manager.Manager
		run domain.Run
	}{{a, runA}, {b, runB}} {
		if err := item.mgr.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := item.mgr.Diagnostics(item.run.ID, 4096)
		if err != nil || diag.Run.Status != domain.StatusPassed {
			t.Fatalf("run %s: %+v %v", item.run.ID, diag, err)
		}
		reports, err := filepath.Glob(filepath.Join(item.mgr.Store().ArtifactPath(item.run.ID, "fixture", "001", ".trx")+".d", "*.trx"))
		if err != nil || len(reports) != 1 {
			t.Fatalf("run %s reports: %v %v", item.run.ID, reports, err)
		}
		data, err := os.ReadFile(reports[0])
		if err != nil || strings.Count(string(data), "<UnitTestResult ") != 1 {
			t.Fatalf("run %s has invalid result evidence: %v", item.run.ID, err)
		}
	}
	if runA.ID == runB.ID {
		t.Fatalf("concurrent runs shared ID %s", runA.ID)
	}
	reopened, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, mgr := range []*manager.Manager{a, b, reopened} {
		snapshot, err := mgr.Snapshot(domain.Filter{Limit: 10})
		if err != nil || len(snapshot.Tests) != 2 {
			t.Fatalf("catalog: %+v %v", snapshot.Tests, err)
		}
		for _, test := range snapshot.Tests {
			if test.LastStatus != domain.StatusPassed {
				t.Errorf("concurrent result not visible for %s: %s", test.FullName, test.LastStatus)
			}
		}
	}
}

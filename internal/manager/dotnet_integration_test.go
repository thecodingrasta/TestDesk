package manager_test

import (
	"context"
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeepRealDotnetArtifactsAndPersistence(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_DOTNET_TESTS") != "1" {
		t.Skip("set TESTDESK_REAL_DOTNET_TESTS=1 for the offline .NET integration fixture")
	}
	dir := t.TempDir()
	userDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUGET_PACKAGES", filepath.Join(dir, ".packages"))
	t.Setenv("DOTNET_CLI_HOME", filepath.Join(dir, "home"))
	t.Setenv("DOTNET_GENERATE_ASPNET_CERTIFICATE", "false")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	files := map[string]string{"fixture.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/></ItemGroup></Project>`, "NuGet.Config": `<configuration><packageSources><clear /><add key="cached" value="` + html.EscapeString(filepath.Join(userDir, ".nuget", "packages")) + `" /></packageSources></configuration>`, "UnitTest.cs": `using Xunit; namespace Fixture; public class UnitTest { [Fact] public void Passes() { Assert.True(true); } }`}
	for name, data := range files {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	cfg := config.Normalise(dir, domain.ProjectConfig{Version: 1, Name: "dotnet-fixture", Workspace: ".testdesk", Providers: []domain.Provider{p}})
	if e := config.Save(dir, cfg); e != nil {
		t.Fatal(e)
	}
	m, e := manager.New(dir, cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, e := m.Discover(ctx)
	if e != nil || len(tests) < 1 {
		t.Fatalf("discover=%d err=%v", len(tests), e)
	}
	t.Logf("discovered=%+v", tests)
	chosen := tests[0]
	for _, tc := range tests {
		if tc.Name == "Passes" {
			chosen = tc
		}
	}
	for i := 0; i < 2; i++ {
		r, e := m.Run(ctx, manager.RunRequest{TestIDs: []string{chosen.ID}})
		if e != nil {
			t.Fatal(e)
		}
		if e = m.WaitForIdle(ctx); e != nil {
			t.Fatal(e)
		}
		d, e := m.Diagnostics(r.ID, 16000)
		if e != nil {
			t.Fatal(e)
		}
		path := m.Store().ArtifactPath(r.ID, "fixture", "001", ".trx") + ".d"
		matches, ae := filepath.Glob(filepath.Join(path, "*.trx"))
		t.Logf("iteration=%d status=%s expectedArtifact=%s artifactError=%v log=%s", i, d.Run.Status, path, ae, d.Log)
		if d.Run.Status != domain.StatusPassed {
			t.Errorf("run did not pass: %s", d.Run.Status)
		}
		if ae != nil || len(matches) == 0 {
			t.Errorf("run-scoped TRX missing: %v (%v)", matches, ae)
		}
	}
	m2, e := manager.New(dir, cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	s, e := m2.Snapshot(domain.Filter{Limit: 10})
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("reopened status=%s", s.Tests[0].LastStatus)
	if s.Tests[0].LastStatus != domain.StatusPassed {
		t.Error("passing result not persisted across reopen")
	}
	for i, arg := range p.Run.Test.Args {
		if strings.HasPrefix(arg, "FullyQualifiedName=") {
			p.Run.Test.Args[i] = "FullyQualifiedName=Fixture.UnitTest.DoesNotExist"
		}
	}
	cfg.Providers[0] = p
	if err := m2.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	noMatchRun, err := m2.Run(ctx, manager.RunRequest{TestIDs: []string{chosen.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m2.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	noMatch, err := m2.Diagnostics(noMatchRun.ID, 4096)
	if err != nil || noMatch.Run == nil || noMatch.Run.Status != domain.StatusFailed {
		t.Fatalf("zero-match run: %+v, %v", noMatch.Run, err)
	}
	afterNoMatch, err := manager.New(dir, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	noMatchSnapshot, err := afterNoMatch.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(noMatchSnapshot.Tests) != 1 || noMatchSnapshot.Tests[0].LastStatus != domain.StatusUnknown {
		t.Fatalf("zero-match persisted status: %+v, %v", noMatchSnapshot.Tests, err)
	}
}

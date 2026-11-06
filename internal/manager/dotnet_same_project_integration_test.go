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
	"github.com/thecodingrasta/testdesk/internal/results"
)

func TestRealDotNetMixedAdaptersInSameProject(t *testing.T) {
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
	marker := filepath.Join(root, "executed.txt")
	t.Setenv("TESTDESK_MIXED_MARKER", marker)
	files := map[string]string{
		"NuGet.Config":       `<configuration><packageSources><clear/><add key="cached" value="` + html.EscapeString(filepath.Join(home, ".nuget", "packages")) + `"/></packageSources></configuration>`,
		"Mixed.Tests.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><IsTestProject>true</IsTestProject><NuGetAudit>false</NuGetAudit><UseSharedCompilation>false</UseSharedCompilation></PropertyGroup><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.14.1"/><PackageReference Include="xunit" Version="2.9.3"/><PackageReference Include="xunit.runner.visualstudio" Version="3.1.4"/><PackageReference Include="NUnit" Version="4.1.0"/><PackageReference Include="NUnit3TestAdapter" Version="4.5.0"/></ItemGroup></Project>`,
		"Cases.cs":           `namespace Mixed; public class XunitCases { [Xunit.Theory] [Xunit.InlineData("a.b:c")] [Xunit.InlineData("other,case")] [Xunit.InlineData("{\"k\":\"a,b(c)\"}")] public void Select(string value) { System.IO.File.AppendAllText(System.Environment.GetEnvironmentVariable("TESTDESK_MIXED_MARKER")!, "x:" + value + "\n"); Xunit.Assert.NotNull(value); } } public class NUnitCases { [NUnit.Framework.TestCase("a.b:c")] [NUnit.Framework.TestCase("other,case")] [NUnit.Framework.TestCase("{\"k\":\"a,b(c)\"}")] public void Select(string value) { System.IO.File.AppendAllText(System.Environment.GetEnvironmentVariable("TESTDESK_MIXED_MARKER")!, "n:" + value + "\n"); NUnit.Framework.Assert.That(value, NUnit.Framework.Is.Not.Null); } }`,
	}
	if framework := os.Getenv("TESTDESK_MIXED_FRAMEWORK"); framework != "" {
		files["Mixed.Tests.csproj"] = strings.ReplaceAll(files["Mixed.Tests.csproj"], "net10.0", framework)
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
	if err != nil || len(cases) != 6 {
		t.Fatalf("mixed project discovery: %+v %v", cases, err)
	}
	for _, tc := range cases {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if !tc.DotNetNUnitLegacy {
			t.Fatalf("missing mixed-adapter setting: %+v", tc)
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
		artifact := m.Store().ArtifactPath(run.ID, tc.ProviderID, "001", ".trx") + ".d"
		parsed, err := results.ParseTRXDir(artifact)
		if err != nil || len(parsed) != 1 || parsed[0].Name != tc.FullName {
			t.Fatalf("%s: results %+v %v", tc.FullName, parsed, err)
		}
		data, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		prefix := "n:"
		if strings.Contains(tc.FullName, "XunitCases") {
			prefix = "x:"
		}
		value := "a.b:c"
		if strings.Contains(tc.FullName, "other,case") {
			value = "other,case"
		}
		if strings.Contains(tc.FullName, "a,b(c)") {
			value = `{"k":"a,b(c)"}`
		}
		if string(data) != prefix+value+"\n" {
			t.Fatalf("%s executed unintended cases: %q", tc.FullName, data)
		}
	}
}

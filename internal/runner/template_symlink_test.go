package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// A project stored under a symlinked spelling (macOS /var -> /private/var) must run from its
// real path, or the .NET test adapters cannot find the copied package assemblies.
func TestDotNetProjectPathIsRunFromItsRealLocation(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	resolvedReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(real, "tests", "Mixed.Tests.csproj")
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("<Project/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := domain.CommandSpec{Exe: "dotnet", Args: []string{"test", "{{provider.root}}", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}}
	data := TemplateData{Provider: domain.Provider{Kind: domain.ProviderDotNet, Root: "."}, Test: &domain.TestCase{DotNetProject: filepath.Join(link, "tests", "Mixed.Tests.csproj")}}
	got, err := RenderCommand(spec, data)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolvedReal, "tests", "Mixed.Tests.csproj")
	if got.Args[1] != want {
		t.Fatalf("project argument = %q, want the real path %q", got.Args[1], want)
	}
	data.Test.DotNetProject = filepath.Join(real, "missing", "Absent.csproj")
	if got, err = RenderCommand(spec, data); err != nil || got.Args[1] != data.Test.DotNetProject {
		t.Fatalf("an unresolvable path must pass through unchanged: %q, %v", got.Args[1], err)
	}
}

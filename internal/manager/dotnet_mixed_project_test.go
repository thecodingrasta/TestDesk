package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestMixedDotNetSingleProjectTargetsProjectWithLegacyNUnit(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Mixed.Tests.csproj")
	if err := os.WriteFile(project, nil, 0600); err != nil {
		t.Fatal(err)
	}
	m := &Manager{}
	cases := []domain.TestCase{{FullName: "Mixed.NUnit.Case", DotNetFilterProperty: "FullyQualifiedName"}, {FullName: "Mixed.Xunit.Case"}}
	err := m.identifyDotNetProjects(context.Background(), config.DotNetProvider(), domain.CommandSpec{WorkingDir: root, Args: []string{"test", "."}}, cases)
	if err != nil || cases[0].DotNetProject != project || cases[1].DotNetProject != project || !cases[0].DotNetNUnitLegacy || !cases[1].DotNetNUnitLegacy {
		t.Fatalf("mixed project ownership: %+v %v", cases, err)
	}
}

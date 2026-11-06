package runner

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestGoPackageTemplateRequiresIdentity(t *testing.T) {
	provider := config.GoTestProvider()
	data := TemplateData{Provider: provider, Test: &domain.TestCase{Selector: "TestShared"}}
	if _, err := RenderCommand(provider.Run.Test, data); err == nil {
		t.Fatal("accepted stale Go test without package")
	}
	data.Test.GoPackage = "example.test/first"
	cmd, err := RenderCommand(provider.Run.Test, data)
	if err != nil || cmd.Args[1] != data.Test.GoPackage || cmd.Args[3] != "^TestShared$" {
		t.Fatalf("package-scoped command: %+v %v", cmd, err)
	}
}

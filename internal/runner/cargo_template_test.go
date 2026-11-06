package runner

import (
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"reflect"
	"testing"
)

func TestCargoTargetRenderingRequiresIdentity(t *testing.T) {
	provider := config.CargoProvider()
	tc := domain.TestCase{Selector: "mod::case", CargoPackage: "fixture@0.1.0", CargoTargetKind: "test", CargoTargetName: "integration"}
	command, err := RenderCommand(provider.Run.Test, TemplateData{Provider: provider, Test: &tc})
	want := []string{"test", "--package", "fixture@0.1.0", "--test", "integration", "mod::case", "--", "--exact"}
	if err != nil || !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("command %+v: %v", command.Args, err)
	}
	tc.CargoPackage = ""
	if _, err := RenderCommand(provider.Run.Test, TemplateData{Provider: provider, Test: &tc}); err == nil {
		t.Fatal("stale catalog was accepted")
	}
}

package manager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestBuiltInVitestTargetFailsClosedBeforeQueueing(t *testing.T) {
	provider := config.VitestProvider()
	m := &Manager{cfg: domain.ProjectConfig{Providers: []domain.Provider{provider}}, tests: map[string]domain.TestCase{
		"case":  {ID: "case", ProviderID: provider.ID, ProviderKind: provider.Kind, Name: "same [a]+", Selector: "same [a]+"},
		"other": {ID: "other", ProviderID: provider.ID, ProviderKind: provider.Kind, Name: "other", Selector: "other"},
	}}
	request := RunRequest{TestIDs: []string{"case"}}
	if items, err := m.Plan(request); err == nil || !strings.Contains(err.Error(), "no Vitest file") || len(items) != 0 {
		t.Fatalf("unsafe plan: %v, %v", items, err)
	}
	if run, err := m.Run(context.Background(), request); err == nil || run.ID != "" {
		t.Fatalf("unsafe run was queued: %+v, %v", run, err)
	}
	items, err := m.Plan(RunRequest{ProviderIDs: []string{provider.ID}})
	if err != nil || len(items) != 1 || items[0].TestID != "" || items[0].ProviderID != provider.ID {
		t.Fatalf("explicit provider-wide plan: %v, %v", items, err)
	}
	test := m.tests["case"]
	test.File = "example.test.js"
	m.tests["case"] = test
	if items, err := m.Plan(request); err != nil || len(items) != 1 || !strings.Contains(items[0].CommandPreview, "Vitest exact") {
		t.Fatalf("valid exact plan: %v, %v", items, err)
	}
	m.cfg.Providers[0].Run.Test.Args = []string{"custom-selection", "{{test.selector}}"}
	if items, err := m.Plan(request); err != nil || len(items) != 1 {
		t.Fatalf("custom command rejected: %v, %v", items, err)
	}
}

func TestVitestSelectedResultRequiresOneExactName(t *testing.T) {
	test := domain.TestCase{FullName: "suite > Case"}
	result := domain.TestResult{Name: "suite > Case", Status: domain.StatusPassed}
	if matched, status := vitestResultStatus(test, []domain.TestResult{result}); !matched || status != domain.StatusPassed {
		t.Fatalf("matching result: %v, %v", matched, status)
	}
	if matched, _ := vitestResultStatus(test, []domain.TestResult{{Name: "suite > case", Status: domain.StatusPassed}}); matched {
		t.Fatal("different case was accepted")
	}
	if matched, status := vitestResultStatus(test, []domain.TestResult{result, result}); !matched || status != domain.StatusUnknown {
		t.Fatalf("ambiguous results: %v, %v", matched, status)
	}
}

func TestVitestExactLauncherRejectsUnsupportedVersion(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is unavailable")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "node_modules", "vitest")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"vitest","version":"3.2.4","exports":{"./package.json":"./package.json"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	provider := config.VitestProvider()
	command, err := vitestExactCommand(provider, &domain.TestCase{File: "example.test.js", Selector: "test"}, root, filepath.Join(root, "result.xml"))
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(command.Exe, command.Args...)
	process.Dir = command.WorkingDir
	output, err := process.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "requires Vitest 4") {
		t.Fatalf("unsupported version: %s, %v", output, err)
	}
}

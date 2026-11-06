package manager

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

//go:embed vitest_exact.mjs
var vitestExactScript string

func vitestExactCommand(provider domain.Provider, test *domain.TestCase, projectRoot, artifactPath string) (domain.CommandSpec, error) {
	if test == nil || test.File == "" || test.Selector == "" {
		return domain.CommandSpec{}, fmt.Errorf("Vitest test identity is incomplete; rediscover tests")
	}
	command, err := runner.RenderCommand(provider.Run.Test, runner.TemplateData{ProjectRoot: projectRoot, Provider: provider, Test: test, ArtifactPath: artifactPath})
	if err != nil {
		return domain.CommandSpec{}, err
	}
	command.Exe = "node"
	command.Args = []string{"--input-type=module", "--eval", vitestExactScript, "--", test.File, test.VitestProject, test.Selector, artifactPath}
	return command, nil
}

func vitestResultStatus(test domain.TestCase, results []domain.TestResult) (bool, domain.Status) {
	var match *domain.TestResult
	for i := range results {
		if results[i].Name != test.FullName {
			continue
		}
		if match != nil {
			return true, domain.StatusUnknown
		}
		match = &results[i]
	}
	if match == nil {
		return false, domain.StatusUnknown
	}
	return true, match.Status
}

// The original generated name-only command has no exact file or project
// boundary. Recognize it so execution can use the isolated Vitest launcher;
// custom commands remain under the provider author's control.
func isUnsafeBuiltInVitestTarget(provider domain.Provider) bool {
	if provider.Kind != domain.ProviderVitest {
		return false
	}
	spec := provider.Run.Test
	exe := strings.ToLower(filepath.Base(spec.Exe))
	if exe != "npx" && exe != "npx.cmd" && exe != "npx.exe" {
		return false
	}
	args := []string{"vitest", "run", "-t", "{{test.selector}}", "--reporter=default", "--reporter=junit", "--outputFile={{artifact.path}}"}
	if len(spec.Args) != len(args) {
		return false
	}
	for i := range args {
		if spec.Args[i] != args[i] {
			return false
		}
	}
	return true
}

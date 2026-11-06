package manager

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func isBuiltInJestTarget(provider domain.Provider) bool {
	generated := config.JestProvider()
	return provider.Kind == domain.ProviderJest && provider.Results == generated.Results && reflect.DeepEqual(provider.Run.Test, generated.Run.Test)
}

func jestResultStatus(test domain.TestCase, results []domain.TestResult, workingDir string) (bool, domain.Status) {
	if test.Selector == "" || len(results) == 0 {
		return false, domain.StatusUnknown
	}
	resolve := func(path string) string {
		path = filepath.FromSlash(path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		path = filepath.Clean(path)
		if runtime.GOOS == "windows" {
			path = strings.ToLower(path)
		}
		return path
	}
	expected := resolve(test.Selector)
	status := domain.StatusSkipped
	for _, result := range results {
		if result.File == "" || resolve(result.File) != expected {
			return false, domain.StatusUnknown
		}
		switch result.Status {
		case domain.StatusFailed:
			status = domain.StatusFailed
		case domain.StatusPassed:
			if status != domain.StatusFailed {
				status = domain.StatusPassed
			}
		case domain.StatusSkipped:
		default:
			return false, domain.StatusUnknown
		}
	}
	return true, status
}

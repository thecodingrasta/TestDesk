package manager

import (
	"path/filepath"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestJestResultStatusRequiresOnlySelectedFile(t *testing.T) {
	root := t.TempDir()
	selected := domain.TestCase{Selector: filepath.Join(root, "one.test.js")}
	for _, tc := range []struct {
		name    string
		results []domain.TestResult
		matched bool
		status  domain.Status
	}{
		{"pass", []domain.TestResult{{File: "one.test.js", Status: domain.StatusPassed}}, true, domain.StatusPassed},
		{"mixed outcomes", []domain.TestResult{{File: "one.test.js", Status: domain.StatusPassed}, {File: "one.test.js", Status: domain.StatusFailed}}, true, domain.StatusFailed},
		{"other file", []domain.TestResult{{File: "other.test.js", Status: domain.StatusPassed}}, false, domain.StatusUnknown},
		{"mixed files", []domain.TestResult{{File: "one.test.js", Status: domain.StatusPassed}, {File: "other.test.js", Status: domain.StatusPassed}}, false, domain.StatusUnknown},
		{"missing file", []domain.TestResult{{Status: domain.StatusPassed}}, false, domain.StatusUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matched, status := jestResultStatus(selected, tc.results, root)
			if matched != tc.matched || status != tc.status {
				t.Fatalf("got %v %s", matched, status)
			}
		})
	}
}

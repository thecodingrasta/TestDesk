package results

import (
	"bufio"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"strings"
)

// ParseCargoText reads libtest terminal case lines. A successful process with
// no case lines remains empty so callers cannot mistake zero matches for a pass.
func ParseCargoText(stdout string) ([]domain.TestResult, error) {
	var parsed []domain.TestResult
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "test ") {
			continue
		}
		name, outcome, ok := strings.Cut(strings.TrimPrefix(line, "test "), " ... ")
		if !ok || name == "" {
			continue
		}
		status := domain.StatusUnknown
		switch {
		case outcome == "ok":
			status = domain.StatusPassed
		case outcome == "FAILED":
			status = domain.StatusFailed
		case outcome == "ignored" || strings.HasPrefix(outcome, "ignored, "):
			status = domain.StatusSkipped
		default:
			continue
		}
		parsed = append(parsed, domain.TestResult{Name: name, FullName: name, Selector: name, Status: status})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return parsed, nil
}

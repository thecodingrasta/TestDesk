package results

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func Parse(format string, path string, stdout string) ([]domain.TestResult, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "none":
		return []domain.TestResult{}, nil
	case "junit":
		return ParseJUnitFile(path)
	case "junit-dir":
		return ParseJUnitDir(path)
	case "trx":
		return ParseTRXFile(path)
	case "trx-dir":
		return ParseTRXDir(path)
	case "vitest-json", "json":
		return ParseVitestJSONFile(path)
	case "go-json", "go-json-stream":
		return ParseGoJSON(stdout)
	case "cargo-text":
		return ParseCargoText(stdout)
	default:
		return nil, fmt.Errorf("unsupported result format %q", format)
	}
}

type junitTestsuite struct {
	XMLName   xml.Name         `xml:"testsuite"`
	Name      string           `xml:"name,attr"`
	TestCases []junitTestcase  `xml:"testcase"`
	Suites    []junitTestsuite `xml:"testsuite"`
}

type junitTestsuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestsuite `xml:"testsuite"`
}

type junitTestcase struct {
	ClassName string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	File      string        `xml:"file,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure"`
	Error     *junitFailure `xml:"error"`
	Skipped   *junitSkipped `xml:"skipped"`
	SystemOut string        `xml:"system-out"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

func readReportFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("result report %s is empty", path)
	}
	return data, nil
}

func ParseJUnitFile(path string) ([]domain.TestResult, error) {
	data, err := readReportFile(path)
	if err != nil || len(data) == 0 {
		return []domain.TestResult{}, err
	}
	return ParseJUnit(data)
}

func ParseJUnitDir(path string) ([]domain.TestResult, error) {
	var results []domain.TestResult
	if strings.TrimSpace(path) == "" {
		return results, nil
	}
	err := filepath.WalkDir(path, func(item string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(item), ".xml") {
			return err
		}
		parsed, parseErr := ParseJUnitFile(item)
		if parseErr != nil {
			return parseErr
		}
		results = append(results, parsed...)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return []domain.TestResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	return results, nil
}

func ParseJUnit(data []byte) ([]domain.TestResult, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return []domain.TestResult{}, nil
	}
	var suites junitTestsuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.Suites) > 0 {
		var results []domain.TestResult
		for _, suite := range suites.Suites {
			results = append(results, suite.toResults()...)
		}
		return results, nil
	}
	var suite junitTestsuite
	if err := xml.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	return suite.toResults(), nil
}

func (s junitTestsuite) toResults() []domain.TestResult {
	var results []domain.TestResult
	for _, nested := range s.Suites {
		results = append(results, nested.toResults()...)
	}
	for _, test := range s.TestCases {
		status := domain.StatusPassed
		message := ""
		if test.Skipped != nil {
			status = domain.StatusSkipped
			message = test.Skipped.Message
		}
		if test.Failure != nil {
			status = domain.StatusFailed
			message = strings.TrimSpace(test.Failure.Message + "\n" + test.Failure.Text)
		}
		if test.Error != nil {
			status = domain.StatusFailed
			if message != "" {
				message += "\n" + strings.TrimSpace(test.Error.Message+"\n"+test.Error.Text)
			} else {
				message = strings.TrimSpace(test.Error.Message + "\n" + test.Error.Text)
			}
		}
		fullName := strings.Trim(strings.TrimSpace(test.ClassName)+"."+strings.TrimSpace(test.Name), ".")
		results = append(results, domain.TestResult{
			Name:           test.Name,
			File:           test.File,
			FullName:       fullName,
			Selector:       fullName,
			Status:         status,
			DurationMillis: secondsToMillis(test.Time),
			Message:        message,
			Output:         test.SystemOut,
		})
	}
	return results
}

type trxTestRun struct {
	XMLName xml.Name            `xml:"TestRun"`
	Results []trxUnitTestResult `xml:"Results>UnitTestResult"`
}

// ParseTRXDir reads every report produced by a multi-project or multi-target run.
func ParseTRXDir(path string) ([]domain.TestResult, error) {
	var all []domain.TestResult
	err := filepath.WalkDir(path, func(item string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".trx") {
			return nil
		}
		parsed, err := ParseTRXFile(item)
		if err != nil {
			return fmt.Errorf("parse TRX %s: %w", item, err)
		}
		all = append(all, parsed...)
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	return all, nil
}

type trxUnitTestResult struct {
	TestName string `xml:"testName,attr"`
	Outcome  string `xml:"outcome,attr"`
	Duration string `xml:"duration,attr"`
	Output   struct {
		StdOut string `xml:"StdOut"`
		Error  string `xml:"ErrorInfo>Message"`
		Stack  string `xml:"ErrorInfo>StackTrace"`
	} `xml:"Output"`
}

func ParseTRXFile(path string) ([]domain.TestResult, error) {
	data, err := readReportFile(path)
	if err != nil || len(data) == 0 {
		return []domain.TestResult{}, err
	}
	var run trxTestRun
	if err := xml.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	results := make([]domain.TestResult, 0, len(run.Results))
	for _, item := range run.Results {
		status := domain.StatusPassed
		switch strings.ToLower(item.Outcome) {
		case "passed":
			status = domain.StatusPassed
		case "failed", "error", "timeout":
			status = domain.StatusFailed
		case "notexecuted", "skipped":
			status = domain.StatusSkipped
		default:
			status = domain.StatusUnknown
		}
		results = append(results, domain.TestResult{Name: item.TestName, FullName: item.TestName, Selector: item.TestName, Status: status, DurationMillis: durationToMillis(item.Duration), Message: strings.TrimSpace(item.Output.Error + "\n" + item.Output.Stack), Output: item.Output.StdOut})
	}
	return results, nil
}

func ParseVitestJSONFile(path string) ([]domain.TestResult, error) {
	data, err := readReportFile(path)
	if err != nil || len(data) == 0 {
		return []domain.TestResult{}, err
	}
	var report struct {
		TestResults []struct {
			Name             string `json:"name"`
			AssertionResults []struct {
				Title           string   `json:"title"`
				FullName        string   `json:"fullName"`
				Status          string   `json:"status"`
				Duration        int64    `json:"duration"`
				FailureMessages []string `json:"failureMessages"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	var results []domain.TestResult
	for _, file := range report.TestResults {
		for _, assertion := range file.AssertionResults {
			status := domain.StatusPassed
			switch strings.ToLower(assertion.Status) {
			case "passed", "success":
				status = domain.StatusPassed
			case "failed", "failure":
				status = domain.StatusFailed
			case "skipped", "pending", "todo":
				status = domain.StatusSkipped
			default:
				status = domain.StatusUnknown
			}
			fullName := assertion.FullName
			if fullName == "" {
				fullName = assertion.Title
			}
			results = append(results, domain.TestResult{Name: assertion.Title, FullName: fullName, Selector: fullName, Status: status, DurationMillis: assertion.Duration, Message: strings.Join(assertion.FailureMessages, "\n")})
		}
	}
	return results, nil
}

func ParseGoJSON(stdout string) ([]domain.TestResult, error) {
	var results []domain.TestResult
	benchmarks := map[string]*goBenchmarkEvidence{}
	passedPackages := map[string]int{}
	for index, rawLine := range strings.Split(stdout, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event struct {
			Action  string  `json:"Action"`
			Package string  `json:"Package"`
			Test    string  `json:"Test"`
			Elapsed float64 `json:"Elapsed"`
			Output  string  `json:"Output"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("invalid Go test event: %w", err)
		}
		if event.Test == "" {
			if event.Action == "pass" && event.Package != "" {
				passedPackages[event.Package] = index + 1
			} else if event.Action == "fail" {
				delete(passedPackages, event.Package)
			}
			continue
		}
		if strings.HasPrefix(event.Test, "Benchmark") && event.Package != "" {
			key := event.Package + "\x00" + event.Test
			benchmark := benchmarks[key]
			if benchmark == nil {
				benchmark = &goBenchmarkEvidence{pkg: event.Package, name: event.Test}
				benchmarks[key] = benchmark
			}
			benchmark.lastEvent = index + 1
			if event.Action == "run" {
				benchmark.started = true
			} else if event.Action == "output" {
				benchmark.addOutput(event.Output)
			} else if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
				benchmark.terminal = true
				switch event.Action {
				case "pass":
					benchmark.status = domain.StatusPassed
				case "fail":
					benchmark.status = domain.StatusFailed
				case "skip":
					benchmark.status = domain.StatusSkipped
				}
			}
		}
		status := domain.StatusUnknown
		switch event.Action {
		case "pass":
			status = domain.StatusPassed
		case "fail":
			status = domain.StatusFailed
		case "skip":
			status = domain.StatusSkipped
		default:
			continue
		}
		fullName := strings.Trim(event.Package+"."+event.Test, ".")
		results = append(results, domain.TestResult{Name: event.Test, FullName: fullName, Selector: event.Test, Status: status, DurationMillis: int64(event.Elapsed * 1000), Output: event.Output})
	}
	// test2json emits benchmark measurements as output rather than a test-level
	// pass. Require a real measurement and the enclosing package's pass event.
	names := make([]string, 0, len(benchmarks))
	children := make(map[string][]*goBenchmarkEvidence)
	for key, benchmark := range benchmarks {
		if benchmark.started && !benchmark.terminal && passedPackages[benchmark.pkg] > benchmark.lastEvent && benchmark.measured {
			benchmark.status = domain.StatusPassed
		}
		names = append(names, key)
		if index := strings.LastIndex(benchmark.name, "/"); index >= 0 {
			parent := benchmark.pkg + "\x00" + benchmark.name[:index]
			children[parent] = append(children[parent], benchmark)
		}
	}
	// Sub-benchmarks produce measurements for children, not their parent. A
	// parent is conclusive only if every observed child is conclusive.
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, key := range names {
		benchmark := benchmarks[key]
		if !benchmark.started || benchmark.terminal || passedPackages[benchmark.pkg] <= benchmark.lastEvent {
			continue
		}
		if benchmark.status == "" {
			found, unknown, passed := false, false, false
			for _, child := range children[key] {
				if !child.started {
					continue
				}
				found = true
				if passedPackages[benchmark.pkg] <= child.lastEvent {
					unknown = true
				} else if child.status == domain.StatusPassed {
					passed = true
				} else if child.status != domain.StatusSkipped {
					unknown = true
				}
			}
			if found && !unknown {
				benchmark.status = domain.StatusSkipped
				if passed {
					benchmark.status = domain.StatusPassed
				}
			}
		}
		if benchmark.status != "" {
			results = append(results, domain.TestResult{Name: benchmark.name, FullName: benchmark.pkg + "." + benchmark.name, Selector: benchmark.name, Status: benchmark.status, Output: benchmark.measurement})
		}
	}
	return results, nil
}

type goBenchmarkEvidence struct {
	pkg, name   string
	pending     strings.Builder
	measurement string
	started     bool
	lastEvent   int
	terminal    bool
	measured    bool
	discardLine bool
	status      domain.Status
}

func (benchmark *goBenchmarkEvidence) addOutput(output string) {
	const maxBenchmarkLine = 8192
	for len(output) > 0 {
		index := strings.IndexByte(output, '\n')
		part := output
		if index >= 0 {
			part = output[:index]
		}
		if !benchmark.discardLine {
			if benchmark.pending.Len()+len(part) > maxBenchmarkLine {
				benchmark.pending.Reset()
				benchmark.discardLine = true
			} else {
				benchmark.pending.WriteString(part)
			}
		}
		if index < 0 {
			return
		}
		if !benchmark.discardLine && hasGoBenchmarkMeasurement(benchmark.name, benchmark.pending.String()) {
			benchmark.measured = true
			benchmark.measurement = benchmark.pending.String()
		}
		benchmark.pending.Reset()
		benchmark.discardLine = false
		output = output[index+1:]
	}
}

func hasGoBenchmarkMeasurement(name, output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != "ns/op" {
			continue
		}
		label := fields[0]
		if label != name {
			if !strings.HasPrefix(label, name+"-") {
				continue
			}
			if _, err := strconv.ParseUint(strings.TrimPrefix(label, name+"-"), 10, 32); err != nil {
				continue
			}
		}
		iterations, iterationsErr := strconv.ParseUint(fields[1], 10, 64)
		measurement, measurementErr := strconv.ParseFloat(fields[2], 64)
		if iterationsErr == nil && iterations > 0 && measurementErr == nil && measurement >= 0 && !math.IsInf(measurement, 0) {
			return true
		}
	}
	return false
}

func secondsToMillis(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var seconds float64
	_, _ = fmt.Sscanf(value, "%f", &seconds)
	return int64(seconds * 1000)
}

func durationToMillis(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration.Milliseconds()
	}
	// TRX commonly uses HH:MM:SS.fffffff.
	parts := strings.Split(value, ":")
	if len(parts) == 3 {
		var hours, minutes float64
		var seconds float64
		_, _ = fmt.Sscanf(parts[0], "%f", &hours)
		_, _ = fmt.Sscanf(parts[1], "%f", &minutes)
		_, _ = fmt.Sscanf(parts[2], "%f", &seconds)
		return int64(((hours*60+minutes)*60 + seconds) * 1000)
	}
	return 0
}

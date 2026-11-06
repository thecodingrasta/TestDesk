package discovery

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// Parse converts native framework discovery output into TestCase descriptors.
func Parse(provider domain.Provider, stdout string) ([]domain.TestCase, error) {
	parser := strings.TrimSpace(strings.ToLower(provider.Discover.Parser))
	switch parser {
	case "", "none":
		return nil, nil
	case "lines":
		return parseLines(provider, stdout), nil
	case "dotnet-list":
		for _, line := range strings.Split(stdout, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "Microsoft.VisualStudio.TestPlatform.ObjectModel.TestPlatformException:") {
				return nil, fmt.Errorf(".NET test discovery reported a test platform exception")
			}
		}
		return parseDotNetList(provider, stdout), nil
	case "playwright-list":
		return parsePlaywrightList(provider, stdout), nil
	case "vitest-json":
		return parseVitestJSON(provider, stdout)
	case "go-list":
		return parseGoList(provider, stdout), nil
	case "go-json-list":
		return parseGoJSONList(provider, stdout)
	case "pytest-collect":
		return parsePytestCollect(provider, stdout), nil
	case "cargo-list":
		return parseCargoList(provider, stdout), nil
	default:
		return nil, fmt.Errorf("unsupported discovery parser %q", provider.Discover.Parser)
	}
}

// The complete output is already in memory. Allow a single line to span it so
// Scanner cannot silently stop at its default 64 KiB token limit.
func discoveryScanner(stdout string) *bufio.Scanner {
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 4096), len(stdout)+1)
	return scanner
}

func parseLines(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	scanner := discoveryScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tests = append(tests, newCase(provider, line, line, line, "", 0, nil))
	}
	return sortCases(tests)
}

func parseDotNetList(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	seenCount := map[string]int{}
	scanner := discoveryScanner(stdout)
	collecting := false
	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "following tests") || strings.Contains(lower, "available tests") {
			collecting = true
			continue
		}
		if strings.HasPrefix(lower, "test run") ||
			strings.HasPrefix(lower, "passed") ||
			strings.HasPrefix(lower, "failed") ||
			strings.HasPrefix(lower, "total tests") ||
			strings.HasPrefix(lower, "build ") ||
			strings.HasPrefix(lower, "determining projects") {
			continue
		}
		if (collecting || strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t")) && dotNetTestIdentifier(line) {
			fullName := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			seenCount[fullName]++
			tc := newCase(provider, dotNetDisplayName(fullName), fullName, fullName, "", 0, nil)
			if count := seenCount[fullName]; count > 1 {
				tc.ID = stableID(provider.ID, fullName, "", strconv.Itoa(count))
			}
			tests = append(tests, tc)
		}
	}
	return sortCases(tests)
}

func dotNetTestIdentifier(value string) bool {
	trimmed := strings.TrimSpace(strings.TrimPrefix(value, "-"))
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, ": warning ") ||
		strings.Contains(lower, ": error ") ||
		strings.Contains(lower, " -> ") ||
		strings.HasPrefix(lower, "determining projects to restore") ||
		strings.HasPrefix(lower, "all projects are up-to-date for restore") ||
		strings.HasPrefix(lower, "restoring packages") ||
		strings.HasPrefix(lower, "restore completed") ||
		strings.Contains(lower, "build succeeded") ||
		strings.Contains(lower, "time elapsed") ||
		strings.HasPrefix(lower, "microsoft (r)") ||
		strings.HasPrefix(lower, "copyright") ||
		strings.HasPrefix(lower, "test run") ||
		strings.HasPrefix(lower, "passed!") ||
		strings.HasPrefix(lower, "failed!") ||
		strings.HasPrefix(lower, "total tests:") ||
		strings.HasPrefix(lower, "attachments:") ||
		strings.HasPrefix(lower, "starting test execution") ||
		strings.HasSuffix(lower, ".dll") ||
		strings.HasSuffix(lower, ".exe") ||
		strings.HasSuffix(lower, ".csproj") ||
		strings.HasSuffix(lower, ".csproj]") ||
		strings.HasSuffix(lower, ".props]") ||
		strings.HasSuffix(lower, ".targets]") {
		return false
	}
	return dotNetTestPattern.MatchString(trimmed)
}

var dotNetTestPattern = regexp.MustCompile(`^[A-Za-z_@` + "`" + `][A-Za-z0-9_.:\+` + "`" + `<>,\s-]*[.:][A-Za-z0-9_.:` + "`" + `<>,\s-]+(?:\([^\r\n]*\))?$`)

// IdentifyDotNetAdapters compares two adapter-rendered lists without trying to
// reconstruct names: the separator can also occur inside test arguments.
func IdentifyDotNetAdapters(normal, alternate []domain.TestCase) error {
	type candidate struct {
		name  string
		nunit bool
	}
	names := make(map[string]int)
	options := make(map[string][]candidate)
	for _, test := range normal {
		name := strings.ToLower(test.FullName)
		names[name]++
	}
	for name := range names {
		options[name] = append(options[name], candidate{name: name})
		converted := strings.ReplaceAll(name, ".", ":")
		if converted != name {
			options[converted] = append(options[converted], candidate{name: name, nunit: true})
		}
	}
	counts := make(map[string]int)
	identified := make(map[string]bool)
	for _, test := range alternate {
		matches := options[strings.ToLower(test.FullName)]
		if len(matches) != 1 {
			return fmt.Errorf("ambiguous or unmatched .NET adapter test identity %q", test.FullName)
		}
		match := matches[0]
		if old, ok := identified[match.name]; ok && old != match.nunit {
			return fmt.Errorf("inconsistent .NET adapter identity %q", match.name)
		}
		identified[match.name] = match.nunit
		counts[match.name]++
	}
	for name, count := range names {
		if counts[name] != count {
			return fmt.Errorf("incomplete .NET adapter identities for %q", name)
		}
	}
	for i := range normal {
		if identified[strings.ToLower(normal[i].FullName)] {
			normal[i].DotNetFilterProperty = "FullyQualifiedName"
		}
	}
	return nil
}

func dotNetDisplayName(fullName string) string {
	identifier := fullName
	if index := strings.Index(identifier, "("); index >= 0 {
		identifier = identifier[:index]
	}
	if index := strings.LastIndex(identifier, "."); index >= 0 && index+1 < len(identifier) {
		return identifier[index+1:]
	}
	return identifier
}

// IdentifyStreamedTestLine identifies if a live stdout line from a test runner represents a discovered test case.
func IdentifyStreamedTestLine(provider domain.Provider, rawLine string) (bool, string) {
	line := strings.TrimSpace(rawLine)
	if line == "" {
		return false, ""
	}
	parser := strings.ToLower(provider.Discover.Parser)
	kind := strings.ToLower(string(provider.Kind))

	if parser == "go-json-list" {
		var event struct {
			Action string `json:"Action"`
			Output string `json:"Output"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || event.Action != "output" {
			return false, ""
		}
		name := strings.TrimSpace(event.Output)
		if token.IsIdentifier(name) && (strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Benchmark")) {
			return true, name
		}
		return false, ""
	}

	if parser == "dotnet-list" || kind == string(domain.ProviderDotNet) {
		if (strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t")) && dotNetTestIdentifier(line) {
			fullName := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			return true, dotNetDisplayName(fullName)
		}
	} else if parser == "go-list" || kind == string(domain.ProviderGo) {
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") || strings.HasPrefix(line, "Benchmark") {
			return true, line
		}
	} else if strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t") {
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "error") && !strings.HasPrefix(lower, "warn") && !strings.Contains(lower, "completed") && !strings.Contains(lower, "passed") {
			return true, line
		}
	}
	return false, ""
}

var playwrightLine = regexp.MustCompile(`^(?:\[(?P<project>[^\]]+)\]\s*›\s*)?(?P<file>[^›]+?)(?::(?P<line>\d+)(?::\d+)?)?\s*›\s*(?P<name>.+)$`)

func parsePlaywrightList(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	scanner := discoveryScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Listing tests") || strings.HasPrefix(line, "Total:") {
			continue
		}
		matches := namedMatches(playwrightLine, line)
		if len(matches) == 0 {
			continue
		}
		file := strings.TrimSpace(matches["file"])
		name := strings.TrimSpace(matches["name"])
		if name == "" {
			continue
		}
		lineNo, _ := strconv.Atoi(matches["line"])
		selector := file
		if lineNo > 0 {
			selector = fmt.Sprintf("%s:%d", file, lineNo)
		}
		tags := []string{}
		if matches["project"] != "" {
			tags = append(tags, matches["project"])
		}
		tests = append(tests, newCase(provider, lastPlaywrightName(name), line, selector, file, lineNo, tags))
	}
	return sortCases(tests)
}

func parseVitestJSON(provider domain.Provider, stdout string) ([]domain.TestCase, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, fmt.Errorf("Vitest discovery returned empty JSON output")
	}

	// Vitest list --json has changed shape over time. Decode into interface and
	// recursively mine objects that contain name/taskName/fullName/filepath.
	var root any
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return nil, fmt.Errorf("invalid Vitest discovery JSON: %w", err)
	}
	if records, ok := root.([]any); ok {
		for _, record := range records {
			if entry, ok := record.(map[string]any); ok && entry["file"] != nil && entry["type"] == nil {
				return parseVitestFlat(provider, records)
			}
		}
	}
	var tests []domain.TestCase
	walkVitest(root, provider, nil, &tests)
	if len(tests) == 0 {
		if items, ok := root.([]any); !ok || len(items) != 0 {
			return nil, fmt.Errorf("unrecognized Vitest discovery JSON; catalog retained")
		}
	}
	return sortCases(dedupeCases(tests)), nil
}

func parseVitestFlat(provider domain.Provider, records []any) ([]domain.TestCase, error) {
	tests := make([]domain.TestCase, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		entry, ok := record.(map[string]any)
		if !ok || entry["type"] != nil {
			return nil, fmt.Errorf("invalid Vitest flat discovery record")
		}
		name, nameOK := entry["name"].(string)
		file, fileOK := entry["file"].(string)
		project, projectOK := entry["projectName"].(string)
		if !nameOK || !fileOK || strings.TrimSpace(name) == "" || strings.TrimSpace(file) == "" || (entry["projectName"] != nil && !projectOK) {
			return nil, fmt.Errorf("Vitest discovery omitted test identity")
		}
		line := 0
		if location, ok := entry["location"].(map[string]any); ok {
			line = int(firstNumber(location, "line"))
		}
		test := newCase(provider, name, name, name, file, line, nil)
		test.VitestProject = project
		identity, err := json.Marshal([]string{provider.ID, name, file, project})
		if err != nil {
			return nil, fmt.Errorf("encode Vitest test identity: %w", err)
		}
		test.ID = stableID(string(identity))
		if seen[test.ID] {
			return nil, fmt.Errorf("Vitest discovery returned duplicate test identities for %q in %q", name, file)
		}
		seen[test.ID] = true
		tests = append(tests, test)
	}
	return sortCases(tests), nil
}

func walkVitest(node any, provider domain.Provider, parents []string, tests *[]domain.TestCase) {
	switch value := node.(type) {
	case []any:
		for _, item := range value {
			walkVitest(item, provider, parents, tests)
		}
	case map[string]any:
		name := firstString(value, "name", "taskName", "testName")
		typeValue := strings.ToLower(firstString(value, "type", "mode"))
		filepathValue := firstString(value, "filepath", "file", "path")
		lineNo := int(firstNumber(value, "line", "startLine"))
		if name != "" {
			newParents := append(append([]string{}, parents...), name)
			if typeValue == "test" || strings.EqualFold(firstString(value, "suite"), "false") || hasAnyKey(value, "result", "errors", "duration") {
				fullName := strings.Join(newParents, " > ")
				selector := fullName
				if filepathValue != "" {
					selector = fullName
				}
				*tests = append(*tests, newCase(provider, name, fullName, selector, filepathValue, lineNo, nil))
			}
			parents = newParents
		}
		for key, child := range value {
			if key == "name" || key == "taskName" || key == "testName" {
				continue
			}
			walkVitest(child, provider, parents, tests)
		}
	}
}

func parseGoList(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	scanner := discoveryScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Benchmark") || strings.HasPrefix(line, "Example") || strings.HasPrefix(line, "Fuzz") {
			tests = append(tests, newCase(provider, line, line, line, "", 0, nil))
		}
	}
	return sortCases(tests)
}

func parseGoJSONList(provider domain.Provider, stdout string) ([]domain.TestCase, error) {
	var tests []domain.TestCase
	packages := make(map[string]bool)
	outputs := make(map[string]*strings.Builder)
	for _, raw := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var event struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Test    string `json:"Test"`
			Output  string `json:"Output"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("invalid Go discovery event: %w", err)
		}
		if event.Action == "build-output" {
			continue
		}
		if event.Package == "" {
			return nil, fmt.Errorf("Go discovery event %q omitted package", event.Action)
		}
		if event.Test != "" {
			return nil, fmt.Errorf("unexpected test-level Go discovery event for %q", event.Test)
		}
		finished, started := packages[event.Package]
		switch event.Action {
		case "start":
			if started {
				return nil, fmt.Errorf("Go discovery repeated package %q", event.Package)
			}
			packages[event.Package] = false
			outputs[event.Package] = &strings.Builder{}
			continue
		case "pass", "skip":
			if !started || finished {
				return nil, fmt.Errorf("unexpected Go discovery completion for %q", event.Package)
			}
			packages[event.Package] = true
			continue
		case "output":
			if !started || finished {
				return nil, fmt.Errorf("Go discovery output outside package %q", event.Package)
			}
		default:
			return nil, fmt.Errorf("unexpected Go discovery action %q", event.Action)
		}
		outputs[event.Package].WriteString(event.Output)
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("Go discovery did not report any packages")
	}
	for pkg, finished := range packages {
		if !finished {
			return nil, fmt.Errorf("Go discovery did not finish package %q", pkg)
		}
		for _, output := range strings.Split(outputs[pkg].String(), "\n") {
			name := strings.TrimSpace(output)
			if !token.IsIdentifier(name) || (!strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Example") && !strings.HasPrefix(name, "Fuzz") && !strings.HasPrefix(name, "Benchmark")) {
				continue
			}
			test := newCase(provider, name, pkg+"."+name, name, "", 0, nil)
			test.GoPackage = pkg
			tests = append(tests, test)
		}
	}
	return sortCases(tests), nil
}

func parsePytestCollect(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	scanner := discoveryScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "collected") || strings.HasPrefix(line, "=") {
			continue
		}
		if strings.Contains(line, "::") {
			tests = append(tests, newCase(provider, lastPytestName(line), line, line, strings.Split(line, "::")[0], 0, nil))
		}
	}
	return sortCases(tests)
}

func parseCargoList(provider domain.Provider, stdout string) []domain.TestCase {
	var tests []domain.TestCase
	scanner := discoveryScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, ": benchmark") {
			continue
		}
		if idx := strings.Index(line, ": test"); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			tests = append(tests, newCase(provider, lastSegment(name), name, name, "", 0, nil))
		}
	}
	return sortCases(tests)
}

func newCase(provider domain.Provider, name string, fullName string, selector string, file string, line int, tags []string) domain.TestCase {
	module := inferModule(file, fullName)
	allTags := append([]string{}, provider.Tags...)
	allTags = append(allTags, tags...)
	return domain.TestCase{
		ID:           stableID(provider.ID, selector, fullName, file),
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ProviderKind: provider.Kind,
		Name:         strings.TrimSpace(name),
		FullName:     strings.TrimSpace(fullName),
		Selector:     strings.TrimSpace(selector),
		File:         strings.TrimSpace(file),
		Line:         line,
		Layer:        provider.Layer,
		Module:       module,
		Tags:         uniqueStrings(allTags),
		LastStatus:   domain.StatusUnknown,
		DiscoveredAt: time.Now().UTC(),
	}
}

func stableID(parts ...string) string {
	hash := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(hash[:])[:16]
}

func sortCases(tests []domain.TestCase) []domain.TestCase {
	sort.SliceStable(tests, func(i, j int) bool {
		if tests[i].ProviderID != tests[j].ProviderID {
			return tests[i].ProviderID < tests[j].ProviderID
		}
		return tests[i].FullName < tests[j].FullName
	})
	return tests
}

func dedupeCases(tests []domain.TestCase) []domain.TestCase {
	seen := map[string]domain.TestCase{}
	for _, test := range tests {
		seen[test.ID] = test
	}
	result := make([]domain.TestCase, 0, len(seen))
	for _, test := range seen {
		result = append(result, test)
	}
	return result
}

func namedMatches(re *regexp.Regexp, text string) map[string]string {
	matches := re.FindStringSubmatch(text)
	if len(matches) == 0 {
		return nil
	}
	result := map[string]string{}
	for idx, name := range re.SubexpNames() {
		if idx == 0 || name == "" {
			continue
		}
		result[name] = matches[idx]
	}
	return result
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if item, ok := value[key]; ok {
			switch typed := item.(type) {
			case string:
				return typed
			case fmt.Stringer:
				return typed.String()
			}
		}
	}
	return ""
}

func firstNumber(value map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if item, ok := value[key]; ok {
			if number, ok := item.(float64); ok {
				return number
			}
		}
	}
	return 0
}

func hasAnyKey(value map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
}

func lastSegment(value string) string {
	value = strings.TrimSpace(value)
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '/' || r == '\\' })
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}

func lastPlaywrightName(value string) string {
	parts := strings.Split(value, "›")
	value = strings.TrimSpace(parts[len(parts)-1])
	parts = strings.Split(value, ">")
	return strings.TrimSpace(parts[len(parts)-1])
}

func lastPytestName(value string) string {
	parts := strings.Split(value, "::")
	return strings.TrimSpace(parts[len(parts)-1])
}

func inferModule(file string, fullName string) string {
	if candidate := strings.Trim(strings.TrimSpace(filepath.ToSlash(file)), "/"); candidate != "" {
		parts := strings.Split(candidate, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
		return strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
	}

	identifier := strings.TrimSpace(fullName)
	if index := strings.Index(identifier, "("); index >= 0 {
		identifier = identifier[:index]
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "general"
	}

	if parts := strings.Split(identifier, "::"); len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	if parts := strings.FieldsFunc(identifier, func(value rune) bool {
		return value == '/' || value == '\\'
	}); len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	if parts := strings.Split(identifier, "."); len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return "general"
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

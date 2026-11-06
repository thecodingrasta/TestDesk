package config

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

const FileName = "testdesk.json"

// Load reads testdesk.json from the project root and normalises default values.
func Load(projectRoot string) (domain.ProjectConfig, error) {
	configPath := filepath.Join(projectRoot, FileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return domain.ProjectConfig{}, err
	}

	var cfg domain.ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return domain.ProjectConfig{}, fmt.Errorf("parse %s: %w", configPath, err)
	}
	cfg = Normalise(projectRoot, cfg)
	detected := Detect(projectRoot).Providers
	cfg.Providers = mergeDetectedProviders(cfg.Providers, detected)
	if err := Validate(cfg); err != nil {
		return domain.ProjectConfig{}, err
	}
	return cfg, nil
}

// MergeDetectedProviders preserves configured providers while appending newly
// detected providers that do not duplicate an equivalent .NET scope.
func MergeDetectedProviders(configured []domain.Provider, detected []domain.Provider) []domain.Provider {
	return mergeDetectedProviders(configured, detected)
}

func mergeDetectedProviders(configured []domain.Provider, detected []domain.Provider) []domain.Provider {
	merged := append([]domain.Provider(nil), configured...)
	known := make(map[string]struct{}, len(configured))
	for _, provider := range configured {
		known[provider.ID] = struct{}{}
	}
	for _, provider := range detected {
		for i := range merged {
			if merged[i].ID == provider.ID {
				merged[i] = migrateSharedDirectoryDotNetProvider(merged[i], provider)
			}
		}
		if _, exists := known[provider.ID]; exists {
			continue
		}
		duplicate := false
		for _, existing := range configured {
			legacyMatches := 0
			if sameLegacyCollidingDotNetProvider(existing, provider) {
				for _, candidate := range detected {
					if sameLegacyCollidingDotNetProvider(existing, candidate) {
						legacyMatches++
					}
				}
			}
			if sameDetectedDotNetScope(existing, provider) || legacyMatches == 1 {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		merged = append(merged, provider)
		known[provider.ID] = struct{}{}
	}
	return merged
}

// Upgrade only the exact generated directory-wide commands when a directory
// now contains multiple project files. Custom commands remain untouched.
func migrateSharedDirectoryDotNetProvider(existing, detected domain.Provider) domain.Provider {
	if len(detected.Discover.Command.Args) != 4 || filepath.Ext(detected.Discover.Command.Args[1]) != ".csproj" {
		return existing
	}
	old := detected
	old.Discover.Command.Args = append([]string(nil), detected.Discover.Command.Args...)
	old.Run.All.Args = append([]string(nil), detected.Run.All.Args...)
	old.Run.Test.Args = append([]string(nil), detected.Run.Test.Args...)
	old.Discover.Command.Args[1] = "."
	old.Run.All.Args[1] = "."
	old.Run.Test.Args[1] = "."
	if sameDetectedDotNetScope(existing, old) {
		existing.Discover.Command = detected.Discover.Command
		existing.Run.All = detected.Run.All
		existing.Run.Test = detected.Run.Test
	}
	return existing
}

// A formerly unique auto-detected project may have been saved with the old
// basename ID and user overrides before a second project acquired that name.
func sameLegacyCollidingDotNetProvider(existing, detected domain.Provider) bool {
	if existing.Kind != domain.ProviderDotNet || detected.Kind != domain.ProviderDotNet {
		return false
	}
	if !strings.HasPrefix(detected.ID, existing.ID+"--") {
		return false
	}
	a, b := filepath.Clean(existing.Root), filepath.Clean(detected.Root)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Only suppress a detected provider when the configured commands and scope
// are equivalent. Custom adapters and explicitly configured providers remain.
func sameDetectedDotNetScope(a, b domain.Provider) bool {
	if a.Kind != domain.ProviderDotNet || b.Kind != domain.ProviderDotNet || a.Discover.Parser != b.Discover.Parser {
		return false
	}
	ar, br := filepath.Clean(a.Root), filepath.Clean(b.Root)
	if runtime.GOOS == "windows" {
		ar, br = strings.ToLower(ar), strings.ToLower(br)
	}
	if ar != br {
		return false
	}
	normalize := func(spec domain.CommandSpec) domain.CommandSpec {
		spec.Args = append([]string(nil), spec.Args...)
		for i, arg := range spec.Args {
			if arg == "{{provider.root}}" && filepath.Clean(a.Root) == "." {
				spec.Args[i] = "."
			}
		}
		return spec
	}
	return reflect.DeepEqual(normalize(a.Discover.Command), normalize(b.Discover.Command)) &&
		reflect.DeepEqual(normalize(a.Run.All), normalize(b.Run.All)) &&
		reflect.DeepEqual(normalize(a.Run.Test), normalize(b.Run.Test))
}

// Save writes a stable, human-editable configuration file.
func Save(projectRoot string, cfg domain.ProjectConfig) error {
	cfg = Normalise(projectRoot, cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	targetPath := filepath.Join(projectRoot, FileName)
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		if err := os.WriteFile(targetPath, append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	return ensureWorkspaceIgnored(projectRoot, cfg.Workspace)
}

// ensureWorkspaceIgnored keeps TestDesk's generated project state out of a
// repository only when that repository already has a .gitignore. It never
// creates or rewrites an ignore file owned by the project.
func ensureWorkspaceIgnored(projectRoot string, workspace string) error {
	ignorePath := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(ignorePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	entry := filepath.ToSlash(filepath.Clean(workspace))
	if entry == "." || filepath.IsAbs(workspace) {
		return nil
	}
	entry = strings.TrimPrefix(entry, "./")
	if !strings.HasSuffix(entry, "/") {
		entry += "/"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(strings.ReplaceAll(line, "\\", "/")) == entry {
			return nil
		}
	}
	content := strings.TrimRight(string(data), "\r\n")
	if content != "" {
		content += "\n\n"
	}
	content += "# TestDesk generated test state: https://github.com/thecodingrasta/testdesk\n" + entry + "\n"
	return os.WriteFile(ignorePath, []byte(content), 0o644)
}

// Exists checks whether a project already has TestDesk config.
func Exists(projectRoot string) bool {
	_, err := os.Stat(filepath.Join(projectRoot, FileName))
	return err == nil
}

// Init detects common frameworks and writes testdesk.json.
func Init(projectRoot string, force bool) (domain.ProjectConfig, error) {
	if Exists(projectRoot) && !force {
		return domain.ProjectConfig{}, fmt.Errorf("%s already exists; use --force to overwrite", FileName)
	}
	cfg := Detect(projectRoot)
	return cfg, Save(projectRoot, cfg)
}

func Normalise(projectRoot string, cfg domain.ProjectConfig) domain.ProjectConfig {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Workspace == "" {
		cfg.Workspace = ".testdesk"
	}
	if cfg.Name == "" {
		cfg.Name = filepath.Base(projectRoot)
	}
	if cfg.UI.Theme == "" {
		cfg.UI.Theme = "obsidian"
	}
	if cfg.UI.WidgetMode == "" {
		cfg.UI.WidgetMode = domain.VisibilityWhenActive
	}
	if cfg.UI.TopmostMode == "" {
		if cfg.UI.AlwaysOnTop {
			cfg.UI.TopmostMode = domain.VisibilityAlways
		} else {
			cfg.UI.TopmostMode = domain.VisibilityOff
		}
	}
	switch strings.ToLower(cfg.UI.WidgetSize) {
	case "small":
		cfg.UI.WidgetSize = "small"
		if cfg.UI.CompactWidth < 340 || cfg.UI.CompactWidth > 440 {
			cfg.UI.CompactWidth = 380
		}
		if cfg.UI.CompactHeight < 85 || cfg.UI.CompactHeight > 130 {
			cfg.UI.CompactHeight = 96
		}
		cfg.UI.CompactExpandedHeight = cfg.UI.CompactHeight + 84
	case "large":
		cfg.UI.WidgetSize = "large"
		if cfg.UI.CompactWidth < 540 || cfg.UI.CompactWidth > 680 {
			cfg.UI.CompactWidth = 580
		}
		if cfg.UI.CompactHeight < 110 || cfg.UI.CompactHeight > 170 {
			cfg.UI.CompactHeight = 118
		}
		cfg.UI.CompactExpandedHeight = cfg.UI.CompactHeight + 112
	default:
		cfg.UI.WidgetSize = "medium"
		if cfg.UI.CompactWidth < 440 || cfg.UI.CompactWidth > 540 {
			cfg.UI.CompactWidth = 480
		}
		if cfg.UI.CompactHeight < 95 || cfg.UI.CompactHeight > 150 {
			cfg.UI.CompactHeight = 106
		}
		cfg.UI.CompactExpandedHeight = cfg.UI.CompactHeight + 94
	}
	if cfg.UI.ManagerWidth < 1200 {
		cfg.UI.ManagerWidth = 1250
	}
	if cfg.UI.ManagerHeight < 790 {
		cfg.UI.ManagerHeight = 825
	}
	if cfg.UI.LogRetentionDays < 0 {
		cfg.UI.LogRetentionDays = 7
	}
	for idx := range cfg.Providers {
		cfg.Providers[idx] = migrateLegacyDotNetResults(cfg.Providers[idx])
		cfg.Providers[idx] = migrateLegacyGoProvider(cfg.Providers[idx])
		cfg.Providers[idx] = migrateLegacyCargoProvider(cfg.Providers[idx])
		cfg.Providers[idx] = migrateLegacyJestResults(cfg.Providers[idx])
		if cfg.Providers[idx].Name == "" {
			cfg.Providers[idx].Name = cfg.Providers[idx].ID
		}
		if cfg.Providers[idx].Root == "" {
			cfg.Providers[idx].Root = "."
		}
		if cfg.Providers[idx].Layer == "" {
			cfg.Providers[idx].Layer = inferLayer(cfg.Providers[idx].Kind)
		}
	}
	return cfg
}

func Validate(cfg domain.ProjectConfig) error {
	if cfg.Version != 1 {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if !validVisibilityMode(cfg.UI.WidgetMode) {
		return fmt.Errorf("unsupported widget mode %q", cfg.UI.WidgetMode)
	}
	if !validVisibilityMode(cfg.UI.TopmostMode) {
		return fmt.Errorf("unsupported topmost mode %q", cfg.UI.TopmostMode)
	}
	seen := map[string]struct{}{}
	for _, provider := range cfg.Providers {
		if strings.TrimSpace(provider.ID) == "" {
			return errors.New("provider id is required")
		}
		if _, exists := seen[provider.ID]; exists {
			return fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		if provider.Kind == "" {
			return fmt.Errorf("provider %q kind is required", provider.ID)
		}
		if provider.Enabled && provider.Run.All.IsZero() {
			return fmt.Errorf("provider %q is enabled but has no run.all command", provider.ID)
		}
	}
	return nil
}

// Detect returns a starter config with first-class built-in providers for common
// runners. Users can add custom providers for anything else.
func validVisibilityMode(mode domain.VisibilityMode) bool {
	switch mode {
	case domain.VisibilityOff, domain.VisibilityAlways, domain.VisibilityWhenActive:
		return true
	default:
		return false
	}
}
func Detect(projectRoot string) domain.ProjectConfig {
	providers := make([]domain.Provider, 0, 9)
	if hasAny(projectRoot, "*.sln", "*.slnx", "*.csproj", "**/*.csproj") {
		providers = append(providers, dotNetProviders(projectRoot)...)
	}
	if hasAny(projectRoot, "playwright.config.*", "**/playwright.config.*") || packageMentions(projectRoot, "@playwright/test", "playwright") {
		providers = append(providers, PlaywrightProvider())
	}
	if hasAny(projectRoot, "cypress.config.*", "**/cypress.config.*") || packageMentions(projectRoot, "cypress") {
		providers = append(providers, CypressProvider())
	}
	if hasAny(projectRoot, "vitest.config.*", "vite.config.*") || packageMentions(projectRoot, "vitest") {
		providers = append(providers, VitestProvider())
	}
	if packageMentions(projectRoot, "jest", "@jest/globals") {
		providers = append(providers, JestProvider())
	}
	if hasAny(projectRoot, "go.mod") {
		providers = append(providers, GoTestProvider())
	}
	if hasAny(projectRoot, "pytest.ini", "pyproject.toml", "setup.cfg") {
		providers = append(providers, PytestProvider())
	}
	if hasAny(projectRoot, "Cargo.toml") {
		providers = append(providers, CargoProvider())
	}
	if hasAny(projectRoot, "pom.xml") {
		providers = append(providers, MavenProvider())
	}
	if hasAny(projectRoot, "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts") {
		providers = append(providers, GradleProvider())
	}
	sort.SliceStable(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	return Normalise(projectRoot, domain.ProjectConfig{
		Version:   1,
		Name:      filepath.Base(projectRoot),
		Workspace: ".testdesk",
		UI: domain.UIConfig{
			Theme:                 "obsidian",
			WidgetMode:            domain.VisibilityWhenActive,
			AlwaysOnTop:           true,
			TopmostMode:           domain.VisibilityAlways,
			StartMinimized:        false,
			CompactWidth:          700,
			CompactHeight:         190,
			CompactExpandedHeight: 290,
			ManagerWidth:          1250,
			ManagerHeight:         825,
			LogRetentionDays:      7,
		},
		Providers: providers,
	})
}

func DotNetProvider() domain.Provider {
	return domain.Provider{
		ID:      "dotnet",
		Name:    ".NET tests",
		Kind:    domain.ProviderDotNet,
		Layer:   "backend",
		Root:    ".",
		Enabled: true,
		Tags:    []string{"backend", "unit", "integration", "xunit", "nunit", "mstest", "tunit"},
		Discover: domain.DiscoverSpec{Parser: "dotnet-list", Command: domain.CommandSpec{
			Exe: "dotnet", Args: []string{"test", "{{provider.root}}", "--list-tests", "--nologo"}, TimeoutSeconds: 180,
		}},
		Run: domain.RunSpec{
			All:  domain.CommandSpec{Exe: "dotnet", Args: []string{"test", "{{provider.root}}", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}, TimeoutSeconds: 1800},
			Test: domain.CommandSpec{Exe: "dotnet", Args: []string{"test", "{{provider.root}}", "--filter", "FullyQualifiedName={{test.selector}}", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}, TimeoutSeconds: 900},
		},
		Results: domain.ResultSpec{Format: "trx-dir", Path: "{{artifact.dir}}"},
	}
}

func dotNetProviders(projectRoot string) []domain.Provider {
	projects := dotNetTestProjects(projectRoot)
	if len(projects) == 0 {
		return []domain.Provider{DotNetProvider()}
	}
	providers := make([]domain.Provider, 0, len(projects))
	counts := make(map[string]int, len(projects))
	directories := make(map[string]int, len(projects))
	for _, project := range projects {
		counts[dotNetProviderForProject(project).ID]++
		directories[filepath.Dir(project)]++
	}
	for _, project := range projects {
		provider := dotNetProviderForProject(project)
		if counts[provider.ID] > 1 {
			digest := sha256.Sum256([]byte(project))
			provider.ID = fmt.Sprintf("%s--%x", provider.ID, digest[:8])
		}
		if directories[filepath.Dir(project)] > 1 {
			name := filepath.Base(project)
			provider.Discover.Command.Args[1] = name
			provider.Run.All.Args[1] = name
			provider.Run.Test.Args[1] = name
		}
		providers = append(providers, provider)
	}
	return providers
}

func dotNetTestProjects(projectRoot string) []string {
	projects := make([]string, 0)
	_ = filepath.WalkDir(projectRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			name := strings.ToLower(entry.Name())
			if name == "bin" || name == "obj" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".csproj") || !isDotNetTestProject(projectRoot, path) {
			return nil
		}
		relative, relativeErr := filepath.Rel(projectRoot, path)
		if relativeErr == nil {
			projects = append(projects, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(projects)
	return projects
}

func isDotNetTestProject(projectRoot string, projectPath string) bool {
	relative, err := filepath.Rel(projectRoot, projectPath)
	if err == nil && strings.HasPrefix(filepath.ToSlash(relative), "tests/") {
		return true
	}
	if strings.Contains(strings.ToLower(filepath.Base(projectPath)), "test") {
		return true
	}
	data, err := os.ReadFile(projectPath)
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "<istestproject>true</istestproject>") || strings.Contains(content, "microsoft.net.test.sdk")
}

func dotNetProviderForProject(project string) domain.Provider {
	provider := DotNetProvider()
	base := strings.TrimSuffix(filepath.Base(project), filepath.Ext(project))
	provider.ID = "dotnet-" + providerSlug(base)
	provider.Name = ".NET: " + base
	provider.Root = filepath.ToSlash(filepath.Dir(project))
	provider.Discover.Command.Args = []string{"test", ".", "--list-tests", "--nologo"}
	provider.Run.All.Args = []string{"test", ".", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}
	provider.Run.Test.Args = []string{"test", ".", "--filter", "FullyQualifiedName={{test.selector}}", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}
	return provider
}

// Migrate only the old generated logger contract. User-defined report paths
// and commands with their own results directory retain their original meaning.
func migrateLegacyDotNetResults(provider domain.Provider) domain.Provider {
	if provider.Kind != domain.ProviderDotNet || provider.Results.Format != "trx" || provider.Results.Path != "{{artifact.path}}" {
		return provider
	}
	convert := func(spec domain.CommandSpec) (domain.CommandSpec, bool) {
		if spec.IsZero() {
			return spec, true
		}
		if !strings.EqualFold(filepath.Base(spec.Exe), "dotnet") && !strings.EqualFold(filepath.Base(spec.Exe), "dotnet.exe") {
			return spec, false
		}
		args := append([]string(nil), spec.Args...)
		index := -1
		for i, arg := range args {
			if strings.EqualFold(arg, "--results-directory") || strings.HasPrefix(strings.ToLower(arg), "--results-directory=") {
				return spec, false
			}
			if arg == "trx;LogFileName={{artifact.name}}" {
				index = i
			}
		}
		if index < 0 {
			return spec, false
		}
		args[index] = "trx"
		spec.Args = append(args, "--results-directory", "{{artifact.dir}}")
		return spec, true
	}
	all, allOK := convert(provider.Run.All)
	test, testOK := convert(provider.Run.Test)
	if !allOK || !testOK || (provider.Run.All.IsZero() && provider.Run.Test.IsZero()) {
		return provider
	}
	provider.Run.All, provider.Run.Test = all, test
	provider.Results = domain.ResultSpec{Format: "trx-dir", Path: "{{artifact.dir}}"}
	return provider
}

func providerSlug(value string) string {
	var builder strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			separator = false
			continue
		}
		if !separator && builder.Len() > 0 {
			builder.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func PlaywrightProvider() domain.Provider {
	return domain.Provider{
		ID:      "playwright",
		Name:    "Playwright",
		Kind:    domain.ProviderPlaywright,
		Layer:   "frontend",
		Root:    ".",
		Enabled: true,
		Tags:    []string{"frontend", "e2e", "browser"},
		Discover: domain.DiscoverSpec{Parser: "playwright-list", Command: domain.CommandSpec{
			Exe: "npx", Args: []string{"playwright", "test", "--list"}, TimeoutSeconds: 120,
		}},
		Run: domain.RunSpec{
			All:  domain.CommandSpec{Exe: "npx", Args: []string{"playwright", "test", "--reporter=line,junit"}, Env: map[string]string{"PLAYWRIGHT_JUNIT_OUTPUT_NAME": "{{artifact.path}}"}, TimeoutSeconds: 1800},
			Test: domain.CommandSpec{Exe: "npx", Args: []string{"playwright", "test", "{{test.selector}}", "--reporter=line,junit"}, Env: map[string]string{"PLAYWRIGHT_JUNIT_OUTPUT_NAME": "{{artifact.path}}"}, TimeoutSeconds: 900},
		},
		Results: domain.ResultSpec{Format: "junit", Path: "{{artifact.path}}"},
	}
}

func CypressProvider() domain.Provider {
	return domain.Provider{
		ID:       "cypress",
		Name:     "Cypress",
		Kind:     domain.ProviderCypress,
		Layer:    "frontend",
		Root:     ".",
		Enabled:  true,
		Tags:     []string{"frontend", "e2e", "browser"},
		Discover: domain.DiscoverSpec{Parser: "none", Command: domain.CommandSpec{}},
		Run: domain.RunSpec{All: domain.CommandSpec{
			Exe: "npx", Args: []string{"cypress", "run", "--reporter", "junit", "--reporter-options", "mochaFile={{artifact.path}}"}, TimeoutSeconds: 1800,
		}},
		Results: domain.ResultSpec{Format: "junit", Path: "{{artifact.path}}"},
	}
}
func VitestProvider() domain.Provider {
	return domain.Provider{
		ID:      "vitest",
		Name:    "Vitest",
		Kind:    domain.ProviderVitest,
		Layer:   "frontend",
		Root:    ".",
		Enabled: true,
		Tags:    []string{"frontend", "unit", "component"},
		Discover: domain.DiscoverSpec{Parser: "vitest-json", Command: domain.CommandSpec{
			Exe: "npx", Args: []string{"vitest", "list", "--json"}, TimeoutSeconds: 120,
		}},
		Run: domain.RunSpec{
			All:  domain.CommandSpec{Exe: "npx", Args: []string{"vitest", "run", "--reporter=default", "--reporter=junit", "--outputFile={{artifact.path}}"}, TimeoutSeconds: 1800},
			Test: domain.CommandSpec{Exe: "npx", Args: []string{"vitest", "run", "-t", "{{test.selector}}", "--reporter=default", "--reporter=junit", "--outputFile={{artifact.path}}"}, TimeoutSeconds: 900},
		},
		Results: domain.ResultSpec{Format: "junit", Path: "{{artifact.path}}"},
	}
}

func JestProvider() domain.Provider {
	return domain.Provider{ID: "jest", Name: "Jest", Kind: domain.ProviderJest, Layer: "frontend", Root: ".", Enabled: true, Tags: []string{"frontend", "unit"}, Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "npx", Args: []string{"jest", "--listTests"}, TimeoutSeconds: 120}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "npx", Args: []string{"jest", "--ci", "--reporters=default", "--reporters=jest-junit"}, Env: map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}", "JEST_JUNIT_ADD_FILE_ATTRIBUTE": "true"}, TimeoutSeconds: 1800}, Test: domain.CommandSpec{Exe: "npx", Args: []string{"jest", "--runTestsByPath", "{{test.selector}}", "--ci", "--reporters=default", "--reporters=jest-junit"}, Env: map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}", "JEST_JUNIT_ADD_FILE_ATTRIBUTE": "true"}, TimeoutSeconds: 900}}, Results: domain.ResultSpec{Format: "junit", Path: "{{artifact.path}}"}}
}

// Migrate only the generated commands and report contract, leaving custom providers alone.
func migrateLegacyJestResults(provider domain.Provider) domain.Provider {
	if provider.Kind != domain.ProviderJest {
		return provider
	}
	generated := JestProvider()
	legacyAll, legacyTest := generated.Run.All, generated.Run.Test
	legacyTest.Args = []string{"jest", "{{test.selector}}", "--ci", "--reporters=default", "--reporters=jest-junit"}
	legacyAll.Env = map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}"}
	legacyTest.Env = map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}"}
	if provider.Results == generated.Results && reflect.DeepEqual(provider.Run.All, legacyAll) && reflect.DeepEqual(provider.Run.Test, legacyTest) {
		provider.Run = generated.Run
		return provider
	}
	legacyAll.Env, legacyTest.Env = nil, nil
	if provider.Results == (domain.ResultSpec{Format: "junit", Path: "junit.xml"}) && reflect.DeepEqual(provider.Run.All, legacyAll) && reflect.DeepEqual(provider.Run.Test, legacyTest) {
		provider.Run = generated.Run
		provider.Results = generated.Results
	}
	return provider
}

func GoTestProvider() domain.Provider {
	return domain.Provider{ID: "go-test", Name: "Go test", Kind: domain.ProviderGo, Layer: "backend", Root: ".", Enabled: true, Tags: []string{"backend", "unit"}, Discover: domain.DiscoverSpec{Parser: "go-json-list", Command: domain.CommandSpec{Exe: "go", Args: []string{"test", "-json", "./...", "-list", "."}, TimeoutSeconds: 120}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "go", Args: []string{"test", "./...", "-json"}, TimeoutSeconds: 1800}, Test: domain.CommandSpec{Exe: "go", Args: []string{"test", "{{test.goPackage}}", "-run", "^{{test.selector}}$", "-json"}, TimeoutSeconds: 900}}, Results: domain.ResultSpec{Format: "go-json", Path: ""}}
}

// Migrate only commands generated by older TestDesk versions; preserve custom adapters.
func migrateLegacyGoProvider(provider domain.Provider) domain.Provider {
	if provider.Kind != domain.ProviderGo {
		return provider
	}
	isGo := func(exe string) bool { return strings.TrimSuffix(strings.ToLower(filepath.Base(exe)), ".exe") == "go" }
	if provider.Discover.Parser == "go-list" && isGo(provider.Discover.Command.Exe) && reflect.DeepEqual(provider.Discover.Command.Args, []string{"test", "./...", "-list", "."}) {
		provider.Discover.Parser = "go-json-list"
		provider.Discover.Command.Args = []string{"test", "-json", "./...", "-list", "."}
	}
	if isGo(provider.Run.Test.Exe) && reflect.DeepEqual(provider.Run.Test.Args, []string{"test", "./...", "-run", "^{{test.selector}}$", "-json"}) {
		provider.Run.Test.Args = []string{"test", "{{test.goPackage}}", "-run", "^{{test.selector}}$", "-json"}
	}
	return provider
}

func PytestProvider() domain.Provider {
	return domain.Provider{ID: "pytest", Name: "pytest", Kind: domain.ProviderPytest, Layer: "backend", Root: ".", Enabled: true, Tags: []string{"backend", "unit", "integration"}, Discover: domain.DiscoverSpec{Parser: "pytest-collect", Command: domain.CommandSpec{Exe: "pytest", Args: []string{"--collect-only", "-q"}, TimeoutSeconds: 120}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "pytest", Args: []string{"--junitxml={{artifact.path}}"}, TimeoutSeconds: 1800}, Test: domain.CommandSpec{Exe: "pytest", Args: []string{"{{test.selector}}", "--junitxml={{artifact.path}}"}, TimeoutSeconds: 900}}, Results: domain.ResultSpec{Format: "junit", Path: "{{artifact.path}}"}}
}

func CargoProvider() domain.Provider {
	return domain.Provider{ID: "cargo-test", Name: "Cargo test", Kind: domain.ProviderCargo, Layer: "backend", Root: ".", Enabled: true, Tags: []string{"backend", "unit"}, Discover: domain.DiscoverSpec{Parser: "cargo-artifacts", Command: domain.CommandSpec{Exe: "cargo", Args: []string{"test", "--no-run", "--message-format=json"}, TimeoutSeconds: 120}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "cargo", Args: []string{"test"}, TimeoutSeconds: 1800}, Test: domain.CommandSpec{Exe: "cargo", Args: []string{"test", "{{test.cargoTarget}}", "{{test.selector}}", "--", "--exact"}, TimeoutSeconds: 900}}, Results: domain.ResultSpec{Format: "cargo-text", Path: ""}}
}

func migrateLegacyCargoProvider(provider domain.Provider) domain.Provider {
	if provider.Kind != domain.ProviderCargo || provider.Discover.Parser != "cargo-list" || provider.Discover.Command.Exe != "cargo" || strings.Join(provider.Discover.Command.Args, "\x00") != "test\x00--\x00--list" || provider.Run.All.Exe != "cargo" || strings.Join(provider.Run.All.Args, "\x00") != "test" || provider.Run.Test.Exe != "cargo" || strings.Join(provider.Run.Test.Args, "\x00") != "test\x00{{test.selector}}" || provider.Results.Format != "none" || provider.Results.Path != "" {
		return provider
	}
	provider.Discover.Parser = "cargo-artifacts"
	provider.Discover.Command.Args = []string{"test", "--no-run", "--message-format=json"}
	provider.Run.Test.Args = []string{"test", "{{test.cargoTarget}}", "{{test.selector}}", "--", "--exact"}
	provider.Results.Format = "cargo-text"
	return provider
}

func MavenProvider() domain.Provider {
	return domain.Provider{ID: "maven", Name: "Maven tests", Kind: domain.ProviderMaven, Layer: "backend", Root: ".", Enabled: true, Tags: []string{"backend", "junit"}, Discover: domain.DiscoverSpec{Parser: "none", Command: domain.CommandSpec{}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "mvn", Args: []string{"test"}, TimeoutSeconds: 1800}}, Results: domain.ResultSpec{Format: "junit-dir", Path: "target/surefire-reports"}}
}

func GradleProvider() domain.Provider {
	gradleExe := "./gradlew"
	if runtime.GOOS == "windows" {
		gradleExe = "gradlew.bat"
	}
	return domain.Provider{ID: "gradle", Name: "Gradle tests", Kind: domain.ProviderGradle, Layer: "backend", Root: ".", Enabled: true, Tags: []string{"backend", "junit"}, Discover: domain.DiscoverSpec{Parser: "none", Command: domain.CommandSpec{}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: gradleExe, Args: []string{"test"}, TimeoutSeconds: 1800}}, Results: domain.ResultSpec{Format: "junit-dir", Path: "build/test-results/test"}}
}

func inferLayer(kind domain.ProviderKind) string {
	switch kind {
	case domain.ProviderPlaywright, domain.ProviderCypress, domain.ProviderVitest, domain.ProviderJest:
		return "frontend"
	default:
		return "backend"
	}
}

func hasAny(root string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(pattern, "**") {
			found := false
			_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || found {
					return nil
				}
				if d.IsDir() {
					name := d.Name()
					if (strings.HasPrefix(name, ".") && name != ".") || name == "node_modules" || name == "vendor" || name == "bin" || name == "obj" || name == "target" || name == "dist" || name == "build" {
						return filepath.SkipDir
					}
					return nil
				}
				matched, _ := filepath.Match(filepath.Base(pattern), filepath.Base(path))
				if matched {
					found = true
				}
				return nil
			})
			if found {
				return true
			}
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, pattern))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

func packageMentions(root string, needles ...string) bool {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	for _, needle := range needles {
		if strings.Contains(content, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

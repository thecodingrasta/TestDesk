package domain

import "time"

// Status describes the lifecycle state of a test, item, or run.
type Status string

const (
	StatusUnknown   Status = "unknown"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusPassed    Status = "passed"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

// VisibilityMode controls the compact controller window behaviour.
type VisibilityMode string

const (
	VisibilityOff        VisibilityMode = "off"
	VisibilityAlways     VisibilityMode = "always"
	VisibilityWhenActive VisibilityMode = "when_active"
)

// ProviderKind identifies the native runner family.
type ProviderKind string

const (
	ProviderDotNet     ProviderKind = "dotnet"
	ProviderPlaywright ProviderKind = "playwright"
	ProviderCypress    ProviderKind = "cypress"
	ProviderVitest     ProviderKind = "vitest"
	ProviderJest       ProviderKind = "jest"
	ProviderGo         ProviderKind = "go-test"
	ProviderPytest     ProviderKind = "pytest"
	ProviderCargo      ProviderKind = "cargo-test"
	ProviderMaven      ProviderKind = "maven"
	ProviderGradle     ProviderKind = "gradle"
	ProviderCustom     ProviderKind = "custom"
)

// CommandSpec is a native executable invocation. No shell is used unless the
// user explicitly configures the executable as a shell.
type CommandSpec struct {
	Exe            string            `json:"exe"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"workingDir,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

func (c CommandSpec) IsZero() bool {
	return c.Exe == "" && len(c.Args) == 0
}

// DiscoverSpec defines how a provider enumerates tests.
type DiscoverSpec struct {
	Command CommandSpec `json:"command"`
	Parser  string      `json:"parser"`
}

// RunSpec defines how a provider runs all tests or one selected test.
type RunSpec struct {
	All  CommandSpec `json:"all"`
	Test CommandSpec `json:"test,omitempty"`
}

// ResultSpec tells TestDesk where native runner reports appear and how to parse
// them. The path supports the same templates as CommandSpec.
type ResultSpec struct {
	Format string `json:"format"`
	Path   string `json:"path"`
}

// Provider is the user-editable runner adapter. This gives first-class built-in
// adapters while still allowing any framework to be wired in without rebuilding
// TestDesk.
type Provider struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Kind     ProviderKind `json:"kind"`
	Layer    string       `json:"layer,omitempty"`
	Root     string       `json:"root"`
	Enabled  bool         `json:"enabled"`
	Tags     []string     `json:"tags,omitempty"`
	Discover DiscoverSpec `json:"discover"`
	Run      RunSpec      `json:"run"`
	Results  ResultSpec   `json:"results,omitempty"`
}

// UIConfig is persisted user configuration for the native desktop experience.
type UIConfig struct {
	Theme                 string         `json:"theme"`
	WidgetMode            VisibilityMode `json:"widgetMode"`
	WidgetSize            string         `json:"widgetSize,omitempty"`
	AlwaysOnTop           bool           `json:"alwaysOnTop"`
	TopmostMode           VisibilityMode `json:"topmostMode,omitempty"`
	StartMinimized        bool           `json:"startMinimized"`
	CompactWidth          int            `json:"compactWidth"`
	CompactHeight         int            `json:"compactHeight"`
	CompactExpandedHeight int            `json:"compactExpandedHeight"`
	CompactX              int            `json:"compactX,omitempty"`
	CompactY              int            `json:"compactY,omitempty"`
	CompactPositionSet    bool           `json:"compactPositionSet,omitempty"`
	ManagerWidth          int            `json:"managerWidth"`
	ManagerHeight         int            `json:"managerHeight"`
	LogRetentionDays      int            `json:"logRetentionDays,omitempty"`
}

// ProjectConfig is stored in testdesk.json at the project root.
type ProjectConfig struct {
	Version   int        `json:"version"`
	Name      string     `json:"name"`
	Workspace string     `json:"workspace"`
	UI        UIConfig   `json:"ui"`
	Providers []Provider `json:"providers"`
}

// TestCase is a stable descriptor discovered from a provider.
type TestCase struct {
	ID                   string       `json:"id"`
	ProviderID           string       `json:"providerId"`
	ProviderName         string       `json:"providerName"`
	ProviderKind         ProviderKind `json:"providerKind"`
	Name                 string       `json:"name"`
	FullName             string       `json:"fullName"`
	Selector             string       `json:"selector"`
	GoPackage            string       `json:"goPackage,omitempty"`
	JestProject          string       `json:"jestProject,omitempty"`
	VitestProject        string       `json:"vitestProject,omitempty"`
	CargoPackage         string       `json:"cargoPackage,omitempty"`
	CargoTargetKind      string       `json:"cargoTargetKind,omitempty"`
	CargoTargetName      string       `json:"cargoTargetName,omitempty"`
	DotNetFilterProperty string       `json:"dotNetFilterProperty,omitempty"`
	DotNetProject        string       `json:"dotNetProject,omitempty"`
	DotNetNUnitLegacy    bool         `json:"dotNetNUnitLegacy,omitempty"`
	File                 string       `json:"file,omitempty"`
	Line                 int          `json:"line,omitempty"`
	Layer                string       `json:"layer,omitempty"`
	Module               string       `json:"module,omitempty"`
	Tags                 []string     `json:"tags,omitempty"`
	LastStatus           Status       `json:"lastStatus"`
	LastDurationMillis   int64        `json:"lastDurationMillis,omitempty"`
	LastRunID            string       `json:"lastRunId,omitempty"`
	LastFailure          string       `json:"lastFailure,omitempty"`
	DiscoveredAt         time.Time    `json:"discoveredAt"`
}

// TestResult is parsed from runner output artifacts.
type TestResult struct {
	Name           string `json:"name"`
	FullName       string `json:"fullName"`
	Selector       string `json:"selector"`
	File           string `json:"file,omitempty"`
	Status         Status `json:"status"`
	DurationMillis int64  `json:"durationMillis,omitempty"`
	Message        string `json:"message,omitempty"`
	Output         string `json:"output,omitempty"`
}

// RunItem is a single executable unit. It can represent an entire provider or a
// selected test depending on the command available.
type RunItem struct {
	ID              string       `json:"id"`
	RunID           string       `json:"runId"`
	ProviderID      string       `json:"providerId"`
	ProviderName    string       `json:"providerName"`
	ProviderKind    ProviderKind `json:"providerKind"`
	TestID          string       `json:"testId,omitempty"`
	TestName        string       `json:"testName"`
	CommandPreview  string       `json:"commandPreview"`
	Warnings        []string     `json:"warnings,omitempty"`
	Status          Status       `json:"status"`
	StartedAt       *time.Time   `json:"startedAt,omitempty"`
	FinishedAt      *time.Time   `json:"finishedAt,omitempty"`
	DurationMillis  int64        `json:"durationMillis,omitempty"`
	EstimatedMillis int64        `json:"estimatedMillis,omitempty"`
	Error           string       `json:"error,omitempty"`
}

// Run represents a lifecycle-managed test run.
type Run struct {
	ID              string    `json:"id"`
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
	Items           []RunItem `json:"items"`
	CurrentIndex    int       `json:"currentIndex"`
	Passed          int       `json:"passed"`
	Failed          int       `json:"failed"`
	Skipped         int       `json:"skipped"`
	Cancelled       int       `json:"cancelled"`
	Total           int       `json:"total"`
	LogPath         string    `json:"logPath,omitempty"`
	DiagnosticsPath string    `json:"diagnosticsPath,omitempty"`
}

// Filter is intentionally small and serialisable so UI and MCP clients can use
// the same language without wasting tokens.
type Filter struct {
	ProviderIDs   []string `json:"providerIds,omitempty"`
	ProviderKinds []string `json:"providerKinds,omitempty"`
	Layers        []string `json:"layers,omitempty"`
	Modules       []string `json:"modules,omitempty"`
	Statuses      []Status `json:"statuses,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Query         string   `json:"query,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Offset        int      `json:"offset,omitempty"`
}

// Snapshot is the compact state returned to UI and MCP clients.
type Snapshot struct {
	ProjectName       string     `json:"projectName"`
	ProviderCount     int        `json:"providerCount"`
	TestCount         int        `json:"testCount"`
	QueuedCount       int        `json:"queuedCount"`
	Paused            bool       `json:"paused"`
	Running           *Run       `json:"running,omitempty"`
	RecentRuns        []Run      `json:"recentRuns"`
	Tests             []TestCase `json:"tests,omitempty"`
	ActiveProviderIDs []string   `json:"activeProviderIds"`
	GeneratedAt       time.Time  `json:"generatedAt"`
}

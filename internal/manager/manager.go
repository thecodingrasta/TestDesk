package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/discovery"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/filters"
	"github.com/thecodingrasta/testdesk/internal/results"
	"github.com/thecodingrasta/testdesk/internal/runner"
	"github.com/thecodingrasta/testdesk/internal/store"
)

type RunRequest struct {
	TestIDs         []string      `json:"testIds,omitempty"`
	ProviderIDs     []string      `json:"providerIds,omitempty"`
	ProviderWideIDs []string      `json:"-"`
	Filter          domain.Filter `json:"filter,omitempty"`
}

type ControlAction string

const (
	ControlStop   ControlAction = "stop"
	ControlKill   ControlAction = "kill"
	ControlSkip   ControlAction = "skip"
	ControlPause  ControlAction = "pause"
	ControlResume ControlAction = "resume"
)

type Diagnostics struct {
	Run  *domain.Run `json:"run,omitempty"`
	Log  string      `json:"log"`
	Path string      `json:"path"`
}

type queuedRun struct {
	run       domain.Run
	ctx       context.Context
	cancelled bool
	lease     *store.RunLease
}

type Manager struct {
	projectRoot   string
	cfg           domain.ProjectConfig
	store         *store.Store
	runner        runner.Runner
	discoveryGate chan struct{}

	mu              sync.Mutex
	tests           map[string]domain.TestCase
	runQueue        []queuedRun
	current         *domain.Run
	itemCancel      context.CancelFunc
	processing      bool
	paused          bool
	skipRequested   bool
	stopRequested   bool
	discoveryStatus DiscoveryStatus
	catalogLoadedAt time.Time
	streamedTests   map[string]domain.Status
	itemPassedBase  int
	itemFailedBase  int
	itemSkippedBase int
}

func New(projectRoot string, cfg domain.ProjectConfig, proc runner.Runner) (*Manager, error) {
	if proc == nil {
		proc = runner.NewProcessRunner()
	}
	cfg = config.Normalise(projectRoot, cfg)
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	st := store.New(projectRoot, cfg.Workspace)
	if err := st.Ensure(); err != nil {
		return nil, err
	}
	if err := st.ReconcileOrphanRuns(); err != nil {
		return nil, err
	}
	catalog, modTime, err := st.LoadCatalogWithModTime()
	if err != nil {
		return nil, err
	}
	tests := make(map[string]domain.TestCase, len(catalog))
	for _, test := range catalog {
		tests[test.ID] = test
	}
	m := &Manager{projectRoot: projectRoot, cfg: cfg, store: st, runner: proc, discoveryGate: make(chan struct{}, 1), tests: tests, catalogLoadedAt: modTime}
	if cfg.UI.LogRetentionDays > 0 {
		_, _ = st.Prune(cfg.UI.LogRetentionDays)
	}
	return m, nil
}

func NewFromProject(projectRoot string) (*Manager, error) {
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, err
	}
	return New(projectRoot, cfg, nil)
}

func (m *Manager) Config() domain.ProjectConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// IsActive reports queued or executing work without reading the catalog or history.
func (m *Manager) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processing || m.current != nil || len(m.runQueue) > 0
}

func (m *Manager) Store() *store.Store { return m.store }
func (m *Manager) StoreRoot() string   { return m.store.Root() }

// ProjectRoot identifies the configured project served by this lifecycle manager.
func (m *Manager) ProjectRoot() string { return m.projectRoot }

func (m *Manager) UpdateConfig(cfg domain.ProjectConfig) error {
	cfg = config.Normalise(m.projectRoot, cfg)
	if err := config.Validate(cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	if cfg.UI.LogRetentionDays > 0 {
		_, _ = m.store.Prune(cfg.UI.LogRetentionDays)
	}
	return nil
}

type DiscoverProgress struct {
	Provider        domain.Provider `json:"provider"`
	Current         int             `json:"current"`
	Total           int             `json:"total"`
	DiscoveredTotal int             `json:"discoveredTotal"`
	ProviderTests   int             `json:"providerTests"`
	Phase           string          `json:"phase"`
	Detail          string          `json:"detail,omitempty"`
	LatestTest      string          `json:"latestTest,omitempty"`
}

type DiscoverProgressFunc func(progress DiscoverProgress)

func boundedDiscoveryText(value string) string {
	if len(value) <= 256 {
		return value
	}
	return strings.ToValidUTF8(value[:256], "")
}

// DiscoveryStatus describes observed discovery activity without retaining output.
type DiscoveryStatus struct {
	Active            bool       `json:"active"`
	Outcome           string     `json:"outcome"`
	ProviderID        string     `json:"providerId,omitempty"`
	ProviderName      string     `json:"providerName,omitempty"`
	Phase             string     `json:"phase,omitempty"`
	Current           int        `json:"current"`
	Total             int        `json:"total"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	ProviderStartedAt *time.Time `json:"providerStartedAt,omitempty"`
	LastOutputAt      *time.Time `json:"lastOutputAt,omitempty"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
	ElapsedMillis     int64      `json:"elapsedMillis"`
	QuietMillis       *int64     `json:"quietMillis,omitempty"`
}

func (m *Manager) DiscoveryStatus() DiscoveryStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.discoveryStatus
	if status.Outcome == "" {
		status.Outcome = "idle"
	}
	now := time.Now().UTC()
	end := now
	if status.FinishedAt != nil {
		end = *status.FinishedAt
	}
	if status.StartedAt != nil {
		status.ElapsedMillis = end.Sub(*status.StartedAt).Milliseconds()
	}
	if status.LastOutputAt != nil {
		quiet := end.Sub(*status.LastOutputAt).Milliseconds()
		status.QuietMillis = &quiet
	}
	return status
}

func (m *Manager) Discover(ctx context.Context) ([]domain.TestCase, error) {
	return m.DiscoverWithProgress(ctx, nil)
}

func (m *Manager) DiscoverWithProgress(ctx context.Context, onProgress DiscoverProgressFunc) (found []domain.TestCase, resultErr error) {
	select {
	case m.discoveryGate <- struct{}{}:
		defer func() { <-m.discoveryGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	m.mu.Lock()
	m.discoveryStatus = DiscoveryStatus{Active: true, Outcome: "running", Phase: "starting", StartedAt: &started}
	m.mu.Unlock()
	defer func() {
		finished := time.Now().UTC()
		m.mu.Lock()
		m.discoveryStatus.Active = false
		m.discoveryStatus.FinishedAt = &finished
		switch {
		case ctx.Err() != nil:
			m.discoveryStatus.Outcome = "cancelled"
		case resultErr != nil:
			m.discoveryStatus.Outcome = "failed"
		default:
			m.discoveryStatus.Outcome = "completed"
		}
		m.mu.Unlock()
	}()
	callback := onProgress
	onProgress = func(progress DiscoverProgress) {
		m.mu.Lock()
		status := &m.discoveryStatus
		status.Phase = progress.Phase
		status.Total = progress.Total
		if progress.Current > 0 {
			status.Current = progress.Current
		}
		if progress.Provider.ID != "" {
			if progress.Phase == "starting" {
				now := time.Now().UTC()
				status.ProviderStartedAt = &now
				status.LastOutputAt = nil
			}
			status.ProviderID = boundedDiscoveryText(progress.Provider.ID)
			status.ProviderName = boundedDiscoveryText(progress.Provider.Name)
		}
		m.mu.Unlock()
		if callback != nil {
			callback(progress)
		}
	}
	providers := m.enabledProviders()
	all := make([]domain.TestCase, 0)
	var messages []string
	total := len(providers)

	if onProgress != nil {
		onProgress(DiscoverProgress{
			Current:         0,
			Total:           total,
			DiscoveredTotal: 0,
			Phase:           "starting",
			Detail:          "Preparing provider discovery…",
		})
	}

	old := m.catalogMap()
	preserveProvider := func(provider domain.Provider, cause error) {
		count := 0
		for _, test := range old {
			if test.ProviderID == provider.ID {
				all = append(all, test)
				count++
			}
		}
		messages = append(messages, fmt.Sprintf("%s: %v (retained %d previously discovered tests)", provider.ID, cause, count))
		if onProgress != nil {
			onProgress(DiscoverProgress{Provider: provider, Total: total, DiscoveredTotal: len(all), ProviderTests: count, Phase: "provider_failed", Detail: cause.Error()})
		}
	}
	for idx, provider := range providers {
		current := idx + 1
		if onProgress != nil {
			onProgress(DiscoverProgress{
				Provider:        provider,
				Current:         current,
				Total:           total,
				DiscoveredTotal: len(all),
				ProviderTests:   0,
				Phase:           "starting",
				Detail:          fmt.Sprintf("Scanning %d/%d (%s)…", current, total, provider.Name),
			})
		}
		if provider.Discover.Command.IsZero() || strings.EqualFold(provider.Discover.Parser, "none") {
			// A provider without discovery cannot refresh its previous catalog.
			for _, test := range old {
				if test.ProviderID == provider.ID {
					all = append(all, test)
				}
			}
			continue
		}
		cmd, err := runner.RenderCommand(provider.Discover.Command, runner.TemplateData{ProjectRoot: m.projectRoot, Provider: provider})
		if err != nil {
			preserveProvider(provider, err)
			continue
		}
		checkDotNetAdapters := isBuiltInDotNetDiscovery(provider)
		alternateCmd := cmd
		if checkDotNetAdapters {
			alternateCmd.Args = append(append([]string{}, cmd.Args...), "--no-build", "--no-restore", "--", "NUnit.DisplayName=FullNameSep", "NUnit.FullnameSeparator=:")
			cmd.Args = append(cmd.Args, "--", "NUnit.DisplayName=FullName")
		}

		providerLiveCount := 0
		var lastEmit time.Time
		var lastTestName string
		var goProgress discovery.GoJSONProgress

		result, err := m.runner.Run(ctx, cmd, func(event runner.Event) {
			if event.Type == runner.EventStdout || event.Type == runner.EventStderr {
				now := time.Now().UTC()
				m.mu.Lock()
				m.discoveryStatus.LastOutputAt = &now
				m.mu.Unlock()
			}
			if event.Type != runner.EventStdout {
				return
			}
			line := strings.TrimSpace(event.Line)
			if line == "" {
				return
			}
			lower := strings.ToLower(line)
			isBuilding := strings.Contains(lower, "restore") || strings.Contains(lower, "building") || strings.Contains(lower, "msbuild") || strings.Contains(lower, "determining")

			isTest := false
			if strings.EqualFold(strings.TrimSpace(provider.Discover.Parser), "go-json-list") {
				names := goProgress.Push(event.Line)
				if len(names) > 0 {
					providerLiveCount += len(names)
					lastTestName = names[len(names)-1]
					isTest = true
				}
			} else if identified, displayName := discovery.IdentifyStreamedTestLine(provider, event.Line); identified {
				providerLiveCount++
				lastTestName = displayName
				isTest = true
			}

			if onProgress == nil {
				return
			}

			now := time.Now()
			shouldEmit := false
			if isBuilding && providerLiveCount == 0 && now.Sub(lastEmit) > 200*time.Millisecond {
				shouldEmit = true
			} else if isTest {
				if providerLiveCount <= 5 || providerLiveCount%15 == 0 || now.Sub(lastEmit) > 50*time.Millisecond {
					shouldEmit = true
				}
			}

			if shouldEmit {
				lastEmit = now
				phase := "scanning"
				detail := fmt.Sprintf("Scanning %d/%d (%s) — %d test(s) found", current, total, provider.Name, len(all)+providerLiveCount)
				if isBuilding && providerLiveCount == 0 {
					phase = "building"
					detail = fmt.Sprintf("Building & resolving %d/%d (%s)…", current, total, provider.Name)
				}
				onProgress(DiscoverProgress{
					Provider:        provider,
					Current:         current,
					Total:           total,
					DiscoveredTotal: len(all) + providerLiveCount,
					ProviderTests:   providerLiveCount,
					Phase:           phase,
					Detail:          detail,
					LatestTest:      lastTestName,
				})
			}
		})

		if err != nil || result.ExitCode != 0 || result.TimedOut || ctx.Err() != nil {
			if err == nil {
				if ctx.Err() != nil {
					err = ctx.Err()
				} else {
					err = fmt.Errorf("discovery did not complete successfully (exit code %d, timed out: %t)", result.ExitCode, result.TimedOut)
				}
			}
			preserveProvider(provider, err)
			continue
		}
		var parsed []domain.TestCase
		var parseErr error
		if provider.Discover.Parser == "cargo-artifacts" {
			parsed, parseErr = m.discoverCargoArtifacts(ctx, provider, cmd, result.Stdout)
		} else {
			parsed, parseErr = discovery.Parse(provider, result.Stdout)
		}
		if parseErr != nil {
			preserveProvider(provider, parseErr)
			continue
		}
		if isBuiltInJestTarget(provider) && isBuiltInJestDiscovery(provider) {
			parsed, parseErr = m.discoverJestProjects(ctx, provider, cmd, parsed)
			if parseErr != nil {
				preserveProvider(provider, parseErr)
				continue
			}
		}
		if checkDotNetAdapters {
			onProgress(DiscoverProgress{Provider: provider, Current: current, Total: total, DiscoveredTotal: len(all) + len(parsed), ProviderTests: len(parsed), Phase: "scanning", Detail: "Checking .NET case identities…"})
			alternate, runErr := m.runner.Run(ctx, alternateCmd, func(event runner.Event) {
				if event.Type == runner.EventStdout || event.Type == runner.EventStderr {
					now := time.Now().UTC()
					m.mu.Lock()
					m.discoveryStatus.LastOutputAt = &now
					m.mu.Unlock()
				}
			})
			if runErr == nil && (alternate.ExitCode != 0 || alternate.TimedOut || ctx.Err() != nil) {
				if ctx.Err() != nil {
					runErr = ctx.Err()
				} else {
					runErr = fmt.Errorf("adapter identity discovery failed (exit code %d, timed out: %t)", alternate.ExitCode, alternate.TimedOut)
				}
			}
			if runErr == nil {
				var alternateTests []domain.TestCase
				alternateTests, runErr = discovery.Parse(provider, alternate.Stdout)
				if runErr == nil {
					runErr = discovery.IdentifyDotNetAdapters(parsed, alternateTests)
				}
			}
			if runErr != nil {
				preserveProvider(provider, runErr)
				continue
			}
			if dotNetMixedAdapters(parsed) {
				onProgress(DiscoverProgress{Provider: provider, Current: current, Total: total, DiscoveredTotal: len(all) + len(parsed), ProviderTests: len(parsed), Phase: "scanning", Detail: "Identifying .NET test projects…"})
				if err := m.identifyDotNetProjects(ctx, provider, cmd, parsed); err != nil {
					preserveProvider(provider, err)
					continue
				}
			}
		}
		for pIdx := range parsed {
			if previous, ok := old[parsed[pIdx].ID]; ok {
				parsed[pIdx].LastStatus = previous.LastStatus
				parsed[pIdx].LastDurationMillis = previous.LastDurationMillis
				parsed[pIdx].LastRunID = previous.LastRunID
				parsed[pIdx].LastFailure = previous.LastFailure
			}
		}
		all = append(all, parsed...)
		if onProgress != nil {
			onProgress(DiscoverProgress{
				Provider:        provider,
				Current:         current,
				Total:           total,
				DiscoveredTotal: len(all),
				ProviderTests:   len(parsed),
				Phase:           "provider_done",
				Detail:          fmt.Sprintf("Discovered %d test(s) in %s (%d/%d)", len(parsed), provider.Name, current, total),
				LatestTest:      lastTestName,
			})
		}
	}

	unique := make(map[string]domain.TestCase, len(all))
	for _, test := range all {
		unique[test.ID] = test
	}
	all = all[:0]
	for _, test := range unique {
		all = append(all, test)
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].ProviderID != all[j].ProviderID {
			return all[i].ProviderID < all[j].ProviderID
		}
		return all[i].FullName < all[j].FullName
	})

	m.mu.Lock()
	for i := range all {
		if latest, ok := m.tests[all[i].ID]; ok {
			all[i].LastStatus = latest.LastStatus
			all[i].LastDurationMillis = latest.LastDurationMillis
			all[i].LastRunID = latest.LastRunID
			all[i].LastFailure = latest.LastFailure
		}
	}
	all, err := m.store.ReplaceCatalog(all)
	if err == nil {
		m.setCatalogLocked(all)
	}
	m.mu.Unlock()
	if err != nil {
		return all, err
	}
	if len(messages) > 0 {
		return all, errors.New(strings.Join(messages, "; "))
	}
	return all, nil
}

func (m *Manager) Snapshot(filter domain.Filter) (domain.Snapshot, error) {
	m.mu.Lock()
	tests := m.testsSliceLocked()
	current := cloneRunPtr(m.current)
	queued := len(m.runQueue)
	paused := m.paused
	m.mu.Unlock()

	_, selected := filters.Apply(tests, filter)
	recent, err := m.store.RecentRuns(8)
	if err != nil {
		return domain.Snapshot{}, err
	}
	active := make([]string, 0)
	providerTestCounts := make(map[string]int)
	for _, test := range tests {
		providerTestCounts[test.ProviderID]++
	}
	for _, provider := range m.enabledProviders() {
		if len(tests) == 0 || providerTestCounts[provider.ID] > 0 {
			active = append(active, provider.ID)
		}
	}
	return domain.Snapshot{ProjectName: m.cfg.Name, ProviderCount: len(active), TestCount: len(tests), QueuedCount: queued, Paused: paused, Running: current, RecentRuns: recent, Tests: selected, ActiveProviderIDs: active, GeneratedAt: time.Now().UTC()}, nil
}

// Plan resolves a run request into executable items without enqueueing anything.
// It is primarily used by MCP clients for safe dry-runs and pre-flight checks.
func (m *Manager) Plan(request RunRequest) ([]domain.RunItem, error) {
	items, err := m.buildRunItems(request)
	if err != nil {
		return nil, err
	}
	copyItems := make([]domain.RunItem, len(items))
	copy(copyItems, items)
	return copyItems, nil
}

func (m *Manager) Run(ctx context.Context, request RunRequest) (domain.Run, error) {
	items, err := m.buildRunItems(request)
	if err != nil {
		return domain.Run{}, err
	}
	if len(items) == 0 {
		return domain.Run{}, errors.New("no tests or providers matched the run request")
	}
	runID := newRunID()
	now := time.Now().UTC()
	for idx := range items {
		items[idx].RunID = runID
		items[idx].ID = fmt.Sprintf("%03d", idx+1)
	}
	run := domain.Run{ID: runID, Status: domain.StatusQueued, CreatedAt: now, Items: items, CurrentIndex: -1, Total: len(items), LogPath: m.store.LogPath(runID), DiagnosticsPath: m.store.DiagnosticsPath(runID)}
	response := cloneRun(run)

	lease, err := m.store.BeginRun(runID)
	if err != nil {
		return domain.Run{}, err
	}
	if err := m.store.SaveRun(run); err != nil {
		_ = lease.Close()
		return domain.Run{}, err
	}
	m.mu.Lock()
	m.runQueue = append(m.runQueue, queuedRun{run: run, ctx: ctx, lease: lease})
	shouldStart := !m.processing
	if shouldStart {
		m.processing = true
		m.stopRequested = false
	}
	m.mu.Unlock()

	if shouldStart {
		go m.processQueue()
	}
	return response, nil
}

func (m *Manager) Control(action ControlAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch action {
	case ControlPause:
		m.paused = true
		return nil
	case ControlResume:
		m.paused = false
		return nil
	case ControlSkip:
		if m.current == nil || m.itemCancel == nil {
			return errors.New("no active item to skip")
		}
		m.skipRequested = true
		m.itemCancel()
		return nil
	case ControlStop, ControlKill:
		if m.current != nil {
			m.stopRequested = true
		}
		for i := range m.runQueue {
			m.runQueue[i].cancelled = true
		}
		if m.itemCancel != nil {
			m.itemCancel()
		}
		return nil
	default:
		return fmt.Errorf("unknown control action %q", action)
	}
}

func (m *Manager) Diagnostics(runID string, maxBytes int64) (Diagnostics, error) {
	if strings.TrimSpace(runID) == "" {
		m.mu.Lock()
		if m.current != nil {
			runID = m.current.ID
		}
		m.mu.Unlock()
	}
	if runID == "" {
		return Diagnostics{}, errors.New("run id is required")
	}
	if err := store.ValidateRunID(runID); err != nil {
		return Diagnostics{}, err
	}
	path := m.store.LogPath(runID)
	logTail, err := m.store.Tail(path, maxBytes)
	if err != nil {
		return Diagnostics{}, err
	}
	var run *domain.Run
	data, err := os.ReadFile(m.store.DiagnosticsPath(runID))
	if err == nil {
		var decoded domain.Run
		if json.Unmarshal(data, &decoded) == nil {
			run = &decoded
		}
	}
	return Diagnostics{Run: run, Log: logTail, Path: path}, nil
}

func (m *Manager) processQueue() {
	for {
		m.mu.Lock()
		if len(m.runQueue) == 0 {
			m.processing = false
			m.mu.Unlock()
			return
		}
		next := m.runQueue[0]
		m.runQueue = m.runQueue[1:]
		m.current = &next.run
		m.current.Status = domain.StatusRunning
		m.current.StartedAt = time.Now().UTC()
		m.stopRequested = next.cancelled
		m.skipRequested = false
		m.streamedTests = make(map[string]domain.Status)
		m.mu.Unlock()

		m.executeRun(next.ctx)

		m.mu.Lock()
		completed := cloneRun(*m.current)
		m.current = nil
		m.itemCancel = nil
		m.stopRequested = false
		m.skipRequested = false
		m.streamedTests = nil
		m.mu.Unlock()
		_ = m.store.SaveRun(completed)
		_ = m.store.AppendHistory(completed)
		_ = next.lease.Close()
		if m.cfg.UI.LogRetentionDays > 0 {
			_, _ = m.store.Prune(m.cfg.UI.LogRetentionDays)
		}
	}
}

func (m *Manager) executeRun(parent context.Context) {
	m.mu.Lock()
	run := m.current
	m.mu.Unlock()
	if run == nil {
		return
	}

	for idx := range run.Items {
		m.waitIfPaused(parent)
		if parent.Err() != nil {
			m.finishRemaining(idx, domain.StatusCancelled, "parent context cancelled")
			break
		}
		if m.isStopRequested() {
			m.finishRemaining(idx, domain.StatusCancelled, "run stopped")
			break
		}

		ctx, cancel := context.WithCancel(parent)
		m.mu.Lock()
		// Stop can arrive between the check above and dispatch. Do not mark or
		// start an item after a stop request has already been recorded.
		if m.stopRequested {
			m.mu.Unlock()
			cancel()
			m.finishRemaining(idx, domain.StatusCancelled, "run stopped")
			break
		}
		m.current.CurrentIndex = idx
		m.current.Items[idx].Status = domain.StatusRunning
		started := time.Now().UTC()
		m.current.Items[idx].StartedAt = &started
		item := m.current.Items[idx]
		m.itemPassedBase, m.itemFailedBase, m.itemSkippedBase = m.current.Passed, m.current.Failed, m.current.Skipped
		m.streamedTests = make(map[string]domain.Status)
		m.itemCancel = cancel
		m.skipRequested = false
		m.mu.Unlock()

		status, resultMessage := m.executeItem(ctx, item)
		cancel()

		finished := time.Now().UTC()
		m.mu.Lock()
		if m.skipRequested {
			status = domain.StatusSkipped
			resultMessage = "skipped by user"
			if item.TestID != "" {
				if test, ok := m.tests[item.TestID]; ok {
					test.LastStatus = domain.StatusSkipped
					test.LastFailure = "skipped by user"
					if m.current.Items[idx].StartedAt != nil {
						test.LastDurationMillis = finished.Sub(*m.current.Items[idx].StartedAt).Milliseconds()
					}
					test.LastRunID = item.RunID
					m.tests[item.TestID] = test
					_ = m.updateCatalogResultsLocked([]domain.TestCase{test})
				}
			}
		} else if parent.Err() != nil {
			status = domain.StatusCancelled
			resultMessage = "caller context cancelled"
		} else if m.stopRequested && status != domain.StatusPassed {
			status = domain.StatusCancelled
			resultMessage = "cancelled by user"
		}
		m.current.Items[idx].Status = status
		m.current.Items[idx].FinishedAt = &finished
		if m.current.Items[idx].StartedAt != nil {
			m.current.Items[idx].DurationMillis = finished.Sub(*m.current.Items[idx].StartedAt).Milliseconds()
		}
		m.current.Items[idx].Error = resultMessage
		m.itemCancel = nil
		itemPassed, itemFailed, itemSkipped, itemCancelled := countItems(m.current.Items)
		if itemPassed > m.current.Passed {
			m.current.Passed = itemPassed
		}
		if itemFailed > m.current.Failed {
			m.current.Failed = itemFailed
		}
		if itemSkipped > m.current.Skipped {
			m.current.Skipped = itemSkipped
		}
		m.current.Cancelled = itemCancelled
		_ = m.store.SaveRun(*m.current)
		stop := m.stopRequested
		m.mu.Unlock()

		if stop {
			m.finishRemaining(idx+1, domain.StatusCancelled, "run stopped")
			break
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	m.current.FinishedAt = time.Now().UTC()
	itemPassed, itemFailed, itemSkipped, itemCancelled := countItems(m.current.Items)
	if itemPassed > m.current.Passed {
		m.current.Passed = itemPassed
	}
	if itemFailed > m.current.Failed {
		m.current.Failed = itemFailed
	}
	if itemSkipped > m.current.Skipped {
		m.current.Skipped = itemSkipped
	}
	m.current.Cancelled = itemCancelled
	switch {
	case m.current.Failed > 0:
		m.current.Status = domain.StatusFailed
	case m.current.Cancelled > 0:
		m.current.Status = domain.StatusCancelled
	case m.current.Skipped > 0 && m.current.Passed == 0:
		m.current.Status = domain.StatusSkipped
	default:
		m.current.Status = domain.StatusPassed
	}
}

func (m *Manager) executeItem(ctx context.Context, item domain.RunItem) (domain.Status, string) {
	provider, ok := m.providerByID(item.ProviderID)
	if !ok {
		return domain.StatusFailed, "provider not found"
	}
	test := m.findTest(item.TestID)
	spec := provider.Run.All
	if item.TestID != "" {
		if test == nil {
			return domain.StatusFailed, "selected test no longer exists in the catalog"
		}
		if provider.Run.Test.IsZero() {
			return domain.StatusFailed, "provider has no targeted test command"
		}
		spec = provider.Run.Test
	}
	artifactPath := m.store.ArtifactPath(item.RunID, provider.ID, item.ID, resultExtension(provider.Results.Format))
	_ = os.MkdirAll(filepath.Dir(artifactPath), 0o755)
	exactVitest := test != nil && isUnsafeBuiltInVitestTarget(provider)
	exactJest := test != nil && isBuiltInJestTarget(provider)
	var cmd domain.CommandSpec
	var err error
	if exactVitest {
		cmd, err = vitestExactCommand(provider, test, m.projectRoot, artifactPath)
	} else {
		cmd, err = runner.RenderCommand(spec, runner.TemplateData{ProjectRoot: m.projectRoot, Provider: provider, Test: test, ArtifactPath: artifactPath})
	}
	if err != nil {
		return domain.StatusFailed, err.Error()
	}
	if exactJest {
		if err := checkJestProjectSelection(ctx, m.runner, cmd, test.JestProject); err != nil {
			return domain.StatusFailed, err.Error()
		}
		if test.JestProject != "" {
			cmd.Args = append(cmd.Args, "--selectProjects="+test.JestProject)
		}
	}

	preview := runner.Preview(cmd)
	if exactVitest {
		preview = fmt.Sprintf("Vitest exact project %q file %q case %q", test.VitestProject, test.File, test.Selector)
	}
	_ = m.store.AppendLog(item.RunID, fmt.Sprintf("$ %s", preview))
	result, runErr := m.runner.Run(ctx, cmd, func(event runner.Event) {
		switch event.Type {
		case runner.EventStdout:
			_ = m.store.AppendLog(item.RunID, event.Line)
			m.updateLiveStreamingStats(item.RunID, event.Line)
		case runner.EventStderr:
			_ = m.store.AppendLog(item.RunID, "ERR "+event.Line)
		}
	})

	formatPath := artifactPath
	if provider.Results.Path != "" && provider.Results.Path != "{{artifact.path}}" {
		formatPath = renderResultPath(provider.Results.Path, m.projectRoot, provider, artifactPath)
	}
	parsed, parseErr := results.Parse(provider.Results.Format, formatPath, result.Stdout)
	resultStatus := func() (bool, domain.Status) {
		if exactVitest {
			return vitestResultStatus(*test, parsed)
		}
		if exactJest {
			return jestResultStatus(*test, parsed, cmd.WorkingDir)
		}
		return selectedResultStatus(*test, parsed)
	}
	if parseErr != nil {
		_ = m.store.AppendLog(item.RunID, "RESULT PARSE ERROR "+parseErr.Error())
		parsed = nil
	}
	status, testStatus, message := domain.StatusPassed, domain.StatusPassed, ""
	if runErr != nil || result.ExitCode != 0 {
		if result.TimedOut {
			status, testStatus, message = domain.StatusTimedOut, domain.StatusTimedOut, "timed out"
		} else {
			message = strings.TrimSpace(result.Stderr)
			if message == "" && runErr != nil {
				message = runErr.Error()
			}
			if message == "" {
				message = fmt.Sprintf("exit code %d", result.ExitCode)
			}
			status, testStatus, message = domain.StatusFailed, domain.StatusFailed, trimForSummary(message)
		}
	} else if parseErr != nil {
		status, testStatus, message = domain.StatusFailed, domain.StatusUnknown, "invalid result report: "+trimForSummary(parseErr.Error())
	} else if format := strings.ToLower(strings.TrimSpace(provider.Results.Format)); format != "" && format != "none" {
		if len(parsed) == 0 {
			status, testStatus, message = domain.StatusFailed, domain.StatusUnknown, "no test results were reported"
		} else if format == "cargo-text" && test != nil && len(parsed) != 1 {
			status, testStatus, message = domain.StatusFailed, domain.StatusUnknown, "Cargo reported more than one result for a selected test"
		} else if inconclusiveResults(parsed) {
			status, testStatus, message = domain.StatusFailed, domain.StatusUnknown, "one or more test results have an unknown outcome"
		} else if test != nil {
			matched, evidenceStatus := resultStatus()
			if !matched || evidenceStatus == domain.StatusUnknown {
				status, testStatus, message = domain.StatusFailed, domain.StatusUnknown, "no conclusive result matched the selected test"
			} else {
				status, testStatus = evidenceStatus, evidenceStatus
				if parsedFailed(parsed) {
					status, message = domain.StatusFailed, "one or more tests failed"
				}
			}
		} else if parsedFailed(parsed) {
			status, testStatus, message = domain.StatusFailed, domain.StatusFailed, "one or more tests failed"
		} else if allResultsSkipped(parsed) {
			status, testStatus, message = domain.StatusSkipped, domain.StatusSkipped, "all reported tests were skipped"
		}
	}
	// A failing command does not establish that the selected test failed: it
	// may have failed before the test ran, or after a conclusive test result.
	format := strings.ToLower(strings.TrimSpace(provider.Results.Format))
	if test != nil && !result.TimedOut && (runErr != nil || result.ExitCode != 0) && format != "" && format != "none" {
		testStatus = domain.StatusUnknown
		if parseErr == nil && !inconclusiveResults(parsed) && (format != "cargo-text" || len(parsed) == 1) {
			if matched, evidenceStatus := resultStatus(); matched {
				testStatus = evidenceStatus
			}
		}
	}
	if ctx.Err() != nil {
		status, testStatus, message = domain.StatusCancelled, domain.StatusCancelled, "caller context cancelled"
	}
	selectedMessage := message
	if testStatus == domain.StatusPassed {
		selectedMessage = ""
	}
	if err := m.applyResults(item.RunID, provider, test, parsed, result, testStatus, selectedMessage); err != nil {
		return domain.StatusFailed, "could not persist test results: " + trimForSummary(err.Error())
	}
	return status, message
}

func allResultsSkipped(parsed []domain.TestResult) bool {
	if len(parsed) == 0 {
		return false
	}
	for _, result := range parsed {
		if result.Status != domain.StatusSkipped {
			return false
		}
	}
	return true
}

func inconclusiveResults(parsed []domain.TestResult) bool {
	for _, result := range parsed {
		if result.Status != domain.StatusPassed && result.Status != domain.StatusFailed && result.Status != domain.StatusSkipped {
			return true
		}
	}
	return false
}

func selectedResultStatus(test domain.TestCase, parsed []domain.TestResult) (bool, domain.Status) {
	matched, status := false, domain.StatusSkipped
	for _, result := range parsed {
		if test.ProviderKind == domain.ProviderGo && test.GoPackage != "" && result.FullName != test.FullName {
			continue
		}
		nameMatch := false
		for _, expected := range []string{test.FullName, test.Selector} {
			for _, actual := range []string{result.FullName, result.Selector} {
				if expected != "" && actual != "" && strings.EqualFold(expected, actual) {
					nameMatch = true
				}
			}
			if strings.ContainsAny(expected, ".:/\\") && strings.EqualFold(expected, result.Name) {
				nameMatch = true
			}
		}
		if !nameMatch {
			continue
		}
		matched = true
		switch result.Status {
		case domain.StatusFailed:
			return true, domain.StatusFailed
		case domain.StatusUnknown:
			status = domain.StatusUnknown
		case domain.StatusPassed:
			if status != domain.StatusUnknown {
				status = domain.StatusPassed
			}
		case domain.StatusSkipped:
		default:
			status = domain.StatusUnknown
		}
	}
	return matched, status
}

func (m *Manager) applyResults(runID string, provider domain.Provider, selected *domain.TestCase, parsed []domain.TestResult, result runner.Result, selectedStatus domain.Status, selectedMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.skipRequested {
		if selected != nil {
			test := *selected
			test.LastStatus = domain.StatusSkipped
			test.LastFailure = "skipped by user"
			test.LastDurationMillis = result.Finished.Sub(result.Started).Milliseconds()
			test.LastRunID = runID
			m.tests[test.ID] = test
			return m.updateCatalogResultsLocked([]domain.TestCase{test})
		}
		return nil
	}
	changed := make([]domain.TestCase, 0)
	if selected != nil {
		test := *selected
		test.LastStatus = selectedStatus
		test.LastFailure = trimForSummary(selectedMessage)
		test.LastDurationMillis = result.Finished.Sub(result.Started).Milliseconds()
		test.LastRunID = runID
		m.tests[test.ID] = test
		changed = append(changed, test)
	}
	format := strings.ToLower(strings.TrimSpace(provider.Results.Format))
	if selected == nil && format != "" && format != "none" && m.current != nil && m.current.ID == runID {
		// Streamed counts are provisional until the provider's final report is
		// available; replace this item's counts with the report evidence.
		m.current.Passed, m.current.Failed, m.current.Skipped = m.itemPassedBase, m.itemFailedBase, m.itemSkippedBase
		m.streamedTests = make(map[string]domain.Status)
	}
	if selected == nil {
		for _, parsedResult := range parsed {
			ids := m.matchTestCandidatesLocked(provider.ID, parsedResult)
			for _, id := range ids {
				test := m.tests[id]
				if len(ids) == 1 {
					test.LastStatus = parsedResult.Status
					test.LastFailure = trimForSummary(parsedResult.Message)
					test.LastDurationMillis = parsedResult.DurationMillis
				} else {
					test.LastStatus = domain.StatusUnknown
					test.LastFailure = "Result identity matches multiple tests"
					test.LastDurationMillis = 0
				}
				test.LastRunID = runID
				m.tests[id] = test
				changed = append(changed, test)
			}
		}
	}
	if selected == nil && len(parsed) > 0 && m.current != nil && m.current.ID == runID {
		// Each final report record represents an execution. Equal names can
		// occur in different projects or target frameworks.
		for _, pr := range parsed {
			switch pr.Status {
			case domain.StatusPassed:
				m.current.Passed++
			case domain.StatusFailed:
				m.current.Failed++
			case domain.StatusSkipped:
				m.current.Skipped++
			}
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return m.updateCatalogResultsLocked(changed)
}

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func (m *Manager) updateLiveStreamingStats(runID string, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	var testName string
	var streamKey string
	var status domain.Status

	if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
		var evt goTestEvent
		if err := json.Unmarshal([]byte(line), &evt); err == nil && evt.Test != "" {
			testName = evt.Test
			streamKey = evt.Package + "\x00" + evt.Test
			switch evt.Action {
			case "pass":
				status = domain.StatusPassed
			case "fail":
				status = domain.StatusFailed
			case "skip":
				status = domain.StatusSkipped
			}
		}
	} else if strings.HasPrefix(line, "=== RUN") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			testName = parts[2]
		}
	} else if strings.HasPrefix(line, "--- PASS:") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			testName = parts[2]
			status = domain.StatusPassed
		}
	} else if strings.HasPrefix(line, "--- FAIL:") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			testName = parts[2]
			status = domain.StatusFailed
		}
	} else if strings.HasPrefix(line, "--- SKIP:") {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			testName = parts[2]
			status = domain.StatusSkipped
		}
	} else if strings.HasPrefix(line, "PASSED ") {
		testName = strings.TrimPrefix(line, "PASSED ")
		status = domain.StatusPassed
	} else if strings.HasPrefix(line, "FAILED ") {
		testName = strings.TrimPrefix(line, "FAILED ")
		status = domain.StatusFailed
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.ID != runID {
		return
	}
	if m.current.CurrentIndex >= 0 && m.current.CurrentIndex < len(m.current.Items) && m.current.Items[m.current.CurrentIndex].TestID != "" {
		// A targeted item is counted once its report has been validated. A
		// streamed line may describe a different test selected by the runner.
		return
	}
	if testName != "" && m.current.CurrentIndex >= 0 && m.current.CurrentIndex < len(m.current.Items) {
		m.current.Items[m.current.CurrentIndex].TestName = testName
	}
	if status != "" && testName != "" {
		if streamKey == "" {
			streamKey = testName
		}
		if m.streamedTests == nil {
			m.streamedTests = make(map[string]domain.Status)
		}
		if prev, seen := m.streamedTests[streamKey]; !seen || prev != status {
			if seen {
				switch prev {
				case domain.StatusPassed:
					m.current.Passed--
				case domain.StatusFailed:
					m.current.Failed--
				case domain.StatusSkipped:
					m.current.Skipped--
				}
			}
			switch status {
			case domain.StatusPassed:
				m.current.Passed++
			case domain.StatusFailed:
				m.current.Failed++
			case domain.StatusSkipped:
				m.current.Skipped++
			}
			m.streamedTests[streamKey] = status
		}
	}
}

func (m *Manager) matchTestCandidatesLocked(providerID string, result domain.TestResult) []string {
	find := func(match func(domain.TestCase) bool) []string {
		var found []string
		for id, test := range m.tests {
			if test.ProviderID != providerID || !match(test) {
				continue
			}
			found = append(found, id)
		}
		return found
	}
	for _, test := range m.tests {
		if test.ProviderID == providerID && test.ProviderKind == domain.ProviderGo && test.GoPackage != "" {
			return find(func(candidate domain.TestCase) bool {
				return candidate.ProviderKind == domain.ProviderGo && candidate.GoPackage != "" && candidate.FullName == result.FullName
			})
		}
	}
	// Prefer the full identity over a selector that may be shared by several
	// packages. An ambiguous stronger identity must not fall back to a name.
	for _, needle := range []string{result.FullName, result.Selector} {
		if needle == "" {
			continue
		}
		matches := find(func(test domain.TestCase) bool {
			return strings.EqualFold(test.Selector, needle) || strings.EqualFold(test.FullName, needle)
		})
		if len(matches) > 0 {
			return matches
		}
	}
	if result.Name == "" {
		return nil
	}
	return find(func(test domain.TestCase) bool { return strings.EqualFold(test.Name, result.Name) })
}

func (m *Manager) buildRunItems(request RunRequest) ([]domain.RunItem, error) {
	m.mu.Lock()
	tests := m.testsSliceLocked()
	m.mu.Unlock()

	providers := m.enabledProviders()
	providerMap := make(map[string]domain.Provider, len(providers))
	for _, provider := range providers {
		providerMap[provider.ID] = provider
	}
	requestedProviders := make(map[string]struct{}, len(request.ProviderIDs))
	for _, id := range request.ProviderIDs {
		if _, ok := providerMap[id]; !ok {
			return nil, fmt.Errorf("unknown or disabled provider ID %q", id)
		}
		requestedProviders[id] = struct{}{}
	}
	wideProviders := make(map[string]struct{}, len(request.ProviderWideIDs))
	if len(request.ProviderWideIDs) > 0 && (len(request.ProviderIDs) > 0 || hasFilter(request.Filter)) {
		return nil, errors.New("provider-wide rerun items cannot be combined with provider or test filters")
	}
	for _, id := range request.ProviderWideIDs {
		if _, ok := providerMap[id]; !ok {
			return nil, fmt.Errorf("unknown or disabled provider ID %q", id)
		}
		wideProviders[id] = struct{}{}
	}
	providerOnly := len(request.ProviderIDs) > 0 && len(request.TestIDs) == 0 && !hasFilter(request.Filter)

	selected := make([]domain.TestCase, 0)
	if len(request.TestIDs) > 0 {
		byID := make(map[string]domain.TestCase, len(tests))
		for _, test := range tests {
			byID[test.ID] = test
		}
		for _, id := range request.TestIDs {
			test, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown test ID %q", id)
			}
			if len(requestedProviders) > 0 {
				if _, ok := requestedProviders[test.ProviderID]; !ok {
					return nil, fmt.Errorf("test ID %q does not belong to a requested provider", id)
				}
			}
			if _, overlaps := wideProviders[test.ProviderID]; overlaps {
				return nil, fmt.Errorf("test ID %q overlaps a provider-wide rerun item", id)
			}
			if hasFilter(request.Filter) {
				criteria := request.Filter
				criteria.Offset, criteria.Limit = 0, 0
				_, matches := filters.Apply([]domain.TestCase{test}, criteria)
				if len(matches) == 0 {
					return nil, fmt.Errorf("test ID %q does not match the requested filters", id)
				}
			}
			selected = append(selected, test)
		}
	} else if hasFilter(request.Filter) {
		candidates := tests
		if len(requestedProviders) > 0 {
			candidates = make([]domain.TestCase, 0, len(tests))
			for _, test := range tests {
				if _, ok := requestedProviders[test.ProviderID]; ok {
					candidates = append(candidates, test)
				}
			}
		}
		_, selected = filters.Apply(candidates, request.Filter)
	} else if providerOnly {
		for _, test := range tests {
			if _, ok := requestedProviders[test.ProviderID]; ok {
				selected = append(selected, test)
			}
		}
	}
	if (len(request.TestIDs) > 0 || hasFilter(request.Filter)) && len(selected) == 0 {
		return nil, errors.New("no tests matched the targeted run request")
	}

	var items []domain.RunItem
	appendProvider := func(provider domain.Provider) {
		item := domain.RunItem{ProviderID: provider.ID, ProviderName: provider.Name, ProviderKind: provider.Kind, TestName: "All " + provider.Name, CommandPreview: runner.Preview(provider.Run.All), Status: domain.StatusQueued, EstimatedMillis: m.providerEstimate(provider.ID)}
		for _, other := range providers {
			if len(requestedProviders) > 0 {
				if _, ok := requestedProviders[other.ID]; !ok {
					continue
				}
			}
			if provider.ID != other.ID && provider.Kind == domain.ProviderDotNet && other.Kind == domain.ProviderDotNet && dotNetRootsMayOverlap(provider.Root, other.Root) {
				item.Warnings = []string{"Selected .NET provider scopes may overlap; verify provider selection before running."}
				break
			}
		}
		items = append(items, item)
	}
	if len(selected) > 0 {
		wideQueued := make(map[string]bool)
		matchingProviders := make(map[string]map[string]struct{})
		for _, test := range selected {
			if test.ProviderKind != domain.ProviderDotNet || test.FullName == "" {
				continue
			}
			key := strings.ToLower(test.FullName)
			if matchingProviders[key] == nil {
				matchingProviders[key] = make(map[string]struct{})
			}
			matchingProviders[key][test.ProviderID] = struct{}{}
		}
		selectorCounts := make(map[string]int)
		for _, test := range tests {
			if test.ProviderKind == domain.ProviderDotNet {
				selectorCounts[test.ProviderID+"\x00"+strings.ToLower(test.Selector)]++
			}
		}
		for _, test := range selected {
			provider, ok := providerMap[test.ProviderID]
			if !ok {
				return nil, fmt.Errorf("provider %q for test %q is unavailable", test.ProviderID, test.ID)
			}
			spec := provider.Run.Test
			if spec.IsZero() {
				return nil, fmt.Errorf("provider %q has no targeted test command", provider.ID)
			}
			if providerOnly && isBuiltInJestTarget(provider) {
				if !wideQueued[provider.ID] {
					appendProvider(provider)
					wideQueued[provider.ID] = true
				}
				continue
			}
			if isUnsafeBuiltInVitestTarget(provider) {
				if providerOnly {
					if !wideQueued[provider.ID] {
						appendProvider(provider)
						wideQueued[provider.ID] = true
					}
					continue
				}
				if test.File == "" || test.Selector == "" {
					return nil, fmt.Errorf("test %q has no Vitest file or case identity; rediscover tests before selecting it", test.ID)
				}
			}
			for _, arg := range spec.Args {
				if strings.Contains(arg, "{{test.goPackage}}") && test.GoPackage == "" {
					return nil, fmt.Errorf("test %q has no Go package identity; rediscover tests before selecting it", test.ID)
				}
				if arg == "{{test.cargoTarget}}" && (test.CargoPackage == "" || test.CargoTargetKind == "" || test.CargoTargetName == "") {
					return nil, fmt.Errorf("test %q has no Cargo target identity; rediscover tests before selecting it", test.ID)
				}
			}
			if test.ProviderKind == domain.ProviderDotNet && usesDefaultDotNetFilter(spec) && selectorCounts[test.ProviderID+"\x00"+strings.ToLower(test.Selector)] > 1 {
				return nil, fmt.Errorf("test %q has an ambiguous .NET case display name in provider %q", test.ID, provider.ID)
			}
			preview := runner.Preview(spec)
			if isUnsafeBuiltInVitestTarget(provider) {
				preview = fmt.Sprintf("Vitest exact project %q file %q case %q", test.VitestProject, test.File, test.Selector)
			}
			if provider.Kind == domain.ProviderGo && strings.HasPrefix(test.Name, "Benchmark") {
				command, err := runner.RenderCommand(spec, runner.TemplateData{ProjectRoot: m.projectRoot, Provider: provider, Test: &test, ArtifactPath: "{{artifact.path}}"})
				if err != nil {
					return nil, err
				}
				preview = runner.Preview(command)
			}
			item := domain.RunItem{ProviderID: provider.ID, ProviderName: provider.Name, ProviderKind: provider.Kind, TestID: test.ID, TestName: test.Name, CommandPreview: preview, Status: domain.StatusQueued, EstimatedMillis: estimateMillis(test)}
			if test.ProviderKind == domain.ProviderDotNet && len(matchingProviders[strings.ToLower(test.FullName)]) > 1 {
				item.Warnings = []string{"A selected .NET test with the same name exists in another provider; their scopes may overlap. Verify provider selection before running."}
			}
			items = append(items, item)
		}
		if providerOnly {
			covered := make(map[string]bool)
			for _, test := range selected {
				covered[test.ProviderID] = true
			}
			for _, provider := range providers {
				if _, requested := requestedProviders[provider.ID]; requested && !covered[provider.ID] {
					appendProvider(provider)
				}
			}
		}
		for _, provider := range providers {
			if _, ok := wideProviders[provider.ID]; ok {
				appendProvider(provider)
			}
		}
		return items, nil
	}

	if len(wideProviders) > 0 {
		providers = filterProviders(providers, wideProviders)
	}
	if len(request.ProviderIDs) > 0 {
		providers = filterProviders(providers, requestedProviders)
	}
	for _, provider := range providers {
		appendProvider(provider)
	}
	return items, nil
}

func isBuiltInDotNetDiscovery(provider domain.Provider) bool {
	command := provider.Discover.Command
	if provider.Kind != domain.ProviderDotNet || !strings.EqualFold(provider.Discover.Parser, "dotnet-list") || strings.TrimSuffix(strings.ToLower(filepath.Base(command.Exe)), ".exe") != "dotnet" || len(command.Args) != 4 {
		return false
	}
	target := command.Args[1]
	projectFile := filepath.Base(target) == target && strings.EqualFold(filepath.Ext(target), ".csproj")
	return command.Args[0] == "test" && (target == "{{provider.root}}" || target == "." || projectFile) && command.Args[2] == "--list-tests" && command.Args[3] == "--nologo"
}

func dotNetRootsMayOverlap(a, b string) bool {
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	}
	return within(a, b) || within(b, a)
}

func usesDefaultDotNetFilter(spec domain.CommandSpec) bool {
	exe := strings.TrimSuffix(strings.ToLower(filepath.Base(spec.Exe)), ".exe")
	if exe != "dotnet" {
		return false
	}
	for index, arg := range spec.Args {
		if index > 0 && spec.Args[index-1] == "--filter" && arg == "FullyQualifiedName={{test.selector}}" {
			return true
		}
	}
	return false
}

func (m *Manager) waitIfPaused(ctx context.Context) {
	for {
		m.mu.Lock()
		paused := m.paused
		stopped := m.stopRequested
		m.mu.Unlock()
		if !paused || stopped || ctx.Err() != nil {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (m *Manager) finishRemaining(start int, status domain.Status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return
	}
	now := time.Now().UTC()
	for idx := start; idx < len(m.current.Items); idx++ {
		if m.current.Items[idx].Status == domain.StatusQueued {
			m.current.Items[idx].Status = status
			m.current.Items[idx].FinishedAt = &now
			m.current.Items[idx].Error = message
		}
	}
	m.current.Passed, m.current.Failed, m.current.Skipped, m.current.Cancelled = countItems(m.current.Items)
}

func (m *Manager) isStopRequested() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopRequested
}

func (m *Manager) enabledProviders() []domain.Provider {
	m.mu.Lock()
	defer m.mu.Unlock()
	providers := make([]domain.Provider, 0, len(m.cfg.Providers))
	for _, provider := range m.cfg.Providers {
		if provider.Enabled {
			providers = append(providers, provider)
		}
	}
	return providers
}

func (m *Manager) providerByID(id string) (domain.Provider, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, provider := range m.cfg.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return domain.Provider{}, false
}

func (m *Manager) syncCatalogFromDiskLocked() {
	if m.store == nil {
		return
	}
	modTime, err := m.store.CatalogModTime()
	if err != nil || !modTime.After(m.catalogLoadedAt) {
		return
	}
	catalog, newModTime, err := m.store.LoadCatalogWithModTime()
	if err != nil {
		return
	}
	m.tests = make(map[string]domain.TestCase, len(catalog))
	for _, test := range catalog {
		m.tests[test.ID] = test
	}
	m.catalogLoadedAt = newModTime
}

func (m *Manager) findTest(id string) *domain.TestCase {
	if id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCatalogFromDiskLocked()
	if test, ok := m.tests[id]; ok {
		copy := test
		return &copy
	}
	return nil
}

func (m *Manager) catalogMap() map[string]domain.TestCase {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCatalogFromDiskLocked()
	copy := make(map[string]domain.TestCase, len(m.tests))
	for key, value := range m.tests {
		copy[key] = value
	}
	return copy
}

func (m *Manager) testsSliceLocked() []domain.TestCase {
	m.syncCatalogFromDiskLocked()
	tests := make([]domain.TestCase, 0, len(m.tests))
	for _, test := range m.tests {
		tests = append(tests, test)
	}
	sort.SliceStable(tests, func(i, j int) bool {
		if tests[i].ProviderID != tests[j].ProviderID {
			return tests[i].ProviderID < tests[j].ProviderID
		}
		return tests[i].FullName < tests[j].FullName
	})
	return tests
}

func (m *Manager) providerEstimate(providerID string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, test := range m.tests {
		if test.ProviderID == providerID && test.LastDurationMillis > 0 {
			total += test.LastDurationMillis
		}
	}
	return total
}

func cloneRunPtr(run *domain.Run) *domain.Run {
	if run == nil {
		return nil
	}
	copy := cloneRun(*run)
	return &copy
}

func cloneRun(run domain.Run) domain.Run {
	items := make([]domain.RunItem, len(run.Items))
	copy(items, run.Items)
	for index := range items {
		items[index].Warnings = append([]string(nil), items[index].Warnings...)
		if items[index].StartedAt != nil {
			started := *items[index].StartedAt
			items[index].StartedAt = &started
		}
		if items[index].FinishedAt != nil {
			finished := *items[index].FinishedAt
			items[index].FinishedAt = &finished
		}
	}
	run.Items = items
	return run
}

func countItems(items []domain.RunItem) (passed int, failed int, skipped int, cancelled int) {
	for _, item := range items {
		switch item.Status {
		case domain.StatusPassed:
			passed++
		case domain.StatusFailed, domain.StatusTimedOut:
			failed++
		case domain.StatusSkipped:
			skipped++
		case domain.StatusCancelled:
			cancelled++
		}
	}
	return passed, failed, skipped, cancelled
}

func resultExtension(format string) string {
	switch strings.ToLower(format) {
	case "trx", "trx-dir":
		return ".trx"
	case "json", "vitest-json", "go-json":
		return ".json"
	default:
		return ".xml"
	}
}

func renderResultPath(path string, projectRoot string, provider domain.Provider, artifactPath string) string {
	rendered, err := runner.RenderCommand(domain.CommandSpec{Exe: "noop", Args: []string{path}}, runner.TemplateData{ProjectRoot: projectRoot, Provider: provider, ArtifactPath: artifactPath})
	if err != nil || len(rendered.Args) == 0 {
		return path
	}
	result := rendered.Args[0]
	if !filepath.IsAbs(result) {
		return filepath.Join(projectRoot, provider.Root, result)
	}
	return result
}

func parsedFailed(parsed []domain.TestResult) bool {
	for _, result := range parsed {
		if result.Status == domain.StatusFailed {
			return true
		}
	}
	return false
}

func estimateMillis(test domain.TestCase) int64 {
	if test.LastDurationMillis > 0 {
		return test.LastDurationMillis
	}
	return 0
}

func trimForSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4000 {
		return value[:4000] + "…"
	}
	return value
}

func hasFilter(filter domain.Filter) bool {
	return len(filter.ProviderIDs) > 0 || len(filter.ProviderKinds) > 0 || len(filter.Layers) > 0 || len(filter.Modules) > 0 || len(filter.Statuses) > 0 || len(filter.Tags) > 0 || strings.TrimSpace(filter.Query) != ""
}

func filterProviders(providers []domain.Provider, allowed map[string]struct{}) []domain.Provider {
	filtered := make([]domain.Provider, 0, len(providers))
	for _, provider := range providers {
		if _, ok := allowed[provider.ID]; ok {
			filtered = append(filtered, provider)
		}
	}
	return filtered
}

func newRunID() string {
	return time.Now().UTC().Format("20060102T150405.000000000")
}

func (m *Manager) WaitForIdle(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.Lock()
		idle := !m.processing && m.current == nil && len(m.runQueue) == 0
		m.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *Manager) Drain(ctx context.Context) error {
	return m.WaitForIdle(ctx)
}

package desktop

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thecodingrasta/testdesk/internal/agents"
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/lifecycle"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
	"github.com/thecodingrasta/testdesk/internal/prefs"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed assets/*
var assets embed.FS

type ProjectEntry struct {
	Root          string `json:"root"`
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	Configured    bool   `json:"configured"`
	LastOpened    string `json:"lastOpened"`
	ProviderCount int    `json:"providerCount"`
	TestCount     int    `json:"testCount"`
}

type ProjectActivity struct {
	Root     string `json:"root"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Selected bool   `json:"selected"`
}

type StartupState struct {
	Configured           bool                 `json:"configured"`
	ProjectRoot          string               `json:"projectRoot"`
	ProjectName          string               `json:"projectName"`
	SuggestedProjectRoot string               `json:"suggestedProjectRoot"`
	PreferencePath       string               `json:"preferencePath"`
	ConfigPath           string               `json:"configPath"`
	Providers            []domain.Provider    `json:"providers"`
	Projects             []ProjectEntry       `json:"projects"`
	UI                   domain.UIConfig      `json:"ui"`
	SetupError           string               `json:"setupError,omitempty"`
	DetectedPreview      domain.ProjectConfig `json:"detectedPreview"`
}

type windowBounds struct {
	MinWidth  int
	MinHeight int
	MaxWidth  int
	MaxHeight int
	Width     int
	Height    int
}

type SetupRequest struct {
	ProjectRoot  string                `json:"projectRoot"`
	Force        bool                  `json:"force"`
	AutoDiscover bool                  `json:"autoDiscover"`
	WidgetMode   domain.VisibilityMode `json:"widgetMode"`
	AlwaysOnTop  bool                  `json:"alwaysOnTop"`
	TopmostMode  domain.VisibilityMode `json:"topmostMode"`
}

type SetupResponse struct {
	Config          domain.ProjectConfig `json:"config"`
	Snapshot        domain.Snapshot      `json:"snapshot"`
	DiscoverWarning string               `json:"discoverWarning,omitempty"`
}

const loadingEvent = "testdesk:loading"

type LoadingState struct {
	Active        bool   `json:"active"`
	Operation     string `json:"operation"`
	Title         string `json:"title"`
	Detail        string `json:"detail"`
	Percent       int    `json:"percent,omitempty"`
	TotalTests    int    `json:"totalTests,omitempty"`
	ProviderTests int    `json:"providerTests,omitempty"`
	Current       int    `json:"current,omitempty"`
	Total         int    `json:"total,omitempty"`
	ProviderName  string `json:"providerName,omitempty"`
	LatestTest    string `json:"latestTest,omitempty"`
	StartedAt     string `json:"startedAt,omitempty"`
}

type WorkspaceResponse struct {
	Startup  StartupState    `json:"startup"`
	Snapshot domain.Snapshot `json:"snapshot"`
}

type App struct {
	ctx context.Context

	mu                   sync.RWMutex
	managers             map[string]*manager.Manager
	projectRoot          string
	suggestedProjectRoot string
	setupError           string
	windowMode           string
	compactManual        bool
	compactPositioned    bool
	trayOnce             sync.Once
	tray                 *trayService
	nativeMonitorOnce    sync.Once
	initialRoot          string
	initialConfigured    bool
	initialiseOnce       sync.Once
	initialiseErr        error
	loading              LoadingState
	lifecycleOwner       *lifecycle.Owner
	automaticStart       bool
}

func Run(projectRoot string, automatic ...bool) error {
	auto := len(automatic) > 0 && automatic[0]
	startup, err := lifecycle.TakeStartupChild()
	if err != nil {
		return err
	}
	if startup != nil {
		stopping := make(chan struct{})
		defer func() { close(stopping); _ = startup.Close() }()
		go func() {
			<-startup.Done()
			if startup.Wait() != nil {
				select {
				case <-stopping:
					return
				default:
					// Before commit no MCP client can have been admitted. Exit
					// even if native startup has stalled and cannot process Quit.
					os.Exit(1)
				}
			}
		}()
	}
	owner, err := lifecycle.Start()
	if err != nil {
		return err
	}
	defer owner.Close()
	initialRoot, err := resolveInitialProject(projectRoot)
	if err != nil {
		return err
	}
	configured := config.Exists(initialRoot)
	app := &App{
		suggestedProjectRoot: initialRoot,
		managers:             map[string]*manager.Manager{},
		windowMode:           "manager",
		automaticStart:       auto,
		initialRoot:          initialRoot,
		initialConfigured:    configured,
		lifecycleOwner:       owner,
		loading: LoadingState{
			Active: true, Operation: "startup", Title: "Opening TestDesk",
			Detail: "Preparing your test workspace…", StartedAt: time.Now().Format(time.RFC3339Nano),
		},
	}
	if auto {
		app.windowMode = "manager_hidden"
	}
	assetRoot, err := fs.Sub(assets, "assets")
	if err != nil {
		return err
	}

	ui := safeUI(app.uiConfig())
	if configured {
		if cfg, loadErr := config.Load(initialRoot); loadErr == nil {
			ui = safeUI(cfg.UI)
		}
	}
	initialWidth, initialHeight := 900, 810
	minWidth, minHeight := 760, 600
	if configured {
		initialWidth, initialHeight = ui.ManagerWidth, ui.ManagerHeight
		minWidth, minHeight = 920, 560
	}
	return wails.Run(&options.App{
		Title:       "TestDesk",
		Width:       initialWidth,
		Height:      initialHeight,
		MinWidth:    minWidth,
		MinHeight:   minHeight,
		AlwaysOnTop: false,
		StartHidden: auto,
		// Standard native chrome is the v1 default. It provides reliable movement,
		// resizing and platform window controls across Windows, Linux and macOS.
		Frameless:         false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 17, G: 24, B: 42, A: 255},
		OnBeforeClose: func(ctx context.Context) bool {
			if app.hasTray() {
				_ = app.SetWindowMode("manager_hidden")
				return true
			}
			return false
		},
		AssetServer: &assetserver.Options{Assets: assetRoot},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
		},
		OnDomReady: func(ctx context.Context) {
			app.ctx = ctx
			app.startTrayOnce()
			app.mu.RLock()
			tray := app.tray
			app.mu.RUnlock()
			if tray != nil {
				go func() {
					select {
					case <-tray.ready:
						if startup != nil {
							if err := startup.Ready(); err != nil {
								return
							}
							select {
							case <-startup.Done():
								if startup.Wait() != nil {
									return
								}
							case <-ctx.Done():
								return
							}
						}
						owner.SetMCPHandler(func(input io.Reader, output io.Writer) {
							server := mcp.NewWithManagerResolver(app.managerForProject)
							_ = server.ServeConnection(input, output)
						})
					case <-ctx.Done():
					}
				}()
			}
			app.startNativeWindowMonitor()
			app.emitLoading(app.LoadingState())
		},
		Bind: []interface{}{app},
	})
}

func resolveInitialProject(projectRoot string) (string, error) {
	if projectRoot != "" {
		return filepath.Abs(projectRoot)
	}
	userPrefs, err := prefs.Load()
	if err == nil {
		for _, project := range userPrefs.Projects {
			if config.Exists(project.Root) {
				return filepath.Abs(project.Root)
			}
		}
		if userPrefs.LastProjectRoot != "" && config.Exists(userPrefs.LastProjectRoot) {
			return filepath.Abs(userPrefs.LastProjectRoot)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(wd)
}

func (a *App) loadProject(projectRoot string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	mgr, err := a.managerForProject(root)
	if err != nil {
		return err
	}
	if err := mgr.UpdateConfig(cfg); err != nil {
		return err
	}
	if _, err := prefs.UpsertProject(root, cfg.Name); err != nil {
		return err
	}

	a.mu.Lock()
	a.managers[root] = mgr
	a.projectRoot = root
	a.suggestedProjectRoot = root
	a.setupError = ""
	a.mu.Unlock()
	return nil
}

// managerForProject owns one manager per desktop project, independent of the
// project currently selected in the UI or by an MCP session.
func (a *App) managerForProject(projectRoot string) (*manager.Manager, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.managers == nil {
		a.managers = make(map[string]*manager.Manager)
	}
	for existingRoot, mgr := range a.managers {
		if samePath(existingRoot, root) {
			return mgr, nil
		}
	}
	mgr, err := manager.NewFromProject(root)
	if err != nil {
		return nil, err
	}
	a.managers[root] = mgr
	return mgr, nil
}

func (a *App) uiConfig() domain.UIConfig {
	a.mu.RLock()
	mgr := a.activeManagerLocked()
	suggested := a.suggestedProjectRoot
	a.mu.RUnlock()
	if mgr != nil {
		return mgr.Config().UI
	}
	return config.Normalise(suggested, domain.ProjectConfig{}).UI
}

func (a *App) Startup() StartupState {
	return a.currentStartupState()
}

func (a *App) Initialise(includeTests bool) (WorkspaceResponse, error) {
	a.initialiseOnce.Do(func() {
		if a.initialConfigured {
			a.updateLoading("startup", "Opening your project", filepath.Base(a.initialRoot))
			if err := a.loadProject(a.initialRoot); err != nil {
				a.mu.Lock()
				a.setupError = err.Error()
				a.mu.Unlock()
				a.initialiseErr = err
			}
		}
	})
	if a.initialiseErr != nil {
		a.finishLoading("startup", "TestDesk needs attention", a.initialiseErr.Error())
		return WorkspaceResponse{Startup: a.currentStartupState()}, a.initialiseErr
	}
	response, err := a.workspaceResponse(includeTests)
	if err != nil {
		a.finishLoading("startup", "TestDesk needs attention", err.Error())
		return WorkspaceResponse{}, err
	}
	if a.automaticStart {
		response.Startup.UI.StartMinimized = true
	}
	a.finishLoading("startup", "TestDesk is ready", response.Startup.ProjectName)
	return response, nil
}

func (a *App) LoadingState() LoadingState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.loading
}

func (a *App) beginLoading(operation string, title string, detail string) {
	state := LoadingState{Active: true, Operation: operation, Title: title, Detail: detail, StartedAt: time.Now().Format(time.RFC3339Nano)}
	a.mu.Lock()
	a.loading = state
	a.mu.Unlock()
	a.emitLoading(state)
}

func (a *App) updateLoading(operation string, title string, detail string) {
	a.updateLoadingWithProgress(operation, title, detail, 0)
}

func (a *App) updateLoadingWithProgress(operation string, title string, detail string, percent int) {
	a.updateLoadingState(LoadingState{
		Operation: operation,
		Title:     title,
		Detail:    detail,
		Percent:   percent,
	})
}

func (a *App) updateLoadingState(state LoadingState) {
	a.mu.Lock()
	if state.StartedAt == "" {
		state.StartedAt = a.loading.StartedAt
		if state.StartedAt == "" {
			state.StartedAt = time.Now().Format(time.RFC3339Nano)
		}
	}
	state.Active = true
	a.loading = state
	a.mu.Unlock()
	a.emitLoading(state)
}

func (a *App) reportDiscoverProgress(operation string, p manager.DiscoverProgress) {
	detail := p.Detail
	if detail == "" {
		detail = "Scanning configured providers…"
	}
	a.updateLoadingState(LoadingState{
		Operation:     operation,
		Title:         "Discovering tests",
		Detail:        detail,
		TotalTests:    p.DiscoveredTotal,
		ProviderTests: p.ProviderTests,
		Current:       p.Current,
		Total:         p.Total,
		ProviderName:  p.Provider.Name,
		LatestTest:    p.LatestTest,
	})
}

func (a *App) finishLoading(operation string, title string, detail string) {
	state := LoadingState{Active: false, Operation: operation, Title: title, Detail: detail}
	a.mu.Lock()
	a.loading = state
	a.mu.Unlock()
	a.emitLoading(state)
}

func (a *App) emitLoading(state LoadingState) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, loadingEvent, state)
	}
}

func (a *App) workspaceResponse(includeTests bool) (WorkspaceResponse, error) {
	state := a.currentStartupState()
	if !state.Configured {
		return WorkspaceResponse{Startup: state}, nil
	}
	limit := 0
	if includeTests {
		limit = 10000
	}
	a.updateLoading(a.LoadingState().Operation, "Loading test catalogue", state.ProjectName)
	snapshot, err := a.Snapshot(domain.Filter{Limit: limit})
	if err != nil {
		return WorkspaceResponse{}, err
	}
	return WorkspaceResponse{Startup: state, Snapshot: snapshot}, nil
}

func (a *App) currentStartupState() StartupState {
	a.mu.RLock()
	mgr := a.activeManagerLocked()
	root := a.projectRoot
	suggested := a.suggestedProjectRoot
	setupError := a.setupError
	a.mu.RUnlock()

	preferencePath, _ := prefs.Path()
	if suggested == "" {
		suggested, _ = os.Getwd()
	}
	state := StartupState{
		Configured:           mgr != nil,
		ProjectRoot:          root,
		SuggestedProjectRoot: suggested,
		PreferencePath:       preferencePath,
		ConfigPath:           filepath.Join(suggested, config.FileName),
		SetupError:           setupError,
		Projects:             a.projectEntries(root),
	}
	if mgr != nil {
		cfg := mgr.Config()
		state.ProjectName = cfg.Name
		state.ConfigPath = filepath.Join(root, config.FileName)
		state.Providers = cfg.Providers
		state.UI = cfg.UI
		state.DetectedPreview = cfg
	} else {
		preview := config.Detect(suggested)
		state.DetectedPreview = preview
		state.UI = preview.UI
		state.Providers = preview.Providers
	}
	return state
}

func (a *App) projectEntries(activeRoot string) []ProjectEntry {
	userPrefs, err := prefs.Load()
	if err != nil {
		return nil
	}
	entries := make([]ProjectEntry, 0, len(userPrefs.Projects))
	for _, project := range userPrefs.Projects {
		entry := ProjectEntry{
			Root:       project.Root,
			Name:       project.Name,
			LastOpened: project.LastOpenedAt.Format(time.RFC3339),
			Active:     samePath(project.Root, activeRoot),
			Configured: config.Exists(project.Root),
		}
		if entry.Configured {
			if cfg, loadErr := config.Load(project.Root); loadErr == nil && cfg.Name != "" {
				if cfg.Name != entry.Name {
					entry.Name = cfg.Name
				}
				entry.ProviderCount = len(cfg.Providers)
			}
			a.mu.RLock()
			mgr := a.managers[project.Root]
			a.mu.RUnlock()
			if mgr != nil {
				if snap, err := mgr.Snapshot(domain.Filter{Limit: 1}); err == nil {
					entry.ProviderCount = snap.ProviderCount
					entry.TestCount = snap.TestCount
				}
			}
		}
		if entry.Name == "" {
			entry.Name = filepath.Base(project.Root)
		}
		entries = append(entries, entry)
	}
	return entries
}

func (a *App) ListProjects() []ProjectEntry {
	a.mu.RLock()
	active := a.projectRoot
	a.mu.RUnlock()
	return a.projectEntries(active)
}

// ActiveProjects includes managers loaded by MCP even when they have not been
// added to the persistent project registry.
func (a *App) ActiveProjects() []ProjectActivity {
	a.mu.RLock()
	selected := a.projectRoot
	managers := make([]*manager.Manager, 0, len(a.managers))
	seen := make(map[*manager.Manager]bool, len(a.managers))
	for _, mgr := range a.managers {
		if mgr != nil && !seen[mgr] {
			seen[mgr] = true
			managers = append(managers, mgr)
		}
	}
	a.mu.RUnlock()
	projects := make([]ProjectActivity, 0, len(managers))
	for _, mgr := range managers {
		root := mgr.ProjectRoot()
		projects = append(projects, ProjectActivity{Root: root, Name: mgr.Config().Name, Active: mgr.IsActive(), Selected: samePath(root, selected)})
	}
	sort.Slice(projects, func(i, j int) bool { return strings.ToLower(projects[i].Root) < strings.ToLower(projects[j].Root) })
	return projects
}

func (a *App) SwitchProject(projectRoot string, includeTests bool) (WorkspaceResponse, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return WorkspaceResponse{}, errors.New("project root is required")
	}
	if !config.Exists(projectRoot) {
		return WorkspaceResponse{}, fmt.Errorf("%s does not contain %s", projectRoot, config.FileName)
	}
	a.beginLoading("switch-project", "Switching project", filepath.Base(projectRoot))
	if err := a.loadProject(projectRoot); err != nil {
		a.finishLoading("switch-project", "Project switch failed", err.Error())
		return WorkspaceResponse{}, err
	}
	a.updateLoading("switch-project", "Preparing project", "Reading providers, history and saved test status…")
	response, err := a.workspaceResponse(includeTests)
	if err != nil {
		a.finishLoading("switch-project", "Project switch failed", err.Error())
		return WorkspaceResponse{}, err
	}
	a.finishLoading("switch-project", "Project ready", response.Startup.ProjectName)
	return response, nil
}

func (a *App) RefreshProject(projectRoot string, includeTests bool) (WorkspaceResponse, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return WorkspaceResponse{}, errors.New("project root is required")
	}
	a.beginLoading("refresh-project", "Refreshing project", filepath.Base(projectRoot))
	projectManager, err := a.managerForProject(projectRoot)
	if err != nil {
		a.finishLoading("refresh-project", "Refresh failed", err.Error())
		return WorkspaceResponse{}, err
	}
	a.updateLoadingWithProgress("refresh-project", "Discovering tests", "Running configured provider discovery commands…", 15)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := projectManager.DiscoverWithProgress(ctx, func(progress manager.DiscoverProgress) {
		a.reportDiscoverProgress("refresh-project", progress)
	}); err != nil {
		a.finishLoading("refresh-project", "Refresh failed", err.Error())
		return WorkspaceResponse{}, err
	}
	if samePath(projectRoot, a.activeProjectRoot()) {
		if err := a.loadProject(projectRoot); err != nil {
			a.finishLoading("refresh-project", "Refresh failed", err.Error())
			return WorkspaceResponse{}, err
		}
	}
	response, err := a.workspaceResponse(includeTests)
	if err != nil {
		a.finishLoading("refresh-project", "Refresh failed", err.Error())
		return WorkspaceResponse{}, err
	}
	a.finishLoading("refresh-project", "Project refreshed", filepath.Base(projectRoot))
	return response, nil
}
func (a *App) RenameProject(projectRoot string, displayName string) (StartupState, error) {
	displayName = strings.TrimSpace(displayName)
	if strings.TrimSpace(projectRoot) == "" {
		return StartupState{}, errors.New("project root is required")
	}
	if displayName == "" {
		return StartupState{}, errors.New("project display name cannot be empty")
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return StartupState{}, err
	}

	// 1. If testdesk.json exists on disk, update cfg.Name and save it
	if config.Exists(absRoot) {
		if cfg, err := config.Load(absRoot); err == nil {
			cfg.Name = displayName
			_ = config.Save(absRoot, cfg)
			a.mu.RLock()
			mgr := a.managers[absRoot]
			a.mu.RUnlock()
			if mgr != nil {
				_ = mgr.UpdateConfig(cfg)
			}
		}
	}

	// 2. Update user preferences
	if _, err := prefs.RenameProject(absRoot, displayName); err != nil {
		return StartupState{}, err
	}

	return a.currentStartupState(), nil
}
func (a *App) ForgetProject(projectRoot string) (StartupState, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return StartupState{}, errors.New("project root is required")
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return StartupState{}, err
	}
	if _, err := prefs.ForgetProject(absRoot); err != nil {
		return StartupState{}, err
	}
	a.mu.Lock()
	delete(a.managers, absRoot)
	if samePath(a.projectRoot, absRoot) {
		a.projectRoot = ""
		for _, entry := range a.projectEntries("") {
			if entry.Configured {
				a.projectRoot = entry.Root
				break
			}
		}
	}
	a.mu.Unlock()
	if a.projectRoot != "" {
		_ = a.loadProject(a.projectRoot)
	}
	return a.currentStartupState(), nil
}

func (a *App) ChooseProject() (string, error) {
	if a.ctx == nil {
		return "", errors.New("desktop context is not ready")
	}
	state := a.currentStartupState()
	chosen, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose a project folder for TestDesk",
		DefaultDirectory:     state.SuggestedProjectRoot,
		CanCreateDirectories: false,
		ShowHiddenFiles:      true,
	})
	if err != nil {
		return "", err
	}
	if chosen == "" {
		return "", nil
	}
	abs, err := filepath.Abs(chosen)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.suggestedProjectRoot = abs
	a.mu.Unlock()
	return abs, nil
}

func (a *App) PreviewProject(projectRoot string) (domain.ProjectConfig, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return domain.ProjectConfig{}, err
	}
	if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
		if err == nil {
			err = fmt.Errorf("%s is not a directory", root)
		}
		return domain.ProjectConfig{}, err
	}
	cfg := config.Detect(root)
	a.mu.Lock()
	a.suggestedProjectRoot = root
	a.mu.Unlock()
	return cfg, nil
}

func (a *App) SetupProject(request SetupRequest) (response SetupResponse, resultErr error) {
	root := request.ProjectRoot
	if root == "" {
		root = a.currentStartupState().SuggestedProjectRoot
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return SetupResponse{}, err
	}
	a.beginLoading("setup-project", "Setting up project", filepath.Base(absRoot))
	defer func() {
		if resultErr != nil {
			a.finishLoading("setup-project", "Project setup failed", resultErr.Error())
		}
	}()
	if stat, err := os.Stat(absRoot); err != nil || !stat.IsDir() {
		if err == nil {
			err = fmt.Errorf("%s is not a directory", absRoot)
		}
		return SetupResponse{}, err
	}

	var cfg domain.ProjectConfig
	if config.Exists(absRoot) && !request.Force {
		a.updateLoading("setup-project", "Reading project configuration", filepath.Base(absRoot))
		cfg, err = config.Load(absRoot)
		if err != nil {
			return SetupResponse{}, err
		}
	} else {
		a.updateLoading("setup-project", "Detecting test providers", filepath.Base(absRoot))
		cfg = config.Detect(absRoot)
	}
	if request.WidgetMode != "" {
		cfg.UI.WidgetMode = request.WidgetMode
	}
	if request.TopmostMode != "" {
		cfg.UI.TopmostMode = request.TopmostMode
	} else if request.AlwaysOnTop {
		cfg.UI.TopmostMode = domain.VisibilityAlways
	} else {
		cfg.UI.TopmostMode = domain.VisibilityOff
	}
	cfg.UI.AlwaysOnTop = cfg.UI.TopmostMode == domain.VisibilityAlways
	if err := config.Save(absRoot, cfg); err != nil {
		return SetupResponse{}, err
	}

	if err := a.loadProject(absRoot); err != nil {
		return SetupResponse{}, err
	}
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, shouldKeepWidgetOnTop(cfg.UI, false))
	}

	mgr := a.manager()
	var discoverWarning string
	if request.AutoDiscover && mgr != nil {
		a.updateLoadingWithProgress("setup-project", "Discovering tests", "Scanning configured providers…", 15)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		_, discoverErr := mgr.DiscoverWithProgress(ctx, func(progress manager.DiscoverProgress) {
			a.reportDiscoverProgress("setup-project", progress)
		})
		cancel()
		if discoverErr != nil {
			discoverWarning = discoverErr.Error()
		}
	}
	a.updateLoadingWithProgress("setup-project", "Preparing test catalogue", cfg.Name, 90)
	snapshot, err := a.Snapshot(domain.Filter{Limit: 5000})
	if err != nil {
		return SetupResponse{}, err
	}
	response = SetupResponse{Config: cfg, Snapshot: snapshot, DiscoverWarning: discoverWarning}
	a.finishLoading("setup-project", "Project ready", cfg.Name)
	return response, nil
}

func (a *App) Snapshot(filter domain.Filter) (domain.Snapshot, error) {
	mgr := a.manager()
	if mgr == nil {
		return domain.Snapshot{ProjectName: "TestDesk", GeneratedAt: time.Now().UTC()}, nil
	}
	return mgr.Snapshot(filter)
}

func (a *App) Discover(includeTests bool) (response WorkspaceResponse, resultErr error) {
	mgr := a.manager()
	if mgr == nil {
		return WorkspaceResponse{}, errors.New("set up a project before discovering tests")
	}
	a.beginLoading("discover", "Discovering tests", mgr.Config().Name)
	defer func() {
		if resultErr != nil {
			a.finishLoading("discover", "Discovery failed", resultErr.Error())
		}
	}()
	if _, err := mgr.DiscoverWithProgress(context.Background(), func(progress manager.DiscoverProgress) {
		a.reportDiscoverProgress("discover", progress)
	}); err != nil {
		return WorkspaceResponse{}, err
	}
	a.updateLoading("discover", "Preparing test catalogue", mgr.Config().Name)
	response, resultErr = a.workspaceResponse(includeTests)
	if resultErr != nil {
		return WorkspaceResponse{}, resultErr
	}
	a.finishLoading("discover", "Discovery complete", response.Startup.ProjectName)
	return response, nil
}

func (a *App) RunTests(request manager.RunRequest) (domain.Run, error) {
	mgr := a.manager()
	if mgr == nil {
		return domain.Run{}, errors.New("set up a project before running tests")
	}
	run, err := mgr.Run(context.Background(), request)
	if err == nil {
		ui := safeUI(a.uiConfig())
		if ui.WidgetMode != domain.VisibilityOff {
			_ = a.SetWindowMode("compact_auto")
		}
		_ = a.ApplyWidgetVisibility(true)
	}
	return run, err
}

func (a *App) Control(action string) error {
	mgr := a.manager()
	if mgr == nil {
		return errors.New("set up a project before controlling runs")
	}
	return mgr.Control(manager.ControlAction(action))
}

func (a *App) Diagnostics(runID string, maxBytes int64) (manager.Diagnostics, error) {
	mgr := a.manager()
	if mgr == nil {
		return manager.Diagnostics{}, errors.New("set up a project before reading diagnostics")
	}
	return mgr.Diagnostics(runID, maxBytes)
}

func (a *App) MCPWorkspaces() ([]agents.Workspace, error) {
	root := a.activeProjectRoot()
	if strings.TrimSpace(root) == "" {
		root = a.suggestedProjectRoot
	}
	installer, err := agents.NewInstaller(root, "")
	if err != nil {
		return nil, err
	}
	return installer.Detect()
}

func (a *App) InstallMCP(target string) (agents.Workspace, error) {
	root := a.activeProjectRoot()
	if strings.TrimSpace(root) == "" {
		return agents.Workspace{}, errors.New("set up or switch to a project before installing MCP")
	}
	installer, err := agents.NewInstaller(root, "")
	if err != nil {
		return agents.Workspace{}, err
	}
	return installer.Install(agents.TargetID(target))
}

func (a *App) InstallSkills() (agents.SkillInstallResult, error) {
	root := a.activeProjectRoot()
	if strings.TrimSpace(root) == "" {
		root = a.suggestedProjectRoot
	}
	if strings.TrimSpace(root) == "" {
		return agents.SkillInstallResult{}, errors.New("set up or switch to a project before installing agent skills")
	}
	return agents.InstallSkills(root)
}

func (a *App) SaveUI(widgetMode string, topmostMode string, logRetentionDays int, widgetSize string) (domain.UIConfig, error) {
	mgr := a.manager()
	if mgr == nil {
		return domain.UIConfig{}, errors.New("set up a project before changing UI preferences")
	}
	cfg := mgr.Config()
	if widgetMode != "" {
		cfg.UI.WidgetMode = domain.VisibilityMode(widgetMode)
	}
	if topmostMode != "" {
		cfg.UI.TopmostMode = domain.VisibilityMode(topmostMode)
	}
	if widgetSize != "" && widgetSize != cfg.UI.WidgetSize {
		cfg.UI.WidgetSize = widgetSize
		switch strings.ToLower(widgetSize) {
		case "small":
			cfg.UI.CompactWidth = 380
			cfg.UI.CompactHeight = 96
			cfg.UI.CompactExpandedHeight = 180
		case "large":
			cfg.UI.CompactWidth = 580
			cfg.UI.CompactHeight = 118
			cfg.UI.CompactExpandedHeight = 230
		default:
			cfg.UI.WidgetSize = "medium"
			cfg.UI.CompactWidth = 480
			cfg.UI.CompactHeight = 106
			cfg.UI.CompactExpandedHeight = 200
		}
	}
	if logRetentionDays >= 0 {
		cfg.UI.LogRetentionDays = logRetentionDays
	}
	cfg.UI.AlwaysOnTop = cfg.UI.TopmostMode == domain.VisibilityAlways
	cfg.UI = safeUI(cfg.UI)
	if err := config.Save(a.activeProjectRoot(), cfg); err != nil {
		return domain.UIConfig{}, err
	}
	if err := mgr.UpdateConfig(cfg); err != nil {
		return domain.UIConfig{}, err
	}
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, shouldKeepWidgetOnTop(cfg.UI, false))
	}
	return cfg.UI, nil
}

func (a *App) ResetWidgetPosition() error {
	mgr := a.manager()
	if mgr == nil {
		return errors.New("set up a project before resetting the compact controller")
	}
	cfg := mgr.Config()
	cfg.UI.CompactX = 0
	cfg.UI.CompactY = 0
	cfg.UI.CompactPositionSet = false
	if err := config.Save(a.activeProjectRoot(), cfg); err != nil {
		return err
	}
	if err := mgr.UpdateConfig(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.compactPositioned = false
	mode := a.windowMode
	a.mu.Unlock()
	if a.ctx != nil && mode == "compact" {
		a.positionCompactOnce(cfg.UI, cfg.UI.CompactHeight)
	}
	return nil
}

func (a *App) SetWindowMode(mode string) error {
	if a.ctx == nil {
		return nil
	}
	ui := a.uiConfig()
	if ui.CompactWidth == 0 {
		ui = config.Normalise(a.suggestedProjectRoot, domain.ProjectConfig{}).UI
	}
	ui = safeUI(ui)
	requestedMode := mode
	if mode == "compact_auto" {
		mode = "compact"
	}
	if mode == "manager_hidden" {
		mode = "manager"
	}
	a.mu.RLock()
	previousMode := a.windowMode
	a.mu.RUnlock()
	if previousMode != mode {
		a.captureWindowGeometry(previousMode)
	}
	a.mu.Lock()
	a.windowMode = mode
	if requestedMode == "manager_hidden" {
		a.windowMode = requestedMode
	}
	a.compactManual = requestedMode == "compact"
	if mode != "compact" {
		a.compactPositioned = false
	}
	a.mu.Unlock()
	switch mode {
	case "compact":
		runtime.WindowUnmaximise(a.ctx)
		nativeSetCompactFrame(a.ctx, true, ui.CompactWidth, ui.CompactHeight)
		runtime.WindowSetMaxSize(a.ctx, ui.CompactWidth, ui.CompactExpandedHeight)
		runtime.WindowSetMinSize(a.ctx, ui.CompactWidth, ui.CompactHeight)
		runtime.WindowSetSize(a.ctx, ui.CompactWidth, ui.CompactHeight)
		runtime.WindowSetAlwaysOnTop(a.ctx, shouldKeepWidgetOnTop(ui, a.runActive()))
		a.positionCompactOnce(ui, ui.CompactHeight)
		nativeShowCompact(a.ctx, requestedMode == "compact")
	case "manager":
		bounds := boundsForWindowMode(mode, ui)
		if previousMode != "manager" {
			nativeSetCompactFrame(a.ctx, false, bounds.Width, bounds.Height)
			runtime.WindowSetMaxSize(a.ctx, bounds.MaxWidth, bounds.MaxHeight)
			runtime.WindowSetMinSize(a.ctx, bounds.MinWidth, bounds.MinHeight)
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			runtime.WindowSetSize(a.ctx, bounds.Width, bounds.Height)
		}
		if requestedMode == "manager_hidden" {
			runtime.WindowHide(a.ctx)
		} else {
			runtime.WindowShow(a.ctx)
			if previousMode == "compact" || previousMode == "" {
				runtime.WindowCenter(a.ctx)
			}
		}
	case "setup":
		bounds := boundsForWindowMode(mode, ui)
		if previousMode != "setup" {
			nativeSetCompactFrame(a.ctx, false, bounds.Width, bounds.Height)
			runtime.WindowSetMaxSize(a.ctx, bounds.MaxWidth, bounds.MaxHeight)
			runtime.WindowSetMinSize(a.ctx, bounds.MinWidth, bounds.MinHeight)
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			runtime.WindowSetSize(a.ctx, bounds.Width, bounds.Height)
			runtime.WindowCenter(a.ctx)
		}
		runtime.WindowShow(a.ctx)
	case "settings":
		bounds := boundsForWindowMode(mode, ui)
		if previousMode != "settings" {
			nativeSetCompactFrame(a.ctx, false, bounds.Width, bounds.Height)
			runtime.WindowSetMaxSize(a.ctx, bounds.MaxWidth, bounds.MaxHeight)
			runtime.WindowSetMinSize(a.ctx, bounds.MinWidth, bounds.MinHeight)
			runtime.WindowSetAlwaysOnTop(a.ctx, false)
			runtime.WindowSetSize(a.ctx, bounds.Width, bounds.Height)
			runtime.WindowCenter(a.ctx)
		}
		runtime.WindowShow(a.ctx)
	case "hide":
		runtime.WindowHide(a.ctx)
	default:
		return fmt.Errorf("unknown window mode %q", mode)
	}
	runtime.EventsEmit(a.ctx, "testdesk:window-mode", mode)
	return nil
}

func boundsForWindowMode(mode string, ui domain.UIConfig) windowBounds {
	switch mode {
	case "setup":
		return windowBounds{MinWidth: 900, MinHeight: 810, MaxWidth: 900, MaxHeight: 810, Width: 900, Height: 810}
	case "settings":
		return windowBounds{MinWidth: 1160, MinHeight: 795, MaxWidth: 1160, MaxHeight: 795, Width: 1160, Height: 795}
	default:
		return windowBounds{MinWidth: 920, MinHeight: 560, Width: ui.ManagerWidth, Height: ui.ManagerHeight}
	}
}

func (a *App) ApplyWidgetVisibility(_ bool) error {
	if a.ctx == nil {
		return nil
	}
	active := a.runActive()
	a.mu.RLock()
	mode := a.windowMode
	manual := a.compactManual
	a.mu.RUnlock()
	ui := safeUI(a.uiConfig())
	targetMode, err := widgetVisibilityTarget(mode, ui.WidgetMode, active, manual)
	if err != nil {
		return err
	}
	if targetMode != "" {
		return a.SetWindowMode(targetMode)
	}
	if mode != "compact" {
		return nil
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, shouldKeepWidgetOnTop(ui, active))
	a.positionCompactOnce(ui, ui.CompactHeight)
	nativeShowCompact(a.ctx, false)
	return nil
}

func widgetVisibilityTarget(currentMode string, visibility domain.VisibilityMode, active bool, manual bool) (string, error) {
	if manual && currentMode == "compact" {
		return "", nil
	}
	switch visibility {
	case domain.VisibilityOff:
		if currentMode == "compact" && !manual {
			return "manager_hidden", nil
		}
	case domain.VisibilityAlways:
		if currentMode == "manager_hidden" {
			return "compact_auto", nil
		}
	case domain.VisibilityWhenActive, "":
		if shouldDisplayWidget(visibility, active, manual) {
			if currentMode == "manager_hidden" {
				return "compact_auto", nil
			}
		} else if currentMode == "compact" && !manual {
			return "manager_hidden", nil
		}
	default:
		return "", fmt.Errorf("unknown widget mode %q", visibility)
	}
	return "", nil
}

func (a *App) startNativeWindowMonitor() {
	a.nativeMonitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-a.ctx.Done():
					return
				case <-ticker.C:
					active := a.runActive()
					_, _ = a.HandleNativeWindowState(active)
					_ = a.ApplyWidgetVisibility(active)
				}
			}
		}()
	})
}

// HandleNativeWindowState lets the standard OS minimise button participate in
// widget policy without recreating native window controls in HTML.
func (a *App) HandleNativeWindowState(active bool) (string, error) {
	if a.ctx == nil || !nativeWindowIsMinimised(a.ctx) {
		return "", nil
	}
	a.mu.RLock()
	mode := a.windowMode
	a.mu.RUnlock()
	if mode == "compact" {
		return "", nil
	}
	ui := safeUI(a.uiConfig())
	if ui.WidgetMode == domain.VisibilityOff {
		return "", nil
	}
	runtime.WindowUnminimise(a.ctx)
	if err := a.SetWindowMode("compact_auto"); err != nil {
		return "", err
	}
	return "compact", nil
}
func (a *App) SetCompactExpanded(expanded bool) error {
	if a.ctx == nil {
		return nil
	}
	a.mu.RLock()
	mode := a.windowMode
	a.mu.RUnlock()
	if mode != "compact" {
		return nil
	}
	ui := safeUI(a.uiConfig())
	height := ui.CompactHeight
	if expanded {
		height = ui.CompactExpandedHeight
	}
	runtime.WindowSetMinSize(a.ctx, ui.CompactWidth, ui.CompactHeight)
	runtime.WindowSetMaxSize(a.ctx, ui.CompactWidth, ui.CompactExpandedHeight)
	runtime.WindowSetSize(a.ctx, ui.CompactWidth, height)
	return nil
}

func (a *App) WindowMinimise() {
	if a.ctx != nil {
		runtime.WindowMinimise(a.ctx)
	}
}

func (a *App) WindowToggleMaximise() {
	if a.ctx != nil {
		runtime.WindowToggleMaximise(a.ctx)
	}
}

func (a *App) CloseWidget() error {
	if a.ctx == nil {
		return nil
	}
	return a.SetWindowMode("manager_hidden")
}
func (a *App) WindowClose() {
	if a.ctx == nil {
		return
	}
	if !a.hasTray() {
		a.Quit()
		return
	}
	ui := safeUI(a.uiConfig())
	if ui.WidgetMode == domain.VisibilityAlways || (ui.WidgetMode == domain.VisibilityWhenActive && a.runActive()) {
		_ = a.SetWindowMode("compact_auto")
		return
	}
	_ = a.SetWindowMode("manager_hidden")
}

func (a *App) WindowQuit() {
	a.Quit()
}

func (a *App) WindowSize() map[string]int {
	if a.ctx == nil {
		return map[string]int{"width": 0, "height": 0}
	}
	width, height := runtime.WindowGetSize(a.ctx)
	return map[string]int{"width": width, "height": height}
}

func (a *App) ResizeWindow(width int, height int) error {
	if a.ctx == nil {
		return nil
	}
	a.mu.RLock()
	mode := a.windowMode
	a.mu.RUnlock()
	minWidth, minHeight := 920, 560
	if mode == "setup" {
		minWidth, minHeight = 900, 810
	}
	if mode == "settings" {
		minWidth, minHeight = 1160, 795
	}
	if width < minWidth {
		width = minWidth
	}
	if height < minHeight {
		height = minHeight
	}
	if width > 2200 {
		width = 2200
	}
	if height > 1400 {
		height = 1400
	}
	runtime.WindowSetSize(a.ctx, width, height)
	return nil
}

func (a *App) Quit() {
	if a.lifecycleOwner != nil {
		_ = a.lifecycleOwner.Close()
	}
	a.mu.RLock()
	mode := a.windowMode
	a.mu.RUnlock()
	a.captureWindowGeometry(mode)
	a.mu.Lock()
	tray := a.tray
	a.tray = nil
	a.mu.Unlock()
	if tray != nil {
		tray.stop()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
		return
	}
	os.Exit(0)
}

func (a *App) hasTray() bool {
	a.mu.RLock()
	tray := a.tray
	a.mu.RUnlock()
	return tray != nil && tray.available()
}

func (a *App) runActive() bool {
	a.mu.RLock()
	managers := make([]*manager.Manager, 0, len(a.managers))
	for _, mgr := range a.managers {
		managers = append(managers, mgr)
	}
	a.mu.RUnlock()
	for _, mgr := range managers {
		if mgr.IsActive() {
			return true
		}
	}
	return false
}

func (a *App) manager() *manager.Manager {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeManagerLocked()
}

func (a *App) activeProjectRoot() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.projectRoot
}

func (a *App) activeManagerLocked() *manager.Manager {
	if a.projectRoot == "" {
		return nil
	}
	return a.managers[a.projectRoot]
}

func (a *App) captureWindowGeometry(mode string) {
	if a.ctx == nil || (mode != "compact" && mode != "manager") {
		return
	}
	mgr := a.manager()
	root := a.activeProjectRoot()
	if mgr == nil || root == "" {
		return
	}
	cfg := mgr.Config()
	if mode == "compact" {
		x, y := runtime.WindowGetPosition(a.ctx)
		cfg.UI.CompactX = x
		cfg.UI.CompactY = y
		cfg.UI.CompactPositionSet = true
	} else {
		width, height := runtime.WindowGetSize(a.ctx)
		if width >= 920 && height >= 560 {
			cfg.UI.ManagerWidth = width
			cfg.UI.ManagerHeight = height
		}
	}
	if err := config.Save(root, cfg); err == nil {
		_ = mgr.UpdateConfig(cfg)
	}
}
func (a *App) positionCompact(ui domain.UIConfig, height int) {
	if a.ctx == nil {
		return
	}
	ui = safeUI(ui)
	if height <= 0 {
		height = ui.CompactHeight
	}

	if x, y, ok := nativeCompactPosition(ui.CompactX, ui.CompactY, ui.CompactWidth, height, ui.CompactPositionSet); ok {
		runtime.WindowSetPosition(a.ctx, x, y)
		return
	}
	x := 16
	y := 16
	if ui.CompactPositionSet {
		runtime.WindowSetPosition(a.ctx, ui.CompactX, ui.CompactY)
		return
	}
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err == nil && len(screens) > 0 {
		chosen := screens[0]
		for _, screen := range screens {
			if screen.IsCurrent {
				chosen = screen
				break
			}
			if screen.IsPrimary {
				chosen = screen
			}
		}
		y = chosen.Height - height - 56
		if y < 16 {
			y = 16
		}
	}
	runtime.WindowSetPosition(a.ctx, x, y)
}

// positionCompactOnce supplies a predictable first location without fighting a
// user who has dragged the compact controller while lifecycle polling continues.
func (a *App) positionCompactOnce(ui domain.UIConfig, height int) {
	a.mu.Lock()
	if a.compactPositioned {
		a.mu.Unlock()
		return
	}
	a.compactPositioned = true
	a.mu.Unlock()
	a.positionCompact(ui, height)
}

func (a *App) startTrayOnce() {
	a.trayOnce.Do(func() {
		tray, err := startNativeTray(a)
		if err != nil {
			return
		}
		a.mu.Lock()
		a.tray = tray
		a.mu.Unlock()
	})
}

func safeUI(ui domain.UIConfig) domain.UIConfig {
	switch strings.ToLower(ui.WidgetSize) {
	case "small":
		ui.WidgetSize = "small"
		if ui.CompactWidth < 340 || ui.CompactWidth > 440 {
			ui.CompactWidth = 380
		}
		if ui.CompactHeight < 85 || ui.CompactHeight > 130 {
			ui.CompactHeight = 96
		}
		ui.CompactExpandedHeight = ui.CompactHeight + 84
	case "large":
		ui.WidgetSize = "large"
		if ui.CompactWidth < 540 || ui.CompactWidth > 680 {
			ui.CompactWidth = 580
		}
		if ui.CompactHeight < 110 || ui.CompactHeight > 170 {
			ui.CompactHeight = 118
		}
		ui.CompactExpandedHeight = ui.CompactHeight + 112
	default:
		ui.WidgetSize = "medium"
		if ui.CompactWidth < 440 || ui.CompactWidth > 540 {
			ui.CompactWidth = 480
		}
		if ui.CompactHeight < 95 || ui.CompactHeight > 150 {
			ui.CompactHeight = 106
		}
		ui.CompactExpandedHeight = ui.CompactHeight + 94
	}
	if ui.ManagerWidth < 1200 {
		ui.ManagerWidth = 1250
	}
	if ui.ManagerHeight < 790 {
		ui.ManagerHeight = 825
	}
	if ui.LogRetentionDays <= 0 {
		ui.LogRetentionDays = 7
	}
	return ui
}

func samePath(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftClean := filepath.Clean(strings.TrimSpace(left))
	rightClean := filepath.Clean(strings.TrimSpace(right))
	leftAbs, leftErr := filepath.Abs(leftClean)
	rightAbs, rightErr := filepath.Abs(rightClean)
	if leftErr == nil {
		leftClean = leftAbs
	}
	if rightErr == nil {
		rightClean = rightAbs
	}
	if leftClean == rightClean {
		return true
	}
	fi1, err1 := os.Stat(leftClean)
	fi2, err2 := os.Stat(rightClean)
	if err1 == nil && err2 == nil && os.SameFile(fi1, fi2) {
		return true
	}
	return strings.EqualFold(leftClean, rightClean)
}

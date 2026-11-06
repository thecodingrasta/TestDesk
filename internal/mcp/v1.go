package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/agents"
	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/prefs"
)

func v1Tools() []map[string]any {
	return []map[string]any{
		{"name": "testdesk_capabilities", "description": "Return the compact TestDesk agent workflow, supported selectors and safety model.", "inputSchema": objectSchema(map[string]any{})},
		{"name": "testdesk_projects", "description": "List a bounded page of known projects and identify the active project.", "inputSchema": objectSchema(map[string]any{"limit": numberSchema(), "offset": numberSchema()})},
		{"name": "testdesk_project_forget_missing", "description": "Explicitly remove one registered, missing project root from the registry. Never deletes project files.", "inputSchema": objectSchema(map[string]any{"projectRoot": stringSchema()})},
		{"name": "testdesk_project_activate", "description": "Change the default MCP project by project name or stable absolute projectRoot. Explicit project arguments on other tools do not change this default.", "inputSchema": objectSchema(map[string]any{"project": stringSchema(), "projectRoot": stringSchema()})},
		{"name": "testdesk_project_add", "description": "Add a project by absolute folder path. Creates testdesk.json only when absent; pass activate=false to retain the current project.", "inputSchema": objectSchema(map[string]any{"projectRoot": stringSchema(), "name": stringSchema(), "activate": boolSchema()})},
		{"name": "testdesk_project_configure", "description": "Detect built-in providers and persist them for an idle project. Existing providers are preserved unless replaceProviders=true.", "inputSchema": projectSchema(map[string]any{"replaceProviders": boolSchema()})},
		{"name": "testdesk_tests", "description": "Return a paged, filtered compact test catalog. Prefer IDs from this tool when running tests.", "inputSchema": projectSchema(map[string]any{"limit": numberSchema(), "offset": numberSchema(), "query": stringSchema(), "providerIds": stringArraySchema(), "providerKinds": stringArraySchema(), "statuses": stringArraySchema(), "layers": stringArraySchema(), "modules": stringArraySchema(), "tags": stringArraySchema()})},
		{"name": "testdesk_run_status", "description": "Return compact active and recent run status without catalog data.", "inputSchema": projectSchema(map[string]any{})},
		{"name": "testdesk_artifacts", "description": "Return artifact references for a run. Artifact contents are never inlined.", "inputSchema": projectSchema(map[string]any{"runId": stringSchema()})},
		{"name": "testdesk_settings", "description": "Return compact current project settings and provider configuration.", "inputSchema": projectSchema(map[string]any{})},
		{"name": "testdesk_history", "description": "Query past test runs with filtering by status, limit and offset, returning pass/fail metrics, duration and timestamps.", "inputSchema": projectSchema(map[string]any{"limit": numberSchema(), "offset": numberSchema(), "status": enumSchema([]string{"passed", "failed", "cancelled", "running"})})},
		{"name": "testdesk_test_details", "description": "Inspect a test by testId or unique name, including its definition, latest duration and matching recent failure. Use testId when names are ambiguous.", "inputSchema": projectSchema(map[string]any{"testId": stringSchema(), "name": stringSchema()})},
		{"name": "testdesk_rerun_failed", "description": "Automatically re-run all failed or skipped tests from the latest (or specified) test run.", "inputSchema": projectSchema(map[string]any{"runId": stringSchema()})},
		{"name": "testdesk_skills_install", "description": "Install or update TestDesk skills and prompt instructions (AGENTS.md, CLAUDE.md, GEMINI.md, .agents/skills, .claude/skills, Copilot instructions, Cursor rules and Gemini skills) for the active or specified project.", "inputSchema": projectSchema(map[string]any{})},
	}
}

func (s *Server) projects() (any, error) {
	return s.pagedProjects(nil)
}

func (s *Server) pagedProjects(args map[string]any) (any, error) {
	value, err := prefs.Load()
	if err != nil {
		return nil, err
	}
	active := ""
	if mgr := s.activeManager(); mgr != nil {
		active = mgr.ProjectRoot()
	} else if value.LastProjectRoot != "" {
		active = value.LastProjectRoot
	}
	projects := make([]map[string]any, 0, len(value.Projects))
	for _, project := range value.Projects {
		projects = append(projects, map[string]any{
			"root": project.Root, "name": project.Name, "active": active != "" && samePath(project.Root, active),
			"configured": config.Exists(project.Root), "lastOpened": project.LastOpenedAt,
		})
	}
	for _, mgr := range s.loadedManagers() {
		if !projectKnown(projects, mgr.ProjectRoot()) {
			projects = append(projects, map[string]any{"root": mgr.ProjectRoot(), "name": mgr.Config().Name, "active": active != "" && samePath(mgr.ProjectRoot(), active), "configured": true})
		}
	}
	limit := intArg(args, "limit", 50)
	if limit <= 0 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}
	offset := intArg(args, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	total := len(projects)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return map[string]any{"activeProjectRoot": active, "projects": projects[offset:end], "total": total, "offset": offset, "limit": limit, "hasMore": end < total}, nil
}

func (s *Server) forgetMissingProject(args map[string]any) (any, error) {
	requestedRoot := stringArg(args, "projectRoot")
	if !filepath.IsAbs(requestedRoot) {
		return nil, errors.New("an absolute projectRoot is required")
	}
	root, err := absoluteProjectRoot(requestedRoot)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(root); err == nil {
		return nil, errors.New("project root still exists; no registry entry was removed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot establish that project root is missing: %w", err)
	}
	for _, mgr := range s.loadedManagers() {
		if samePath(mgr.ProjectRoot(), root) {
			return nil, errors.New("project is loaded in this session; no registry entry was removed")
		}
	}
	value, err := prefs.Load()
	if err != nil {
		return nil, err
	}
	known := false
	for _, project := range value.Projects {
		if samePath(project.Root, root) {
			known = true
			break
		}
	}
	if !known {
		return nil, errors.New("project root is not registered")
	}
	if _, err := prefs.ForgetProject(root); err != nil {
		return nil, err
	}
	return map[string]any{"forgottenProjectRoot": root}, nil
}

func (s *Server) activateProject(args map[string]any) (any, error) {
	root, err := s.resolveProjectSelector(projectSelector(args))
	if err != nil {
		return nil, err
	}
	if !config.Exists(root) {
		return nil, fmt.Errorf("%s does not contain %s", root, config.FileName)
	}
	mgr, err := s.managerForRoot(root)
	if err != nil {
		return nil, err
	}
	if _, err := prefs.UpsertProject(root, mgr.Config().Name); err != nil {
		return nil, err
	}
	s.setActiveManager(mgr)
	return map[string]any{"activeProjectRoot": root, "project": mgr.Config().Name}, nil
}

func (s *Server) addProject(args map[string]any) (any, error) {
	root, err := absoluteProjectRoot(stringArg(args, "projectRoot"))
	if err != nil {
		return nil, err
	}
	if stat, statErr := os.Stat(root); statErr != nil || !stat.IsDir() {
		if statErr != nil {
			return nil, statErr
		}
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	cfg := config.Detect(root)
	if config.Exists(root) {
		cfg, err = config.Load(root)
		if err != nil {
			return nil, err
		}
	} else if err := config.Save(root, cfg); err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(stringArg(args, "name")); name != "" {
		cfg.Name = name
		if err := config.Save(root, cfg); err != nil {
			return nil, err
		}
	}
	if _, err := prefs.UpsertProject(root, cfg.Name); err != nil {
		return nil, err
	}
	if boolArg(args, "activate", true) {
		mgr, err := s.managerForRoot(root)
		if err != nil {
			return nil, err
		}
		s.setActiveManager(mgr)
	}
	active := false
	if activeMgr := s.activeManager(); activeMgr != nil {
		active = samePath(activeMgr.ProjectRoot(), root)
	}
	return map[string]any{"root": root, "name": cfg.Name, "active": active, "providers": len(cfg.Providers)}, nil
}

func (s *Server) configureProject(args map[string]any) (any, error) {
	root, err := s.resolveProjectSelector(projectSelector(args))
	if err != nil {
		return nil, err
	}
	mgr, err := s.managerForRoot(root)
	if err != nil {
		return nil, err
	}
	snapshot, err := mgr.Snapshot(domain.Filter{Limit: 0})
	if err != nil {
		return nil, err
	}
	if snapshot.Running != nil || snapshot.QueuedCount > 0 {
		return nil, errors.New("cannot reconfigure providers while this project has an active or queued run")
	}
	detected := config.Detect(root).Providers
	if len(detected) == 0 {
		return nil, errors.New("no built-in test providers were detected; add a custom provider to testdesk.json")
	}
	cfg := mgr.Config()
	previous := len(cfg.Providers)
	if boolArg(args, "replaceProviders", false) {
		cfg.Providers = detected
	} else {
		cfg.Providers = config.MergeDetectedProviders(cfg.Providers, detected)
	}
	if err := config.Save(root, cfg); err != nil {
		return nil, err
	}
	if err := mgr.UpdateConfig(cfg); err != nil {
		return nil, err
	}
	return map[string]any{"project": cfg.Name, "projectRoot": root, "providers": len(cfg.Providers), "providersAdded": len(cfg.Providers) - previous, "replaced": boolArg(args, "replaceProviders", false)}, nil
}

func (s *Server) artifacts(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	runID := filepath.Base(filepath.Clean(strings.TrimSpace(stringArg(args, "runId"))))
	if runID == "" || runID == "." || runID == "/" || runID == "\\" {
		return nil, errors.New("runId is required")
	}
	diagnostics, err := mgr.Diagnostics(runID, 0)
	if err != nil {
		return nil, err
	}
	return map[string]any{"runId": runID, "diagnostics": diagnostics.Path, "runDirectory": filepath.Join(mgr.StoreRoot(), "runs", runID)}, nil
}

func (s *Server) settings(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	cfg := mgr.Config()
	return map[string]any{
		"projectName": cfg.Name,
		"providers":   cfg.Providers,
		"ui":          cfg.UI,
	}, nil
}

func (s *Server) history(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	limit := intArg(args, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := intArg(args, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	statusFilter := strings.TrimSpace(stringArg(args, "status"))
	fetchCount := limit + offset + 100
	if statusFilter != "" {
		fetchCount = 0
	}
	runs, err := mgr.Store().RecentRuns(fetchCount)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.Run, 0, len(runs))
	for _, r := range runs {
		if statusFilter != "" && !strings.EqualFold(string(r.Status), statusFilter) {
			continue
		}
		filtered = append(filtered, r)
	}
	totalMatching := len(filtered)
	if offset > len(filtered) {
		filtered = nil
	} else {
		filtered = filtered[offset:]
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return map[string]any{
		"project": mgr.Config().Name,
		"total":   totalMatching,
		"offset":  offset,
		"limit":   limit,
		"runs":    compactRuns(filtered),
	}, nil
}

func (s *Server) testDetails(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	testID := strings.TrimSpace(stringArg(args, "testId"))
	if testID == "" {
		testID = strings.TrimSpace(stringArg(args, "id"))
	}
	testName := strings.TrimSpace(stringArg(args, "testName"))
	if testName == "" {
		testName = strings.TrimSpace(stringArg(args, "name"))
	}
	if testID == "" && testName == "" {
		return nil, errors.New("either 'testId' or 'testName' is required")
	}

	snap, err := mgr.Snapshot(domain.Filter{Limit: 0})
	if err != nil {
		return nil, err
	}

	var found domain.TestCase
	var foundOk bool
	for _, t := range snap.Tests {
		if testID != "" {
			if t.ID != testID {
				continue
			}
			if testName != "" && !strings.EqualFold(t.Name, testName) && !strings.EqualFold(t.FullName, testName) {
				return nil, fmt.Errorf("test id and name refer to different tests")
			}
			found, foundOk = t, true
			break
		}
		if testName != "" && (strings.EqualFold(t.Name, testName) || strings.EqualFold(t.FullName, testName)) {
			if foundOk {
				return nil, fmt.Errorf("test name %q is ambiguous; provide a testId", testName)
			}
			found, foundOk = t, true
		}
	}
	if !foundOk {
		return nil, fmt.Errorf("test not found matching id=%q name=%q", testID, testName)
	}

	out := map[string]any{
		"id":         found.ID,
		"provider":   found.ProviderID,
		"kind":       found.ProviderKind,
		"layer":      found.Layer,
		"module":     found.Module,
		"name":       found.Name,
		"fullName":   found.FullName,
		"selector":   found.Selector,
		"file":       found.File,
		"line":       found.Line,
		"status":     found.LastStatus,
		"durationMs": found.LastDurationMillis,
		"tags":       found.Tags,
	}
	if found.JestProject != "" {
		out["jestProject"] = found.JestProject
	}
	uniqueLegacyName := true
	for _, candidate := range snap.Tests {
		if candidate.ID != found.ID && candidate.ProviderID == found.ProviderID && candidate.Name == found.Name {
			uniqueLegacyName = false
			break
		}
	}

	// Look up last failure details if available in recent runs
	for _, r := range snap.RecentRuns {
		for _, item := range r.Items {
			matches := item.TestID == found.ID || (item.TestID == "" && uniqueLegacyName && item.ProviderID == found.ProviderID && item.TestName == found.Name)
			if matches && item.Status == domain.StatusFailed && item.Error != "" {
				out["lastFailure"] = map[string]any{
					"runId":    r.ID,
					"error":    item.Error,
					"time":     r.StartedAt.Format(time.RFC3339),
					"provider": item.ProviderID,
				}
				break
			}
		}
		if out["lastFailure"] != nil {
			break
		}
	}

	return out, nil
}

func (s *Server) rerunFailed(ctx context.Context, args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	runID := strings.TrimSpace(stringArg(args, "runId"))
	var targetRun *domain.Run
	if runID != "" {
		if runID == "." || runID == ".." || runID != filepath.Base(runID) || runID != filepath.Clean(runID) {
			return nil, fmt.Errorf("invalid run ID %q", runID)
		}
		diag, err := mgr.Diagnostics(runID, 0)
		if err != nil {
			return nil, fmt.Errorf("load run %q: %w", runID, err)
		}
		if diag.Run == nil {
			return nil, fmt.Errorf("run %q was not found", runID)
		}
		targetRun = diag.Run
	}
	if targetRun == nil {
		snap, err := mgr.Snapshot(domain.Filter{Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(snap.RecentRuns) > 0 {
			targetRun = &snap.RecentRuns[0]
		}
	}
	if targetRun == nil {
		return nil, errors.New("no previous run found to re-run failed tests from")
	}
	var failedTestIDs []string
	var failedProviderIDs []string
	for _, item := range targetRun.Items {
		if item.Status == domain.StatusFailed || item.Status == domain.StatusCancelled || item.Status == domain.StatusSkipped || item.Status == domain.StatusTimedOut {
			if item.TestID != "" {
				failedTestIDs = append(failedTestIDs, item.TestID)
			} else if item.ProviderID != "" {
				failedProviderIDs = append(failedProviderIDs, item.ProviderID)
			}
		}
	}
	if len(failedTestIDs) == 0 && len(failedProviderIDs) == 0 {
		return map[string]any{
			"message":           fmt.Sprintf("no failed or skipped tests found in run %s (status: %s)", targetRun.ID, targetRun.Status),
			"retestedFromRunId": targetRun.ID,
			"total":             0,
		}, nil
	}
	req := manager.RunRequest{TestIDs: failedTestIDs, ProviderWideIDs: failedProviderIDs}
	run, err := mgr.Run(ctx, req)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"retestedFromRunId": targetRun.ID,
		"runId":             run.ID,
		"status":            run.Status,
		"total":             run.Total,
		"items":             compactItems(run.Items),
	}, nil
}

func absoluteProjectRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("projectRoot is required")
	}
	return filepath.Abs(value)
}

func (s *Server) managerForArgs(args map[string]any) (*manager.Manager, error) {
	selector := strings.TrimSpace(stringArg(args, "project"))
	if selector == "" {
		selector = strings.TrimSpace(stringArg(args, "projectRoot"))
	}
	if selector != "" {
		root, err := s.resolveProjectSelector(selector)
		if err != nil {
			return nil, err
		}
		return s.managerForRoot(root)
	}
	if active := s.activeManager(); active != nil {
		return active, nil
	}
	if userPrefs, err := prefs.Load(); err == nil && len(userPrefs.Projects) > 0 {
		targetRoot := userPrefs.LastProjectRoot
		if targetRoot == "" && len(userPrefs.Projects) > 0 {
			targetRoot = userPrefs.Projects[0].Root
		}
		if targetRoot != "" && config.Exists(targetRoot) {
			if mgr, err := s.managerForRoot(targetRoot); err == nil {
				s.setActiveManager(mgr)
				return mgr, nil
			}
		}
	}
	if config.Exists(".") {
		if mgr, err := s.managerForRoot("."); err == nil {
			s.setActiveManager(mgr)
			return mgr, nil
		}
	}
	return nil, errors.New("no active project configured in TestDesk; pass 'project' (name or path) on the tool call or use testdesk_projects / testdesk_project_activate")
}

func (s *Server) activeManager() *manager.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.managers[projectKey(s.activeRoot)]
}

func (s *Server) setActiveManager(mgr *manager.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managers[projectKey(mgr.ProjectRoot())] = mgr
	s.activeRoot = mgr.ProjectRoot()
}

func (s *Server) managerForRoot(root string) (*manager.Manager, error) {
	key := projectKey(root)
	s.mu.Lock()
	mgr := s.managers[key]
	if mgr != nil {
		s.mu.Unlock()
		return mgr, nil
	}
	if loading := s.loading[key]; loading != nil {
		s.mu.Unlock()
		<-loading.done
		return loading.mgr, loading.err
	}
	if s.loading == nil {
		s.loading = make(map[string]*managerLoad)
	}
	loading := &managerLoad{done: make(chan struct{})}
	s.loading[key] = loading
	s.mu.Unlock()

	var loaded *manager.Manager
	var err error
	if s.resolveManager != nil {
		loaded, err = s.resolveManager(root)
	} else {
		loaded, err = manager.NewFromProject(root)
	}
	s.mu.Lock()
	if err == nil {
		if mgr = s.managers[key]; mgr == nil {
			mgr = loaded
			s.managers[key] = mgr
		}
	}
	loading.mgr, loading.err = mgr, err
	delete(s.loading, key)
	close(loading.done)
	s.mu.Unlock()
	return mgr, err
}

func (s *Server) resolveProjectSelector(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("project is required")
	}
	if filepath.IsAbs(selector) {
		root, err := absoluteProjectRoot(selector)
		if err != nil {
			return "", err
		}
		if !config.Exists(root) {
			return "", fmt.Errorf("%s does not contain %s", root, config.FileName)
		}
		return root, nil
	}
	if absPath, err := filepath.Abs(selector); err == nil && config.Exists(absPath) {
		return absPath, nil
	}

	candidates := []string{}
	for _, mgr := range s.loadedManagers() {
		if strings.EqualFold(mgr.Config().Name, selector) && config.Exists(mgr.ProjectRoot()) {
			candidates = append(candidates, mgr.ProjectRoot())
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	value, err := prefs.Load()
	if err == nil {
		for _, project := range value.Projects {
			if strings.EqualFold(project.Name, selector) && config.Exists(project.Root) && !projectKnownRoots(candidates, project.Root) {
				candidates = append(candidates, project.Root)
			}
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("unknown project %q; call testdesk_projects and use its name or root", selector)
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("project name %q is ambiguous; use its absolute project root", selector)
	}
}

func projectSelector(args map[string]any) string {
	if root := strings.TrimSpace(stringArg(args, "projectRoot")); root != "" {
		return root
	}
	return stringArg(args, "project")
}

func (s *Server) loadedManagers() []*manager.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	managers := make([]*manager.Manager, 0, len(s.managers))
	for _, mgr := range s.managers {
		managers = append(managers, mgr)
	}
	sort.Slice(managers, func(i int, j int) bool {
		return projectKey(managers[i].ProjectRoot()) < projectKey(managers[j].ProjectRoot())
	})
	return managers
}

func projectKnown(projects []map[string]any, root string) bool {
	for _, project := range projects {
		value, ok := project["root"].(string)
		if ok && samePath(value, root) {
			return true
		}
	}
	return false
}

func projectKnownRoots(roots []string, root string) bool {
	for _, item := range roots {
		if samePath(item, root) {
			return true
		}
	}
	return false
}

func samePath(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftClean := filepath.Clean(strings.TrimSpace(left))
	rightClean := filepath.Clean(strings.TrimSpace(right))
	leftRoot, leftErr := filepath.Abs(leftClean)
	rightRoot, rightErr := filepath.Abs(rightClean)
	if leftErr == nil {
		leftClean = leftRoot
	}
	if rightErr == nil {
		rightClean = rightRoot
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

func (s *Server) installSkills(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	res, err := agents.InstallSkills(mgr.ProjectRoot())
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"projectRoot":    res.ProjectRoot,
		"installedFiles": res.InstalledFiles,
		"updatedFiles":   res.UpdatedFiles,
		"message":        res.Message,
	}, nil
}

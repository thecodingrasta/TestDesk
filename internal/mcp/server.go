package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

const serverVersion = "1.0.0"

type Server struct {
	mu             sync.RWMutex
	activeRoot     string
	managers       map[string]*manager.Manager
	loading        map[string]*managerLoad
	resolveManager func(string) (*manager.Manager, error)
	sharedOwner    bool
}

// NewWithManagerResolver lets a desktop owner supply the project managers used
// by an MCP session. Session selection remains independent of the owner.
func NewWithManagerResolver(resolve func(string) (*manager.Manager, error)) *Server {
	s := New(nil)
	s.resolveManager = resolve
	s.sharedOwner = true
	return s
}

// ServeConnection cancels this client's work when its transport disconnects.
// It does not drain other clients' runs in the shared owner.
func (s *Server) ServeConnection(input io.Reader, output io.Writer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return s.Serve(ctx, &disconnectReader{Reader: input, cancel: cancel}, output)
}

type disconnectReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r *disconnectReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && n == 0 {
		r.cancel()
	}
	return n, err
}

type managerLoad struct {
	done chan struct{}
	mgr  *manager.Manager
	err  error
}

func New(mgr *manager.Manager) *Server {
	if mgr == nil {
		return &Server{managers: map[string]*manager.Manager{}}
	}
	root := mgr.ProjectRoot()
	return &Server{activeRoot: root, managers: map[string]*manager.Manager{projectKey(root): mgr}}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type longRequestResult struct {
	key      string
	response response
}

type longRequestState struct {
	cancel    context.CancelFunc
	cancelled bool
}

type progressNotification struct {
	key    string
	state  *longRequestState
	params map[string]any
}

type progressEmitterKey struct{}

func requestIDKey(raw json.RawMessage) string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return "s:" + value
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil && number.String() != "" && !strings.ContainsAny(number.String(), ".eE") {
		return "n:" + number.String()
	}
	return ""
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	scanner := bufio.NewScanner(input)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	encoder := json.NewEncoder(output)
	lines := make(chan string)
	scanDone := make(chan error, 1)
	const maxLongRequests = 16
	completed := make(chan longRequestResult, maxLongRequests)
	progress := make(chan progressNotification, 32)
	active := make(map[string]*longRequestState)
	pending := 0
	var inputErr error
	inputClosed := false
	go func() {
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				scanDone <- nil
				return
			}
		}
		scanDone <- scanner.Err()
	}()
	for {
		if inputClosed && pending == 0 {
			if !s.sharedOwner {
				s.Drain(5 * time.Second)
			}
			return inputErr
		}
		select {
		case <-ctx.Done():
			if !s.sharedOwner {
				s.Drain(2 * time.Second)
			}
			return nil
		case err := <-scanDone:
			inputClosed = true
			inputErr = err
			scanDone = nil
			lines = nil
		case result := <-completed:
			pending--
			state := active[result.key]
			delete(active, result.key)
			if state == nil || state.cancelled {
				continue
			}
			if err := encoder.Encode(result.response); err != nil {
				return err
			}
		case event := <-progress:
			if state := active[event.key]; state != nil && state == event.state && !state.cancelled {
				if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": event.params}); err != nil {
					return err
				}
			}
		case scanned := <-lines:
			line := strings.TrimSpace(scanned)
			if line == "" {
				continue
			}
			var req request
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				if err := encoder.Encode(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}}); err != nil {
					return err
				}
				continue
			}
			if len(req.ID) == 0 {
				if req.Method == "notifications/cancelled" {
					var params struct {
						RequestID json.RawMessage `json:"requestId"`
					}
					if json.Unmarshal(req.Params, &params) == nil {
						if state := active[requestIDKey(params.RequestID)]; state != nil {
							state.cancelled = true
							state.cancel()
						}
					}
				}
				continue
			}
			if isLongRequest(req) {
				key := requestIDKey(req.ID)
				if key == "" || active[key] != nil {
					if err := encoder.Encode(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32600, Message: "invalid or duplicate request ID"}}); err != nil {
						return err
					}
					continue
				}
				if pending >= maxLongRequests {
					if err := encoder.Encode(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32000, Message: "too many concurrent long-running requests"}}); err != nil {
						return err
					}
					continue
				}
				requestCtx, requestCancel := context.WithCancel(s.bindLongRequestProject(ctx, req))
				state := &longRequestState{cancel: requestCancel}
				active[key] = state
				var params struct {
					Meta struct {
						Token json.RawMessage `json:"progressToken"`
					} `json:"_meta"`
				}
				if json.Unmarshal(req.Params, &params) == nil && len(params.Meta.Token) <= 256 && requestIDKey(params.Meta.Token) != "" {
					token := append(json.RawMessage(nil), params.Meta.Token...)
					emit := func(value int, total int, message string) {
						if len(message) > 512 {
							message = message[:512]
						}
						select {
						case progress <- progressNotification{key: key, state: state, params: map[string]any{"progressToken": token, "progress": value, "total": total, "message": message}}:
						default:
						}
					}
					requestCtx = context.WithValue(requestCtx, progressEmitterKey{}, emit)
				}
				pending++
				go func(req request, key string, requestCtx context.Context, requestCancel context.CancelFunc) {
					defer requestCancel()
					res := s.handle(requestCtx, req)
					select {
					case completed <- longRequestResult{key: key, response: res}:
					case <-ctx.Done():
					}
				}(req, key, requestCtx, requestCancel)
				continue
			}
			if err := encoder.Encode(s.handle(ctx, req)); err != nil {
				return err
			}
		}
	}
}

func isLongRequest(req request) bool {
	if req.Method != "tools/call" {
		return false
	}
	var params struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(req.Params, &params) != nil {
		return false
	}
	return params.Name == "testdesk_wait" || params.Name == "testdesk_discover"
}

// Capture the session default before dispatch so a later project switch cannot
// redirect a request that has already arrived.
type requestManagerKey struct{}
type requestManagerBinding struct {
	manager *manager.Manager
	err     error
}

func (s *Server) bindLongRequestProject(ctx context.Context, req request) context.Context {
	var params map[string]json.RawMessage
	if json.Unmarshal(req.Params, &params) != nil {
		return ctx
	}
	args := map[string]any{}
	if raw := params["arguments"]; len(raw) > 0 && json.Unmarshal(raw, &args) != nil {
		return ctx
	}
	if args == nil {
		args = make(map[string]any)
	}
	if strings.TrimSpace(stringArg(args, "project")) != "" || strings.TrimSpace(stringArg(args, "projectRoot")) != "" {
		return ctx
	}
	mgr, err := s.managerForArgs(args)
	return context.WithValue(ctx, requestManagerKey{}, requestManagerBinding{manager: mgr, err: err})
}

func (s *Server) managerForLongRequest(ctx context.Context, args map[string]any) (*manager.Manager, error) {
	if binding, ok := ctx.Value(requestManagerKey{}).(requestManagerBinding); ok {
		return binding.manager, binding.err
	}
	return s.managerForArgs(args)
}

func (s *Server) Drain(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, mgr := range s.loadedManagers() {
		_ = mgr.Drain(ctx)
	}
}

func (s *Server) handle(ctx context.Context, req request) response {
	res := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "testdesk/session":
		var params struct {
			ProjectRoot string `json:"projectRoot"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			res.Error = &rpcError{Code: -32602, Message: err.Error()}
		} else if !filepath.IsAbs(params.ProjectRoot) || !config.Exists(params.ProjectRoot) {
			res.Error = &rpcError{Code: -32602, Message: "session project must be an absolute configured project root"}
		} else if mgr, err := s.managerForRoot(params.ProjectRoot); err != nil {
			res.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			s.setActiveManager(mgr)
			res.Result = map[string]any{"projectRoot": mgr.ProjectRoot()}
		}
	case "initialize":
		res.Result = map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo":      map[string]any{"name": "testdesk", "version": serverVersion},
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"listChanged": false},
				"prompts":   map[string]any{"listChanged": false},
			},
		}
	case "tools/list":
		res.Result = map[string]any{"tools": tools()}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			res.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			res.Result = result
		}
	case "resources/list":
		res.Result = map[string]any{"resources": resources()}
	case "resources/read":
		result, err := s.readResource(ctx, req.Params)
		if err != nil {
			res.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			res.Result = result
		}
	case "prompts/list":
		res.Result = map[string]any{"prompts": prompts()}
	case "prompts/get":
		result, err := s.getPrompt(req.Params)
		if err != nil {
			res.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			res.Result = result
		}
	case "ping":
		res.Result = map[string]any{}
	default:
		res.Error = &rpcError{Code: -32601, Message: "method not found"}
	}
	return res
}

func tools() []map[string]any {
	base := []map[string]any{
		{
			"name":        "testdesk_guide",
			"description": "Return the concise agent workflow, safety model, filters and tool-selection guidance for efficient TestDesk usage.",
			"inputSchema": objectSchema(map[string]any{}),
		},
		{
			"name":        "testdesk_snapshot",
			"description": "Return compact lifecycle state. Default omits tests to minimise context. Use includeTests only with filters/limit.",
			"inputSchema": projectSchema(map[string]any{"includeTests": boolSchema(), "limit": numberSchema(), "offset": numberSchema(), "query": stringSchema(), "providerIds": stringArraySchema(), "providerKinds": stringArraySchema(), "statuses": stringArraySchema(), "layers": stringArraySchema(), "modules": stringArraySchema(), "tags": stringArraySchema()}),
		},
		{
			"name":        "testdesk_catalog",
			"description": "Return a paged compact test catalog. Use this instead of snapshot when the agent only needs tests.",
			"inputSchema": projectSchema(map[string]any{"limit": numberSchema(), "offset": numberSchema(), "query": stringSchema(), "providerIds": stringArraySchema(), "providerKinds": stringArraySchema(), "statuses": stringArraySchema(), "layers": stringArraySchema(), "modules": stringArraySchema(), "tags": stringArraySchema()}),
		},
		{
			"name":        "testdesk_discover",
			"description": "Refresh the project test catalog by invoking enabled provider discovery commands. Use before catalog if tests look stale or missing.",
			"inputSchema": projectSchema(map[string]any{}),
		},
		{
			"name":        "testdesk_discovery_status",
			"description": "Inspect the current or latest provider discovery, including observed output timing; silence alone does not establish a stall.",
			"inputSchema": projectSchema(map[string]any{}),
		},
		{
			"name":        "testdesk_plan",
			"description": "Dry-run a request and return the executable items that would be queued, without running tests.",
			"inputSchema": projectSchema(runSchema()),
		},
		{
			"name":        "testdesk_run",
			"description": "Queue a managed test run by explicit test IDs, provider IDs, or compact filters. Prefer testdesk_plan first for broad filters.",
			"inputSchema": projectSchema(runSchema()),
		},
		{
			"name":        "testdesk_control",
			"description": "Control the active lifecycle: pause queue, resume queue, stop after current process cancellation, hard kill current process, or skip current item.",
			"inputSchema": projectSchema(map[string]any{"action": enumSchema([]string{"pause", "resume", "stop", "kill", "skip"})}),
		},
		{
			"name":        "testdesk_wait",
			"description": "Wait until TestDesk is idle or timeoutMs elapses. Returns the latest compact snapshot.",
			"inputSchema": projectSchema(map[string]any{"timeoutMs": numberSchema()}),
		},
		{
			"name":        "testdesk_diagnostics",
			"description": "Return bounded diagnostics for a run. Keep maxBytes small until deeper investigation is needed.",
			"inputSchema": projectSchema(map[string]any{"runId": stringSchema(), "maxBytes": numberSchema(), "includeRun": boolSchema(), "includeLog": boolSchema()}),
		},
	}
	return append(base, v1Tools()...)
}

func runSchema() map[string]any {
	return map[string]any{"testIds": stringArraySchema(), "providerIds": stringArraySchema(), "providerKinds": stringArraySchema(), "query": stringSchema(), "layers": stringArraySchema(), "modules": stringArraySchema(), "tags": stringArraySchema(), "statuses": stringArraySchema(), "limit": numberSchema()}
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	args := map[string]any{}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return nil, err
		}
	}
	var payload any
	var err error
	switch params.Name {
	case "testdesk_guide", "testdesk_capabilities":
		payload = guide()
	case "testdesk_snapshot":
		payload, err = s.snapshot(args)
	case "testdesk_catalog", "testdesk_tests":
		payload, err = s.catalog(args)
	case "testdesk_discover":
		payload, err = s.discoverProject(ctx, args)
	case "testdesk_discovery_status":
		var mgr *manager.Manager
		mgr, err = s.managerForArgs(args)
		if err == nil {
			payload = mgr.DiscoveryStatus()
		}
	case "testdesk_plan":
		payload, err = s.plan(args)
	case "testdesk_run":
		payload, err = s.run(ctx, args)
	case "testdesk_control":
		payload, err = s.control(args)
	case "testdesk_wait":
		payload, err = s.wait(ctx, args)
	case "testdesk_diagnostics":
		payload, err = s.diagnostics(args)
	case "testdesk_run_status":
		payload, err = s.snapshot(args)
	case "testdesk_projects":
		payload, err = s.pagedProjects(args)
	case "testdesk_project_forget_missing":
		payload, err = s.forgetMissingProject(args)
	case "testdesk_project_activate":
		payload, err = s.activateProject(args)
	case "testdesk_project_add":
		payload, err = s.addProject(args)
	case "testdesk_project_configure":
		payload, err = s.configureProject(args)
	case "testdesk_artifacts":
		payload, err = s.artifacts(args)
	case "testdesk_settings":
		payload, err = s.settings(args)
	case "testdesk_history":
		payload, err = s.history(args)
	case "testdesk_test_details":
		payload, err = s.testDetails(args)
	case "testdesk_rerun_failed":
		payload, err = s.rerunFailed(ctx, args)
	case "testdesk_skills_install":
		payload, err = s.installSkills(args)
	default:
		err = fmt.Errorf("unknown tool %q", params.Name)
	}
	if err != nil {
		return nil, err
	}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, marshalErr
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(data)}}}, nil
}

func resources() []map[string]any {
	return []map[string]any{
		{
			"uri":         "testdesk://guide",
			"name":        "TestDesk Agent Workflow Guide",
			"description": "Complete agent workflow guide, safety rules and execution principles.",
			"mimeType":    "text/markdown",
		},
		{
			"uri":         "testdesk://capabilities",
			"name":        "TestDesk System Capabilities",
			"description": "Supported selectors, safety boundaries and multi-project routing model.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "testdesk://projects",
			"name":        "TestDesk Configured Projects",
			"description": "Known projects, active session default and project roots.",
			"mimeType":    "application/json",
		},
	}
}

func (s *Server) readResource(ctx context.Context, params json.RawMessage) (any, error) {
	var input struct {
		URI string `json:"uri"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &input)
	}
	switch input.URI {
	case "testdesk://guide":
		guideMarkdown := `# TestDesk — Code Testing Intelligence & Orchestrator

## Core Principles
1. Always Plan Before Broad Runs: Use testdesk_plan before queueing broad filters or entire suites.
2. Prefer Explicit IDs: Discover stable IDs with testdesk_tests and run them with testdesk_run(testIds: [...]).
3. Multi-Project Isolation: Each project maintains isolated queues, logs and catalogs. Pass project: "nameOrPath" to target a specific project without switching the session default.
4. Fast Failure Iteration: After applying code edits to fix test failures, immediately run testdesk_rerun_failed() to re-test only the failed subset.
5. Bounded Diagnostics: Never read raw terminal dumps; use testdesk_diagnostics(runId: "...", maxBytes: 4096) or testdesk_test_details(testId: "...").

## Standard Workflow
1. testdesk_snapshot(includeTests: false) — verify idle state.
2. testdesk_tests(query: "<module>") — find test IDs.
3. testdesk_run(testIds: ["..."]) — execute targeted verification.
4. testdesk_wait(timeoutMs: 30000) — await completion.
5. If failing: testdesk_diagnostics(runId: "...") -> fix -> testdesk_rerun_failed().
`
		return map[string]any{
			"contents": []map[string]any{
				{
					"uri":      "testdesk://guide",
					"mimeType": "text/markdown",
					"text":     guideMarkdown,
				},
			},
		}, nil
	case "testdesk://capabilities":
		caps := guide()
		data, _ := json.MarshalIndent(caps, "", "  ")
		return map[string]any{
			"contents": []map[string]any{
				{
					"uri":      "testdesk://capabilities",
					"mimeType": "application/json",
					"text":     string(data),
				},
			},
		}, nil
	case "testdesk://projects":
		projects, _ := s.projects()
		data, _ := json.MarshalIndent(projects, "", "  ")
		return map[string]any{
			"contents": []map[string]any{
				{
					"uri":      "testdesk://projects",
					"mimeType": "application/json",
					"text":     string(data),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("resource %q not found", input.URI)
	}
}

func prompts() []map[string]any {
	return []map[string]any{
		{
			"name":        "testdesk-verify-changes",
			"description": "Prompt instructions to verify recent code changes by discovering, running and waiting for targeted tests.",
			"arguments": []map[string]any{
				{"name": "module", "description": "Name or query for the modified code module or component", "required": false},
			},
		},
		{
			"name":        "testdesk-debug-failure",
			"description": "Prompt instructions to inspect failing tests, analyze diagnostics and verify fixes with testdesk_rerun_failed.",
			"arguments": []map[string]any{
				{"name": "runId", "description": "Optional specific run ID to inspect", "required": false},
			},
		},
		{
			"name":        "testdesk-plan-suite",
			"description": "Prompt instructions to dry-run broad test suites or layers before execution.",
			"arguments": []map[string]any{
				{"name": "provider", "description": "Optional provider ID or kind", "required": false},
			},
		},
	}
}

func (s *Server) getPrompt(params json.RawMessage) (any, error) {
	var input struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &input)
	}
	switch input.Name {
	case "testdesk-verify-changes":
		module := input.Arguments["module"]
		promptText := fmt.Sprintf("Verify code changes with TestDesk:\n1. Call testdesk_snapshot to confirm TestDesk is idle.\n2. Call testdesk_tests(query: %q) to locate affected test IDs.\n3. Call testdesk_run(testIds: [...]) to execute targeted verification.\n4. Call testdesk_wait(timeoutMs: 30000) to await completion.\n5. If tests fail, inspect diagnostics and fix.", module)
		return map[string]any{
			"description": "Verify code changes with targeted tests",
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": promptText,
					},
				},
			},
		}, nil
	case "testdesk-debug-failure":
		runId := input.Arguments["runId"]
		promptText := fmt.Sprintf("Debug test failure with TestDesk:\n1. Call testdesk_diagnostics(runId: %q, maxBytes: 4096) to inspect failure stack trace.\n2. Call testdesk_test_details(testId: \"...\") for specific test case history.\n3. Apply source code fix in editor.\n4. Call testdesk_rerun_failed() to immediately verify only the failed test subset.", runId)
		return map[string]any{
			"description": "Debug test failures with bounded diagnostics",
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": promptText,
					},
				},
			},
		}, nil
	case "testdesk-plan-suite":
		provider := input.Arguments["provider"]
		promptText := fmt.Sprintf("Dry-run and plan test execution:\n1. Call testdesk_plan(providerIds: [%q]) to inspect queued items without executing.\n2. Review matched tests and duration estimates.\n3. Execute via testdesk_run if plan looks optimal.", provider)
		return map[string]any{
			"description": "Dry-run and plan test suite",
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": promptText,
					},
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("prompt %q not found", input.Name)
	}
}

func guide() map[string]any {
	return map[string]any{
		"workflow": []string{
			"Start with testdesk_snapshot includeTests=false.",
			"Use testdesk_catalog with narrow filters and limit/offset when selecting tests.",
			"Use testdesk_plan before broad runs so the user/agent can validate scope.",
			"Use testdesk_run with explicit testIds where possible; use providerIds or filters for intentional bulk runs.",
			"Poll with testdesk_snapshot or testdesk_wait; fetch diagnostics only for failing/cancelled runs.",
		},
		"filters":     []string{"providerIds", "providerKinds", "layers", "modules", "statuses", "tags", "query", "limit", "offset"},
		"controls":    []string{"pause", "resume", "skip", "stop", "kill"},
		"projects":    "Use testdesk_projects to list names and roots. Pass project on any project-scoped tool to target one call without changing the active project.",
		"tokenPolicy": "Snapshots omit test lists by default. Catalog and diagnostics are paged/bounded. Prefer IDs over names once discovered.",
		"safety":      "TestDesk only runs configured project test commands. Agents should plan broad requests first and avoid kill unless stop/skip failed.",
	}
}

func (s *Server) snapshot(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	includeTests := boolArg(args, "includeTests", false)
	filter := filterFromArgs(args)
	if !includeTests {
		filter.Limit = 0
	}
	snap, err := mgr.Snapshot(filter)
	if err != nil {
		return nil, err
	}
	if !includeTests {
		snap.Tests = nil
	}
	return compactSnapshot(snap), nil
}

func (s *Server) catalog(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	filter := filterFromArgs(args)
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	snap, err := mgr.Snapshot(filter)
	if err != nil {
		return nil, err
	}
	return map[string]any{"project": snap.ProjectName, "totalKnown": snap.TestCount, "count": len(snap.Tests), "offset": filter.Offset, "limit": filter.Limit, "tests": compactTests(snap.Tests)}, nil
}

func (s *Server) discoverProject(ctx context.Context, args map[string]any) (any, error) {
	mgr, err := s.managerForLongRequest(ctx, args)
	if err != nil {
		return nil, err
	}
	emit, _ := ctx.Value(progressEmitterKey{}).(func(int, int, string))
	last := -1
	var onProgress manager.DiscoverProgressFunc
	if emit != nil {
		onProgress = func(p manager.DiscoverProgress) {
			if p.Total == 0 {
				return
			}
			value := -1
			if p.Phase == "starting" && p.Current > 0 {
				value = p.Current - 1
			}
			if p.Phase == "provider_done" && p.Current == p.Total {
				value = p.Current
			}
			if value > last {
				last = value
				emit(value, p.Total, p.Detail)
			}
		}
	}
	tests, err := mgr.DiscoverWithProgress(ctx, onProgress)
	return map[string]any{"count": len(tests), "warning": errorString(err)}, nil
}

func (s *Server) plan(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	items, err := mgr.Plan(runRequestFromArgs(args))
	if err != nil {
		return nil, err
	}
	return map[string]any{"total": len(items), "items": compactItems(items)}, nil
}

func (s *Server) run(ctx context.Context, args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	run, err := mgr.Run(ctx, runRequestFromArgs(args))
	if err != nil {
		return nil, err
	}
	return map[string]any{"runId": run.ID, "status": run.Status, "total": run.Total, "items": compactItems(run.Items)}, nil
}

func (s *Server) control(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	action := manager.ControlAction(stringArg(args, "action"))
	if err := mgr.Control(action); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "action": action}, nil
}

func (s *Server) wait(ctx context.Context, args map[string]any) (any, error) {
	mgr, err := s.managerForLongRequest(ctx, args)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(intArg(args, "timeoutMs", 30000)) * time.Millisecond
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err = mgr.WaitForIdle(waitCtx)
	snap, snapErr := mgr.Snapshot(domain.Filter{Limit: 0})
	if snapErr != nil {
		return nil, snapErr
	}
	// A zero filter limit is unbounded, so explicitly remove the catalog from
	// the compact wait response just as snapshot does by default.
	snap.Tests = nil
	payload := compactSnapshot(snap)
	payload["idle"] = err == nil
	if err != nil {
		payload["warning"] = err.Error()
	}
	return payload, nil
}

func (s *Server) diagnostics(args map[string]any) (any, error) {
	mgr, err := s.managerForArgs(args)
	if err != nil {
		return nil, err
	}
	maxBytes := int64(intArg(args, "maxBytes", 32768))
	if maxBytes <= 0 {
		maxBytes = 32768
	}
	if maxBytes > 512000 {
		maxBytes = 512000
	}
	diag, err := mgr.Diagnostics(stringArg(args, "runId"), maxBytes)
	if err != nil {
		return nil, err
	}
	includeRun := boolArg(args, "includeRun", true)
	includeLog := boolArg(args, "includeLog", true)
	out := map[string]any{"path": diag.Path}
	if includeRun && diag.Run != nil {
		out["run"] = compactRun(*diag.Run)
	}
	if includeLog {
		out["log"] = diag.Log
	}
	return out, nil
}

func runRequestFromArgs(args map[string]any) manager.RunRequest {
	filter := filterFromArgs(args)
	// A provider-only request intentionally selects providers, including those
	// without discovery. Keep provider IDs in the filter when narrowing tests.
	if strings.TrimSpace(filter.Query) == "" && len(filter.ProviderKinds) == 0 && len(filter.Layers) == 0 && len(filter.Modules) == 0 && len(filter.Tags) == 0 && len(filter.Statuses) == 0 {
		filter.ProviderIDs = nil
	}
	return manager.RunRequest{TestIDs: stringsArg(args, "testIds"), ProviderIDs: stringsArg(args, "providerIds"), Filter: filter}
}

func filterFromArgs(args map[string]any) domain.Filter {
	return domain.Filter{
		Query:         stringArg(args, "query"),
		ProviderIDs:   stringsArg(args, "providerIds"),
		ProviderKinds: stringsArg(args, "providerKinds"),
		Layers:        stringsArg(args, "layers"),
		Modules:       stringsArg(args, "modules"),
		Tags:          stringsArg(args, "tags"),
		Statuses:      statusArgs(args, "statuses"),
		Limit:         intArg(args, "limit", 0),
		Offset:        intArg(args, "offset", 0),
	}
}

func compactSnapshot(snapshot domain.Snapshot) map[string]any {
	out := map[string]any{
		"project":    snapshot.ProjectName,
		"providers":  snapshot.ProviderCount,
		"tests":      snapshot.TestCount,
		"queued":     snapshot.QueuedCount,
		"generated":  snapshot.GeneratedAt.Format(time.RFC3339),
		"activeIds":  snapshot.ActiveProviderIDs,
		"recentRuns": compactRuns(snapshot.RecentRuns),
	}
	if snapshot.Running != nil {
		out["running"] = compactRun(*snapshot.Running)
	}
	if snapshot.Tests != nil {
		out["testPage"] = compactTests(snapshot.Tests)
	}
	return out
}

func compactRuns(runs []domain.Run) []map[string]any {
	out := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		out = append(out, compactRun(run))
	}
	return out
}

func compactRun(run domain.Run) map[string]any {
	out := map[string]any{"id": run.ID, "status": run.Status, "passed": run.Passed, "failed": run.Failed, "skipped": run.Skipped, "cancelled": run.Cancelled, "total": run.Total, "currentIndex": run.CurrentIndex}
	if run.CurrentIndex >= 0 && run.CurrentIndex < len(run.Items) {
		out["current"] = compactItem(run.Items[run.CurrentIndex])
	}
	return out
}

func compactItems(items []domain.RunItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, compactItem(item))
	}
	return out
}

func compactItem(item domain.RunItem) map[string]any {
	result := map[string]any{"id": item.ID, "provider": item.ProviderID, "kind": item.ProviderKind, "testId": item.TestID, "name": item.TestName, "status": item.Status, "ms": item.DurationMillis, "etaMs": item.EstimatedMillis, "error": item.Error}
	if len(item.Warnings) > 0 {
		result["warnings"] = item.Warnings
	}
	return result
}

func compactTests(tests []domain.TestCase) []map[string]any {
	out := make([]map[string]any, 0, len(tests))
	for _, test := range tests {
		out = append(out, map[string]any{"id": test.ID, "provider": test.ProviderID, "kind": test.ProviderKind, "layer": test.Layer, "module": test.Module, "name": test.Name, "fullName": test.FullName, "selector": test.Selector, "file": test.File, "line": test.Line, "status": test.LastStatus, "lastMs": test.LastDurationMillis, "tags": test.Tags})
	}
	return out
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}
func projectSchema(properties map[string]any) map[string]any {
	properties["project"] = stringSchema()
	return objectSchema(properties)
}
func stringSchema() map[string]any { return map[string]any{"type": "string"} }
func boolSchema() map[string]any   { return map[string]any{"type": "boolean"} }
func numberSchema() map[string]any { return map[string]any{"type": "number"} }
func stringArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": stringSchema()}
}
func enumSchema(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if value, ok := args[key].(bool); ok {
		return value
	}
	return fallback
}
func stringArg(args map[string]any, key string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}
func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}
func stringsArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}
func statusArgs(args map[string]any, key string) []domain.Status {
	strings := stringsArg(args, key)
	statuses := make([]domain.Status, 0, len(strings))
	for _, item := range strings {
		statuses = append(statuses, domain.Status(item))
	}
	return statuses
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func projectKey(root string) string {
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}
	return strings.ToLower(filepath.Clean(root))
}

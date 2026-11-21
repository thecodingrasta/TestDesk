// Fixture data and screen scenarios for the product screenshots.
// Consumed by capture.cjs. Every value here is sample data: project roots use a
// neutral demo path, and test names are real names taken from the TestDesk and
// Workcopy Go test suites. Timestamps are computed from the capture time so the
// running-run clock and "opened" dates look current.

const DEMO_HOME = 'C:\\Users\\demo\\Documents\\GitHub';
const TESTDESK_ROOT = `${DEMO_HOME}\\TestDesk`;
const WORKCOPY_ROOT = `${DEMO_HOME}\\Workcopy`;

function testCase(providerId, providerName, providerKind, layer, name, module, status, extra = {}) {
  return {
    id: `${providerId}:${module}.${name}`,
    providerId,
    providerName,
    providerKind,
    name,
    fullName: `${module}.${name}`,
    selector: name,
    layer,
    module,
    tags: [],
    lastStatus: status,
    lastDurationMillis: 0,
    discoveredAt: null,
    ...extra,
  };
}

function runItem(run, index, test, status, extra = {}) {
  return {
    id: `${run.id}-${index}`,
    runId: run.id,
    providerId: test.providerId,
    providerName: test.providerName,
    providerKind: test.providerKind,
    testId: test.id,
    testName: test.name,
    commandPreview: `go test -run ^${test.name}$ ./${test.module.split('.')[0]}`,
    status,
    durationMillis: status === 'queued' || status === 'running' ? 0 : 900 + index * 137,
    ...extra,
  };
}

function buildFixtures(now = new Date()) {
  const iso = (msAgo) => new Date(now.getTime() - msAgo).toISOString();

  const testdeskProviders = [
    { id: 'go', name: 'Go', kind: 'go', layer: 'unit', root: TESTDESK_ROOT, enabled: true, tags: [] },
    { id: 'dotnet', name: '.NET (xUnit)', kind: 'dotnet', layer: 'unit', root: TESTDESK_ROOT, enabled: true, tags: [] },
    { id: 'cargo', name: 'Cargo', kind: 'cargo', layer: 'integration', root: TESTDESK_ROOT, enabled: true, tags: [] },
  ];

  const testdeskTests = [
    testCase('go', 'Go', 'go', 'unit', 'TestDisconnectCancelsPendingDiscovery', 'internal/mcpbridge', 'passed', { lastDurationMillis: 1204, lastRunId: 'run-live' }),
    testCase('go', 'Go', 'go', 'unit', 'TestBridgeDiscoveryProgressAndCancellation', 'internal/mcpbridge', 'passed', { lastDurationMillis: 3710, lastRunId: 'run-live' }),
    testCase('go', 'Go', 'go', 'unit', 'TestDiscoveryFailureRetainsPreviousProviderCatalog', 'internal/runner', 'passed', { lastDurationMillis: 842, lastRunId: 'run-live' }),
    testCase('go', 'Go', 'go', 'unit', 'TestDiscoveryPersistsTestsAfterLongOutputLine', 'internal/runner', 'passed', { lastDurationMillis: 615, lastRunId: 'run-live' }),
    testCase('go', 'Go', 'go', 'unit', 'TestDiagnosticsRejectsRunPathTraversal', 'internal/manager', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestRerunProviderWideScopeFailsClosed', 'internal/manager', 'unknown'),
    testCase('dotnet', '.NET (xUnit)', 'dotnet', 'unit', 'TestDuplicateDotNetCaseCannotBeTargeted', 'SampleSuite', 'unknown'),
    testCase('dotnet', '.NET (xUnit)', 'dotnet', 'unit', 'TestDotNetAdapterFailurePreservesPreviousCatalog', 'SampleSuite', 'unknown'),
    testCase('cargo', 'Cargo', 'cargo', 'integration', 'TestCargoIncompleteDiscoveryRetainsPriorCatalog', 'cargo_discovery', 'unknown'),
    testCase('cargo', 'Cargo', 'cargo', 'integration', 'TestCargoPartialOutputDoesNotPersistPassingEvidence', 'cargo_partial_output', 'unknown'),
  ];

  const workcopyProviders = [
    { id: 'go', name: 'Go', kind: 'go', layer: 'unit', root: WORKCOPY_ROOT, enabled: true, tags: [] },
  ];
  const workcopyTests = [
    testCase('go', 'Go', 'go', 'unit', 'TestSelectedAgentsServeOnlyTheirFiles', 'internal/agents', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestOwnerConfigOptInCopiesSettingsButNotCredentials', 'internal/agents', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestDetectionCopiesInstructionsAndNothingElse', 'internal/plan', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestSymbolicLinksAreNeverFollowed', 'internal/plan', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestExecutableBitIsKeptOnlyWhenAsked', 'internal/plan', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestClientNamesSelectTheirTool', 'internal/agents', 'unknown'),
    testCase('go', 'Go', 'go', 'unit', 'TestSelectionPrecedenceAndFallbacks', 'internal/agents', 'unknown'),
  ];

  const projects = [
    { root: TESTDESK_ROOT, name: 'TestDesk', active: true, configured: true, lastOpened: iso(5 * 60e3), providerCount: testdeskProviders.length, testCount: testdeskTests.length },
    { root: WORKCOPY_ROOT, name: 'Workcopy', active: false, configured: true, lastOpened: iso(42 * 60e3), providerCount: workcopyProviders.length, testCount: workcopyTests.length },
  ];

  const ui = {
    theme: 'dark',
    widgetMode: 'always',
    widgetSize: 'medium',
    alwaysOnTop: true,
    topmostMode: 'always',
    startMinimized: false,
    compactWidth: 400,
    compactHeight: 240,
    compactExpandedHeight: 420,
    managerWidth: 1280,
    managerHeight: 820,
  };

  const startup = {
    configured: true,
    projectRoot: TESTDESK_ROOT,
    projectName: 'TestDesk',
    suggestedProjectRoot: TESTDESK_ROOT,
    preferencePath: `${DEMO_HOME}\\..\\AppData\\Roaming\\TestDesk\\preferences.json`,
    configPath: `${TESTDESK_ROOT}\\testdesk.json`,
    providers: testdeskProviders,
    projects,
    ui,
    detectedPreview: null,
  };

  const testsWithStatus = (tests) => tests.map((t) => ({ ...t }));

  // Run in progress: five tests passed, the sixth is running, the rest are queued.
  const liveRun = {
    id: 'run-live',
    status: 'running',
    createdAt: iso(40e3),
    startedAt: iso(40e3),
    currentIndex: 5,
    total: testdeskTests.length,
    passed: 5,
    failed: 0,
    skipped: 0,
    cancelled: 0,
    items: testdeskTests.map((t, i) =>
      runItem({ id: 'run-live' }, i, t, i < 5 ? 'passed' : i === 5 ? 'running' : 'queued', {
        startedAt: i <= 5 ? iso((40 - i * 5) * 1e3) : null,
        finishedAt: i < 5 ? iso((35 - i * 5) * 1e3) : null,
      })),
  };

  // Completed run with one failure, shown on the Recent Runs tab.
  const failedTestName = 'TestCargoPartialOutputDoesNotPersistPassingEvidence';
  const failedRun = {
    id: 'run-failed',
    status: 'failed',
    createdAt: iso(18 * 60e3),
    startedAt: iso(18 * 60e3),
    finishedAt: iso(17 * 60e3),
    currentIndex: testdeskTests.length,
    total: testdeskTests.length,
    passed: testdeskTests.length - 1,
    failed: 1,
    skipped: 0,
    cancelled: 0,
    logPath: 'run-failed.log',
    items: testdeskTests.map((t, i) => {
      const failed = t.name === failedTestName;
      return runItem({ id: 'run-failed' }, i, t, failed ? 'failed' : 'passed', {
        error: failed ? 'expected 2 cargo targets after partial output, found 3' : undefined,
        startedAt: iso((18 * 60 - i * 9) * 1e3),
        finishedAt: iso((18 * 60 - i * 9 - 6) * 1e3),
      });
    }),
  };

  const runLogs = {
    'run-live': [
      '▶ Discovering Go tests in TestDesk',
      '✔ TestDisconnectCancelsPendingDiscovery  1.2s',
      '✔ TestBridgeDiscoveryProgressAndCancellation  3.7s',
      '✔ TestDiscoveryFailureRetainsPreviousProviderCatalog  0.8s',
      '✔ TestDiscoveryPersistsTestsAfterLongOutputLine  0.6s',
      '▶ TestDiagnosticsRejectsRunPathTraversal',
    ].join('\n'),
    'run-failed': [
      '▶ Running 11 tests for TestDesk',
      '✔ Go: 6 passed',
      '✔ .NET (xUnit): 2 passed',
      '✖ Cargo: TestCargoPartialOutputDoesNotPersistPassingEvidence',
      '    expected 2 cargo targets after partial output, found 3',
      '✔ Cargo: 2 passed',
      'Finished: 10 passed, 1 failed in 18.4s',
    ].join('\n'),
  };

  const snapshot = (tests, running, recentRuns) => ({
    projectName: 'TestDesk',
    providerCount: testdeskProviders.length,
    testCount: tests.length,
    queuedCount: 0,
    paused: false,
    running: running || null,
    recentRuns,
    tests: testsWithStatus(tests),
    activeProviderIds: testdeskProviders.map((p) => p.id),
    generatedAt: now.toISOString(),
  });

  const mcpTargets = [
    { id: 'codex', name: 'Codex', description: 'OpenAI Codex CLI / IDE extension stdio MCP configuration.', scope: 'User config', configPath: '~/.codex/config.toml', detected: true, installed: true, canInstall: true, commandPreview: ['testdesk', 'mcp'], manual: '[mcp_servers.testdesk]\ncommand = "C:\\\\Tools\\\\TestDesk\\\\TestDesk.exe"\nargs = ["mcp"]\nenabled = true\nstartup_timeout_sec = 20\ntool_timeout_sec = 600\n', notes: [], manualOnly: false },
    { id: 'claude-code', name: 'Claude Code', description: 'Anthropic Claude Code CLI', scope: 'Project config', configPath: '.mcp.json', detected: true, installed: false, canInstall: true, commandPreview: ['testdesk', 'mcp'], manual: '{\n  "mcpServers": {\n    "testdesk": {\n      "command": "C:\\\\Tools\\\\TestDesk\\\\TestDesk.exe",\n      "args": ["mcp"]\n    }\n  }\n}', notes: [], manualOnly: false },
    { id: 'opencode', name: 'OpenCode', description: 'OpenCode terminal agent', scope: 'Project config', configPath: 'opencode.json', detected: false, installed: false, canInstall: true, commandPreview: ['testdesk', 'mcp'], manual: 'Add a local MCP entry to opencode.json.', notes: [], manualOnly: false },
    { id: 'antigravity', name: 'Antigravity', description: 'Google Antigravity agent IDE', scope: 'User config', configPath: '~/.gemini/config/mcp_config.json', detected: false, installed: false, canInstall: true, commandPreview: ['testdesk', 'mcp'], manual: 'Add a local MCP entry to mcp_config.json.', notes: [], manualOnly: false },
  ];

  const activeProjectsList = projects.map((p) => ({ root: p.root, name: p.name, active: true, configured: true, providerCount: p.providerCount, testCount: p.testCount, lastOpened: p.lastOpened }));

  return {
    startup,
    projects,
    ui,
    mcpTargets,
    activeProjects: activeProjectsList,
    testdeskTests,
    liveSnapshot: snapshot(testdeskTests, liveRun, []),
    failedSnapshot: snapshot(testdeskTests, null, [failedRun]),
    idleSnapshot: snapshot(testdeskTests, null, []),
    runLogs,
    workcopyProviders,
    workcopyTests,
  };
}

// Each shot names one screen state. `js` runs in the page after the app has booted.
// Keep each shot distinct: one screen, one purpose, no near-duplicates.
const SHOTS = [
  {
    name: 'manager-overview',
    description: 'Manager window with TestDesk discovering and running Go tests',
    width: 1280,
    height: 820,
    snapshot: 'live',
    js: `await setMode('manager'); await refresh(true); setRunPane('active'); renderManager();`,
  },
  {
    name: 'manager-recent-failure',
    description: 'Manager window showing a failed run on the Recent Runs tab',
    width: 1280,
    height: 820,
    snapshot: 'failed',
    js: `await setMode('manager'); await refresh(true); setRunPane('recent'); renderManager();`,
  },
  {
    name: 'compact-running',
    description: 'Compact controller while one project runs its tests',
    width: 480,
    height: 106,
    snapshot: 'live',
    js: `await setMode('compact'); await refresh(false); renderPlayer();`,
  },
  {
    name: 'compact-small-running',
    description: 'Small compact HUD (380px) while a run is active, with the optional heading parts collapsed to fit',
    width: 380,
    height: 96,
    snapshot: 'live',
    js: `startupState.ui.widgetSize = 'small'; await setMode('compact'); await refresh(false); renderPlayer();`,
  },
  {
    name: 'compact-multi-project',
    description: 'Compact controller with two active projects, TestDesk and Workcopy',
    width: 480,
    height: 106,
    snapshot: 'live',
    js: `await setMode('compact'); await refresh(false); renderCompactProjects(); renderPlayer();`,
  },
  {
    name: 'projects',
    description: 'Projects page listing the connected TestDesk and Workcopy workspaces',
    width: 1280,
    height: 820,
    snapshot: 'idle',
    js: `await setMode('setup');`,
  },
  {
    name: 'settings-general',
    description: 'Settings, General tab: compact controller and retention preferences',
    width: 1280,
    height: 820,
    snapshot: 'idle',
    js: `await setMode('settings'); setSettingsPane('general');`,
  },
  {
    name: 'settings-agent-integrations',
    description: 'Settings, Agent Integrations tab: detected agent workspaces',
    width: 1280,
    height: 820,
    snapshot: 'idle',
    js: `await setMode('settings'); setSettingsPane('agents');`,
  },
];

module.exports = { buildFixtures, SHOTS, DEMO_HOME, TESTDESK_ROOT, WORKCOPY_ROOT };

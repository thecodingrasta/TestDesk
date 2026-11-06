const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '..', 'assets', 'app.js'), 'utf8');
test('compact project picker shows only concurrently active projects', () => {
  const begin = source.indexOf('function renderCompactProjects()');
  const finish = source.indexOf('\nfunction debounce(', begin);
  assert.ok(begin >= 0 && finish > begin);
  const elements = new Map();
  const element = id => {
    if (!elements.has(id)) {
      const classes = new Set();
      elements.set(id, { classList: { toggle: (name, force) => force ? classes.add(name) : classes.delete(name), contains: name => classes.has(name) }, dataset: {}, innerHTML: '' });
    }
    return elements.get(id);
  };
  const context = { $: element, startupState: { configured: true, projectRoot: 'A' }, activeProjectsRefreshFailed: false, lastActiveProjectsUpdate: Date.now(), activeProjects: [{ root: 'A', name: 'Alpha', active: true }, { root: 'B', name: 'Beta', active: false }], same: (a, b) => a === b, escapeAttr: value => value, escapeHtml: value => value, Date, JSON };
  vm.runInNewContext(source.slice(begin, finish), context);
  context.renderCompactProjects();
  assert.equal(element('compactProjectSelect').classList.contains('hidden'), true);
  context.activeProjects.push({ root: 'C', name: 'Gamma', active: true });
  context.renderCompactProjects();
  assert.equal(element('compactProjectSelect').classList.contains('hidden'), false);
  assert.match(element('compactProjectSelect').innerHTML, /Alpha/);
  assert.match(element('compactProjectSelect').innerHTML, /Gamma/);
  assert.doesNotMatch(element('compactProjectSelect').innerHTML, /Beta/);
});
const start = source.indexOf('function statusUnavailable(');
const end = source.indexOf('\nfunction setCompactState(', start);
assert.ok(start >= 0 && end > start);

test('widget marks failed or delayed status as unavailable and recovers on a fresh update', () => {
  let now = 10000;
  const elements = new Map();
  const getElement = id => {
    if (!elements.has(id)) {
      const classes = new Set();
      elements.set(id, { classList: { add: value => classes.add(value), remove: value => classes.delete(value), contains: value => classes.has(value) }, style: {}, textContent: '' });
    }
    return elements.get(id);
  };
  const item = { status: 'running', testName: 'fixture' };
  const context = {
    Date: { now: () => now }, lastStatusUpdate: now, statusRefreshFailed: false,
    $: getElement, snapshot: { running: { status: 'running', items: [item] } },
    startupState: { configured: true, projectName: 'fixture', ui: {} }, mode: 'compact',
    widgetPaused: false, wasRunning: false, runInitiatedFromManager: false,
    updateWidgetActionButton() {}, renderCompactStats() {}, currentItem: () => item,
    renderWidgetProgress(value) { context.progress = value; }, renderCompactDetails() {},
    compactStateTone: status => status, formatStatus: status => status,
    currentElapsed: () => 1000, formatMs: () => '1s',
    setCompactState(label) { context.label = label; }
  };
  const activeStart = source.indexOf('function runIsActive(');
  const activeEnd = source.indexOf('\nfunction workItemProgress(', activeStart);
  vm.runInNewContext(source.slice(activeStart, activeEnd) + source.slice(start, end), context);
  context.renderPlayer();
  assert.equal(context.label, 'running');
  now += 6001;
  context.renderPlayer();
  assert.equal(context.label, 'Status unavailable');
  assert.equal(getElement('runClock').classList.contains('hidden'), true);
  assert.equal(getElement('widgetPassed').textContent, '—');
  assert.equal(context.progress, null);
  context.lastStatusUpdate = now;
  context.statusRefreshFailed = true;
  context.renderPlayer();
  assert.equal(context.label, 'Status unavailable');
  context.statusRefreshFailed = false;
  context.renderPlayer();
  assert.equal(context.label, 'running');
  assert.equal(getElement('runClock').classList.contains('hidden'), false);
});

test('switching projects accepts the returned snapshot as fresh after a polling failure', async () => {
  const switchStart = source.indexOf('async function switchProject(');
  const switchEnd = source.indexOf('\nasync function refreshProject(', switchStart);
  assert.ok(switchStart >= 0 && switchEnd > switchStart);
  const context = {
    Date: { now: () => 20000 }, lastStatusUpdate: 1000, statusRefreshFailed: true,
    snapshot: null, startupState: { projectRoot: 'old' }, mode: 'compact',
    projectSwitchGeneration: 0, frontendLoadingOperations: new Set(),
    catalogueTests: [], collapsedProviders: new Set(), filterOptionsKey: '',
    selected: new Set(), expandedTests: new Set(), selectedRunId: '', lastTestsRenderKey: '',
    same: (a, b) => a === b,
    call: async () => ({ startup: { projectRoot: 'new' }, snapshot: { running: { id: 'new-run' } } }),
    presentLoading() {}, applyMode() {}, leaveLoading() {}, renderPlayer() {}, renderCompactProjects() {}
  };
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  assert.ok(acceptStart >= 0 && acceptEnd > acceptStart);
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(switchStart, switchEnd), context);
  await context.switchProject('new');
  assert.equal(context.snapshot.running.id, 'new-run');
  assert.equal(context.lastStatusUpdate, 20000);
  assert.equal(context.statusRefreshFailed, false);
});

test('a failed poll marks status unavailable and a later poll restores it', async () => {
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  const refreshStart = source.indexOf('async function refresh(');
  const refreshEnd = source.indexOf('\nfunction renderCompactProjects(', refreshStart);
  assert.ok(acceptStart >= 0 && acceptEnd > acceptStart && refreshStart >= 0 && refreshEnd > refreshStart);
  let fail = true;
  const context = {
    Date: { now: () => 20000 }, lastStatusUpdate: 1000, statusRefreshFailed: false,
    snapshot: null, startupState: { configured: true }, mode: 'compact',
    refreshInFlight: false, activeProjectsInFlight: false, activeProjectsRefreshFailed: false, lastActiveProjectsUpdate: 0,
    frontendLoadingOperations: new Set(), projectSwitchGeneration: 0,
    catalogueTests: [], activeProjects: [], lastWidgetVisibilityState: false,
    console: { error() {} },
    call: async method => {
      if (method === 'Snapshot') {
        if (fail) throw new Error('disconnected');
        return { running: null, recentRuns: [] };
      }
      if (method === 'ActiveProjects') return [];
      throw new Error(`Unexpected call: ${method}`);
    },
    renderCompactProjects() {}, renderPlayer() {}, runIsActive: () => false
  };
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(refreshStart, refreshEnd), context);
  await context.refresh();
  assert.equal(context.statusRefreshFailed, true);
  assert.equal(context.refreshInFlight, false);
  fail = false;
  await context.refresh();
  assert.equal(context.statusRefreshFailed, false);
  assert.equal(context.lastStatusUpdate, 20000);
  assert.equal(context.refreshInFlight, false);
});

test('an old project polling failure does not mark the new project unavailable', async () => {
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  const refreshStart = source.indexOf('async function refresh(');
  const refreshEnd = source.indexOf('\nfunction renderCompactProjects(', refreshStart);
  assert.ok(acceptStart >= 0 && acceptEnd > acceptStart && refreshStart >= 0 && refreshEnd > refreshStart);
  let rejectOldSnapshot;
  const oldSnapshot = new Promise((resolve, reject) => { rejectOldSnapshot = reject; });
  const context = {
    Date: { now: () => 20000 }, lastStatusUpdate: 1000, statusRefreshFailed: false,
    snapshot: null, startupState: { configured: true }, mode: 'compact',
    refreshInFlight: false, activeProjectsInFlight: false, activeProjectsRefreshFailed: false, lastActiveProjectsUpdate: 0,
    frontendLoadingOperations: new Set(), projectSwitchGeneration: 0,
    catalogueTests: [], activeProjects: [], lastWidgetVisibilityState: false,
    console: { error() {} }, call: method => method === 'Snapshot' ? oldSnapshot : Promise.resolve([]),
    renderCompactProjects() {}, renderPlayer() {}, runIsActive: () => false
  };
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(refreshStart, refreshEnd), context);
  const pending = context.refresh();
  context.projectSwitchGeneration++;
  context.acceptSnapshot({ projectName: 'new' });
  rejectOldSnapshot(new Error('old project disconnected'));
  await pending;
  assert.equal(context.snapshot.projectName, 'new');
  assert.equal(context.statusRefreshFailed, false);
  assert.equal(context.lastStatusUpdate, 20000);
});

test('a stalled visibility update does not stop subsequent status polls', async () => {
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  const refreshStart = source.indexOf('async function refresh(');
  const refreshEnd = source.indexOf('\nfunction renderCompactProjects(', refreshStart);
  let snapshots = 0;
  const context = {
    Date, lastStatusUpdate: 0, statusRefreshFailed: false, snapshot: null,
    startupState: { configured: true }, mode: 'compact', refreshInFlight: false,
    activeProjectsInFlight: false, activeProjectsRefreshFailed: false, lastActiveProjectsUpdate: 0,
    frontendLoadingOperations: new Set(), projectSwitchGeneration: 0,
    catalogueTests: [], activeProjects: [], lastWidgetVisibilityState: null,
    console: { error() {} },
    call: method => {
      if (method === 'Snapshot') return Promise.resolve({ running: { id: String(++snapshots) }, recentRuns: [] });
      if (method === 'ActiveProjects') return Promise.resolve([]);
      if (method === 'ApplyWidgetVisibility') return new Promise(() => {});
      throw new Error(`Unexpected call: ${method}`);
    },
    renderCompactProjects() {}, renderPlayer() {}, runIsActive: () => true
  };
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(refreshStart, refreshEnd), context);
  const completed = await Promise.race([context.refresh().then(() => true), new Promise(resolve => setTimeout(() => resolve(false), 50))]);
  assert.equal(completed, true, 'visibility request blocked status refresh');
  await context.refresh();
  assert.equal(snapshots, 2);
  assert.equal(context.snapshot.running.id, '2');
});

test('a stalled project list does not stop snapshots and late old-project data is ignored', async () => {
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  const refreshStart = source.indexOf('async function refresh(');
  const refreshEnd = source.indexOf('\nfunction renderCompactProjects(', refreshStart);
  let snapshots = 0;
  let projectCalls = 0;
  let resolveProjects;
  const pendingProjects = new Promise(resolve => { resolveProjects = resolve; });
  const context = {
    Date, lastStatusUpdate: 0, statusRefreshFailed: false, snapshot: null,
    startupState: { configured: true }, mode: 'compact', refreshInFlight: false,
    activeProjectsInFlight: false, lastActiveProjectsUpdate: Date.now(), activeProjectsRefreshFailed: false,
    frontendLoadingOperations: new Set(), projectSwitchGeneration: 0,
    catalogueTests: [], activeProjects: [], console: { error() {} },
    call: method => {
      if (method === 'Snapshot') return Promise.resolve({ running: { id: String(++snapshots) }, recentRuns: [] });
      if (method === 'ActiveProjects') { projectCalls++; return pendingProjects; }
      throw new Error(`Unexpected call: ${method}`);
    },
    renderCompactProjects() {}, renderPlayer() {}, runIsActive: () => true
  };
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(refreshStart, refreshEnd), context);
  const completed = await Promise.race([context.refresh().then(() => true), new Promise(resolve => setTimeout(() => resolve(false), 50))]);
  assert.equal(completed, true, 'project-list request blocked status refresh');
  await context.refresh();
  assert.equal(snapshots, 2);
  assert.equal(projectCalls, 1, 'stalled project-list calls must remain bounded');
  context.projectSwitchGeneration++;
  resolveProjects([{root:'old'}]);
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(context.activeProjects, []);
});

test('stalled manager startup metadata does not block snapshots or overwrite a newer project', async () => {
  const acceptStart = source.indexOf('function acceptSnapshot(');
  const acceptEnd = source.indexOf('\nfunction statusUnavailable(', acceptStart);
  const refreshStart = source.indexOf('async function refresh(');
  const refreshEnd = source.indexOf('\nfunction renderCompactProjects(', refreshStart);
  let snapshots = 0;
  let startupCalls = 0;
  let resolveStartup;
  const pendingStartup = new Promise(resolve => { resolveStartup = resolve; });
  const context = {
    Date, lastStatusUpdate: 0, statusRefreshFailed: false, snapshot: null,
    startupState: { configured: true, projectRoot: 'old' }, mode: 'manager', refreshInFlight: false,
    startupRefreshInFlight: false, activeProjectsInFlight: false, lastActiveProjectsUpdate: Date.now(), activeProjectsRefreshFailed: false,
    frontendLoadingOperations: new Set(), projectSwitchGeneration: 0, catalogueTests: [], activeProjects: [],
    console: { error() {} },
    call: method => {
      if (method === 'Startup') { startupCalls++; return pendingStartup; }
      if (method === 'Snapshot') return Promise.resolve({ running: { id: String(++snapshots) }, recentRuns: [], tests: [] });
      if (method === 'ActiveProjects') return Promise.resolve([]);
      throw new Error(`Unexpected call: ${method}`);
    },
    showCatalogueLoader() {}, hideCatalogueLoader() {}, renderCompactProjects() {}, renderPlayer() {}, renderManager() {}
  };
  vm.runInNewContext(source.slice(acceptStart, acceptEnd) + '\n' + source.slice(refreshStart, refreshEnd), context);
  const completed = await Promise.race([context.refresh(true).then(() => true), new Promise(resolve => setTimeout(() => resolve(false), 50))]);
  assert.equal(completed, true, 'startup metadata blocked status refresh');
  await context.refresh(true);
  assert.equal(snapshots, 2);
  assert.equal(startupCalls, 1, 'stalled startup calls must remain bounded');
  context.projectSwitchGeneration++;
  context.startupState = { configured: true, projectRoot: 'new' };
  resolveStartup({ configured: true, projectRoot: 'old' });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(context.startupState.projectRoot, 'new');
});

test('startup metadata cannot overwrite a newer state and can retry after failure', async () => {
  const start = source.indexOf('async function refreshStartup(');
  const end = source.indexOf('\nasync function refreshActiveProjects(', start);
  assert.ok(start >= 0 && end > start);
  let resolveStartup;
  let attempts = 0;
  const context = {
    startupRefreshInFlight: false, projectSwitchGeneration: 0,
    startupState: { configured: true, projectName: 'before' }, mode: 'compact',
    call: () => {
      attempts++;
      if (attempts === 1) return new Promise(resolve => { resolveStartup = resolve; });
      if (attempts === 2) return Promise.reject(new Error('unavailable'));
      return Promise.resolve({ configured: true, projectName: 'fresh' });
    },
    console: { error() {} }, renderCompactProjects() {}, renderPlayer() {}
  };
  vm.runInNewContext(source.slice(start, end), context);
  const previous = context.startupState;
  const pending = context.refreshStartup(0, previous);
  context.startupState = { configured: true, projectName: 'edited' };
  resolveStartup({ configured: true, projectName: 'stale' });
  await pending;
  assert.equal(context.startupState.projectName, 'edited');
  await context.refreshStartup(0, context.startupState);
  assert.equal(context.startupRefreshInFlight, false);
  await context.refreshStartup(0, context.startupState);
  assert.equal(context.startupState.projectName, 'fresh');
});

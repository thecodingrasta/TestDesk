const $ = (id) => document.getElementById(id);

const selected = new Set();
const expandedTests = new Set();
const collapsedProviders = new Set();
let snapshot = null;
let lastStatusUpdate = Date.now();
let statusRefreshFailed = false;
let startupState = null;
let mode = 'loading';
let lastTests = [];
let setupBusy = false;
let compactDetailsOpen = false;
let selectedRunId = '';
let lastDiagnostics = null;
let logRefreshInFlight = false;
let lastTestsRenderKey = "";
let mcpTargets = [];
let mcpInstallStatus = "";
let mcpSelectedTarget = "";
let loadingReturnMode = 'loading';
let loadingOperation = '';
const frontendLoadingOperations = new Set();
let toastTimer = null;
let refreshInFlight = false;
let startupRefreshInFlight = false;
let activeProjectsInFlight = false;
let activeProjectsRefreshFailed = false;
let lastActiveProjectsUpdate = Date.now();
let filterOptionsKey = '';
let catalogueTests = [];
let filteredTests = [];
let visibleTestLimit = 160;
let filterRenderFrame = 0;
let modeTransition = 0;
let runPane = 'active';
let settingsPane = 'general';
let addProjectSheetOpen = false;
let projectSwitchGeneration = 0;
let activeProjects = [];
const CATALOGUE_BATCH_SIZE = 160;

function showToast(message) {
  const toast = $('toast');
  if (!toast) return;
  toast.textContent = message;
  toast.classList.remove('hidden');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.add('hidden'), 2600);
}

function confirmAction(title, message, acceptLabel = 'Continue') {
  const dialog = $('confirmDialog');
  if (!dialog) return Promise.resolve(window.confirm(message));
  $('confirmTitle').textContent = title;
  $('confirmMessage').textContent = message;
  $('confirmAcceptBtn').textContent = acceptLabel;
  return new Promise(resolve => {
    const finish = () => resolve(dialog.returnValue === 'accept');
    dialog.addEventListener('close', finish, { once: true });
    dialog.showModal();
  });
}

function requestProjectName(currentName) {
  const dialog = $('renameDialog');
  if (!dialog) return Promise.resolve(window.prompt('Project display name', currentName || ''));
  const input = $('renameProjectInput');
  const form = dialog.querySelector('form');
  input.value = currentName || '';
  return new Promise(resolve => {
    let accepted = false;
    const onSubmit = (e) => {
      e.preventDefault();
      accepted = true;
      dialog.close('accept');
    };
    const onClose = () => {
      form.removeEventListener('submit', onSubmit);
      if (accepted || dialog.returnValue === 'accept') {
        resolve(input.value.trim());
      } else {
        resolve(null);
      }
    };
    form.addEventListener('submit', onSubmit, { once: true });
    dialog.addEventListener('close', onClose, { once: true });
    dialog.showModal();
    input.focus();
    input.select();
  });
}

const bridge = () => window.go?.desktop?.App || window.go?.main?.App || null;

async function call(method, ...args) {
  const api = bridge();
  if (!api || typeof api[method] !== 'function') {
    throw new Error('TestDesk desktop connection is unavailable. Restart the application.');
  }
  return api[method](...args);
}


function applyMode(next) {
  mode = next;
  document.body.classList.toggle('manager-open', next === 'manager');
  document.body.classList.toggle('compact-open', next === 'compact');
  $('app').className = next;
  $('app').setAttribute('aria-busy', next === 'loading' ? 'true' : 'false');
  $('splash').classList.toggle('hidden', next !== 'loading');
  const size = startupState?.ui?.widgetSize || 'medium';
  $('player').className = `player panel ${next === 'compact' ? '' : 'hidden'} size-${size}`;
  $('manager').classList.toggle('hidden', next !== 'manager');
  $('setup').classList.toggle('hidden', next !== 'setup');
  $('settings').classList.toggle('hidden', next !== 'settings');
  if (next === 'setup') renderSetup();
  if (next === 'settings') renderSettings().catch(console.error);
  if (next === 'compact') renderPlayer();
}

async function setMode(next) {
  if (next !== 'compact') compactDetailsOpen = false;
  if (next !== 'setup' && addProjectSheetOpen) setAddProjectSheet(false);
  const transition = ++modeTransition;
  const operation = `window-${next}`;
  frontendLoadingOperations.add(operation);
  loadingReturnMode = next;
  presentLoading({
    active: true,
    operation,
    title: next === 'compact' ? 'Opening compact controller' : `Opening ${next === 'setup' ? 'Projects' : next[0].toUpperCase() + next.slice(1)}`,
    detail: 'Preparing the native window and view…'
  });
  try {
    await call('SetWindowMode', next);
    if (transition !== modeTransition) return;
    if (next === 'compact') await call('SetCompactExpanded', compactDetailsOpen);
    applyMode(next);
    if (next === 'manager') await refresh(catalogueTests.length === 0);
  } catch (error) {
    showToast(error.message || String(error));
    leaveLoading(startupState?.configured ? 'manager' : 'setup');
  } finally {
    frontendLoadingOperations.delete(operation);
  }
}

function presentLoading(state) {
  if (!state || typeof state !== 'object') return;
  loadingOperation = state.operation || loadingOperation;
  $('splashOperation').textContent = String(state.operation || 'Working').replace(/-/g, ' ');
  $('splashTitle').textContent = state.title || 'Working…';
  $('splashDetail').textContent = state.detail || 'Please wait while TestDesk finishes this operation.';

  const liveBadge = $('splashLiveBadge');
  const liveCount = $('splashLiveCount');
  if (liveBadge && liveCount) {
    if (typeof state.totalTests === 'number' && state.totalTests > 0) {
      liveCount.textContent = Number(state.totalTests).toLocaleString();
      liveBadge.classList.remove('hidden');
    } else {
      liveBadge.classList.add('hidden');
    }
  }

  const subtitle = $('splashSubtitle');
  if (subtitle) {
    if (state.latestTest) {
      subtitle.textContent = `Latest: ${state.latestTest}`;
      subtitle.title = state.latestTest;
    } else if (state.providerName && state.total) {
      subtitle.textContent = `Provider ${state.current || 1} of ${state.total}: ${state.providerName}`;
    } else {
      subtitle.textContent = 'Loading project state, providers and recent test history.';
    }
  }

  const progressEl = $('splashProgress');
  if (progressEl) {
    if (typeof state.percent === 'number' && state.percent > 0) {
      progressEl.style.width = `${Math.min(100, Math.max(0, state.percent))}%`;
      progressEl.style.animation = 'none';
    } else {
      progressEl.style.width = '45%';
      progressEl.style.animation = '';
    }
  }
  $('splash').classList.toggle('loading-complete', state.active === false);
	if (state.active !== false) {
		if (mode !== 'loading') loadingReturnMode = mode;
		applyMode('loading');
	} else {
		const completedOperation = state.operation || loadingOperation;
		setTimeout(() => {
			if (mode === 'loading' && loadingOperation === completedOperation && !frontendLoadingOperations.has(completedOperation)) leaveLoading();
		}, 120);
	}
}

function leaveLoading(preferredMode = '') {
  const fallback = startupState?.configured ? 'manager' : 'setup';
  const next = preferredMode && preferredMode !== 'loading' ? preferredMode : (loadingReturnMode !== 'loading' ? loadingReturnMode : fallback);
  loadingOperation = '';
  $('splash').classList.remove('loading-complete');
  $('splashLiveBadge')?.classList.add('hidden');
  applyMode(next);
}

function formatMs(ms) {
  if (!ms || ms < 0) return '--';
  const total = Math.floor(ms / 1000);
  const hrs = Math.floor(total / 3600);
  const mins = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hrs > 0) return `${hrs}:${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;
  return `${mins}:${String(secs).padStart(2, '0')}`;
}

function formatDate(value) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return date.toLocaleString();
}

function formatStatus(value) {
  const status = String(value || '').trim().toLocaleLowerCase();
  const labels = {
    cancelled: 'Stopped',
    failed: 'Failed',
    idle: 'Idle',
    passed: 'Passed',
    paused: 'Paused',
    queued: 'Queued',
    running: 'Running',
    skipped: 'Skipped',
    stopped: 'Stopped',
    timed_out: 'Timed Out',
    unknown: 'Unknown'
  };
  return labels[status] || (status ? status.replace(/_/g, ' ').replace(/\b\w/g, letter => letter.toLocaleUpperCase()) : '—');
}

function currentItem() {
  const run = snapshot?.running;
  if (!run || run.currentIndex < 0) return null;
  return run.items?.[run.currentIndex] || null;
}

function currentElapsed(item) {
  if (!item?.startedAt) return item?.durationMillis || 0;
  return Math.max(0, Date.now() - new Date(item.startedAt).getTime());
}

function activeRunId() {
  return selectedRunId || snapshot?.running?.id || snapshot?.recentRuns?.[0]?.id || '';
}

let wasRunning = false;
let runInitiatedFromManager = false;

function runIsActive() {
  return Boolean(snapshot?.running) || (snapshot?.queuedCount || 0) > 0;
}

function workItemProgress(run) {
  const total = Number(run?.total);
  if (!Number.isInteger(total) || total <= 0 || !Array.isArray(run.items) || run.items.length !== total) return null;
  const terminal = new Set(['passed', 'failed', 'skipped', 'cancelled', 'timed_out']);
  const completed = run.items.filter(item => terminal.has(item.status)).length;
  return { completed, total, percent: completed * 100 / total };
}

function renderWidgetProgress(run) {
  const progress = workItemProgress(run);
  const shell = $('widgetProgress');
  const bar = $('progressBar');
  const label = $('widgetProgressText');
  if (!progress) {
    bar.style.width = '0%';
    shell.removeAttribute('aria-valuenow');
    shell.setAttribute('aria-valuetext', 'Work item progress unavailable');
    label.textContent = '';
    return;
  }
  bar.style.width = `${progress.percent}%`;
  shell.setAttribute('aria-valuenow', String(progress.completed));
  shell.setAttribute('aria-valuemax', String(progress.total));
  shell.setAttribute('aria-valuetext', `${progress.completed} of ${progress.total} work items finished`);
  label.textContent = `${progress.completed}/${progress.total} items finished`;
}

function acceptSnapshot(nextSnapshot) {
  snapshot = nextSnapshot;
  lastStatusUpdate = Date.now();
  statusRefreshFailed = false;
}

function statusUnavailable() {
  return statusRefreshFailed || Date.now() - lastStatusUpdate > 6000;
}

function renderPlayer() {
  widgetPaused = Boolean(snapshot?.paused);
  updateWidgetActionButton();
  const run = snapshot?.running;
  const active = runIsActive();
  renderCompactStats(statusUnavailable() ? null : (run || (!active && snapshot?.recentRuns?.[0])));
  const item = currentItem();

  const size = startupState?.ui?.widgetSize || 'medium';
  $('player').className = `player panel ${mode === 'compact' ? '' : 'hidden'} size-${size}`;

  if (!startupState?.configured) {
    $('providerLabel').textContent = 'TestDesk Setup';
    setCompactState('Setup', 'state-warning');
    $('runClock').classList.add('hidden');
    $('testTitle').textContent = 'Choose a project';
    renderWidgetProgress(null);
    renderCompactDetails(null, null);
    return;
  }

  if (statusUnavailable()) {
    $('providerLabel').textContent = startupState.projectName || 'TestDesk';
    setCompactState('Status unavailable', 'state-warning');
    $('runClock').classList.add('hidden');
    $('testTitle').textContent = 'Waiting for a status update';
    $('widgetPassed').textContent = '—';
    $('widgetFailed').textContent = '—';
    renderWidgetProgress(null);
    renderCompactDetails(null, null);
    return;
  }

  if (!run || !item) {
    $('providerLabel').textContent = startupState?.projectName || 'TestDesk';
    setCompactState(active ? (widgetPaused ? 'Paused' : 'Queued') : 'Idle', 'state-warning');
    $('runClock').classList.add('hidden');
    $('testTitle').textContent = active ? 'Tests queued' : 'No active tests';
    const recent = !active ? snapshot?.recentRuns?.[0] : null;
    if (recent && ['passed', 'failed', 'skipped', 'cancelled', 'timed_out', 'unknown'].includes(recent.status)) {
      setCompactState(formatStatus(recent.status), compactStateTone(recent.status));
      $('testTitle').textContent = `Last run: ${formatStatus(recent.status)}`;
    }
    renderWidgetProgress(run || recent);
    renderCompactDetails(null, null);

    if (!active && wasRunning && runInitiatedFromManager) {
      runInitiatedFromManager = false;
      wasRunning = false;
      if (mode === 'compact' && startupState?.ui?.widgetMode === 'when_active') {
        setTimeout(() => {
          if (!runIsActive() && mode === 'compact') {
            setMode('manager').catch(console.error);
          }
        }, 1200);
      }
    } else if (!active) {
      wasRunning = false;
    }
    return;
  }

  wasRunning = true;
  const rawState = widgetPaused ? 'paused' : (item.status || run.status || 'running');
  setCompactState(formatStatus(rawState), compactStateTone(rawState));
  const elapsed = currentElapsed(item);

  const providerLabel = item.providerKind ? `${startupState?.projectName || 'Project'} • ${item.providerKind}` : (startupState?.projectName || 'TestDesk');
  $('providerLabel').textContent = providerLabel;
  $('providerLabel').title = providerLabel;
  $('runClock').textContent = formatMs(elapsed);
  $('runClock').classList.remove('hidden');
  const runningTitle = item.testName || (item.providerName ? `Running ${item.providerName}` : 'Running tests');
  $('testTitle').textContent = runningTitle;
  $('testTitle').title = runningTitle;
  renderWidgetProgress(run);
  renderCompactDetails(run, item);
}

function setCompactState(label, tone) {
  $('widgetStateLabel').textContent = label;
  $('widgetState').className = `widget-state ${tone}`;
}

function compactStateTone(value) {
  const status = String(value || '').toLocaleLowerCase();
  if (['failed', 'stopped', 'cancelled', 'timed_out'].includes(status)) return 'state-danger';
  if (['queued', 'paused', 'skipped', 'unknown'].includes(status)) return 'state-warning';
  return 'state-ready';
}

function updateWidgetActionButton() {
  const btn = $('widgetActionBtn');
  if (!btn) return;
  const playGlyph = btn.querySelector('.play-glyph');
  const pauseGlyph = btn.querySelector('.pause-glyph');
  const resumeGlyph = btn.querySelector('.resume-glyph');

  if (widgetPaused) {
    btn.dataset.tooltip = 'Resume Queue';
    btn.setAttribute('aria-label', 'Resume Queue');
    if (playGlyph) playGlyph.classList.add('hidden');
    if (pauseGlyph) pauseGlyph.classList.add('hidden');
    if (resumeGlyph) resumeGlyph.classList.remove('hidden');
  } else if (runIsActive()) {
    btn.dataset.tooltip = 'Pause Queue';
    btn.setAttribute('aria-label', 'Pause Queue');
    if (playGlyph) playGlyph.classList.add('hidden');
    if (pauseGlyph) pauseGlyph.classList.remove('hidden');
    if (resumeGlyph) resumeGlyph.classList.add('hidden');
  } else {
    btn.dataset.tooltip = 'Run All Tests';
    btn.setAttribute('aria-label', 'Run All Tests');
    if (playGlyph) playGlyph.classList.remove('hidden');
    if (pauseGlyph) pauseGlyph.classList.add('hidden');
    if (resumeGlyph) resumeGlyph.classList.add('hidden');
  }
}

function renderCompactStats(run) {
  $('widgetPassed').textContent = String(run?.passed || 0);
  $('widgetFailed').textContent = String(run?.failed || 0);
}
function renderCompactDetails(run, item) {
  const panel = $('compactDetails');
  panel.classList.toggle('hidden', !compactDetailsOpen);
  if (!compactDetailsOpen) return;
  if (!run || !item) {
    panel.innerHTML = '<div class="empty">No active run details.</div>';
    return;
  }
  panel.innerHTML = `
    <div><strong>${escapeHtml(item.testName || 'Running')}</strong></div>
    <div>${escapeHtml(item.commandPreview || '')}</div>
    <div>${escapeHtml(run.id)}  /  ${run.passed} passed  /  ${run.failed} failed  /  ${run.skipped} skipped</div>
  `;
}

function renderSetup() {
  if (!startupState) return;
  const root = startupState.suggestedProjectRoot || startupState.projectRoot || '';
  if (!$('projectPathInput').value) $('projectPathInput').value = root;
  const providerList = startupState.detectedPreview?.providers || startupState.providers || [];
  renderDetectedProviders(providerList);
  renderProjectList(startupState.projects || []);
  const hintParts = [];
  if (startupState.configured) hintParts.push(`Current project: ${startupState.projectName}`);
  if (startupState.configPath) hintParts.push(startupState.configPath);
  if (startupState.setupError) hintParts.push(`Warning: ${startupState.setupError}`);
  $('setupHint').textContent = hintParts.join('  /  ') || 'TestDesk will create testdesk.json in this folder.';
  $('setupBackBtn').disabled = !startupState.configured;
  if (!startupState.configured && !(startupState.projects || []).length && !addProjectSheetOpen) setAddProjectSheet(true);
}

function renderProjectList(projects) {
  const list = $('projectList');
  if (!projects.length) {
    list.innerHTML = '<div class="empty">No projects added yet.</div>';
    return;
  }
  list.innerHTML = projects.map(project => `
    <div class="project-item ${project.active ? 'active' : ''}">
      <div>
        <strong>${escapeHtml(project.name || project.root)}</strong>
        <small>${escapeHtml(project.root)}${project.configured ? '' : '  /  missing config'}</small>
        <small>${project.providerCount || 0} providers / ${project.testCount || 0} tests / opened ${escapeHtml(formatDate(project.lastOpened))}</small>
      </div>
      <div class="project-actions">
        <button class="button" data-switch-project="${escapeAttr(project.root)}" ${project.configured ? '' : 'disabled'}>${project.active ? 'Active' : 'Switch'}</button>
        <button class="button ghost" data-refresh-project="${escapeAttr(project.root)}" ${project.configured ? '' : 'disabled'}>Refresh</button>
        <button class="button ghost" data-rename-project="${escapeAttr(project.root)}" data-project-name="${escapeAttr(project.name || project.root)}">Rename</button>
        <button class="button ghost" data-forget-project="${escapeAttr(project.root)}">Forget</button>
      </div>
    </div>
  `).join('');
  list.querySelectorAll('[data-switch-project]').forEach(button => button.addEventListener('click', () => switchProject(button.dataset.switchProject)));
  list.querySelectorAll('[data-refresh-project]').forEach(button => button.addEventListener('click', () => refreshProject(button.dataset.refreshProject, button)));
  list.querySelectorAll('[data-rename-project]').forEach(button => button.addEventListener('click', () => renameProject(button.dataset.renameProject, button.dataset.projectName)));
  list.querySelectorAll('[data-forget-project]').forEach(button => button.addEventListener('click', () => forgetProject(button.dataset.forgetProject)));
}

function renderDetectedProviders(providers) {
  $('detectedProviders').innerHTML = providers.length ? providers.map(provider => `
    <span class="chip"><strong>${escapeHtml(provider.name || provider.id)}</strong><small>${escapeHtml(provider.kind || '')}  /  ${escapeHtml(provider.layer || '')}</small></span>
  `).join('') : '<div class="empty">No built-in providers detected yet. You can still add custom providers in testdesk.json.</div>';
}

let lastProjectSelectKey = '';
let lastRunPanelKey = '';
let lastRecentRunsKey = '';

function ensureManagerBounds() {
  if (mode === 'manager' && (window.innerHeight < 200 || window.innerWidth < 400)) {
    call('SetWindowMode', 'manager').catch(console.error);
  }
}

function renderManager() {
  ensureManagerBounds();
  const unavailable = statusUnavailable();
  $('managerStatusUnavailable').classList.toggle('hidden', !unavailable);
  const pName = startupState?.projectName || 'TestDesk';
  if ($('managerTitle').textContent !== pName) $('managerTitle').textContent = pName;
  const pRoot = startupState?.projectRoot || 'Native Go test lifecycle manager';
  if ($('managerSubtitle').textContent !== pRoot) $('managerSubtitle').textContent = pRoot;
  const pCount = String(snapshot?.providerCount ?? 0);
  if ($('providerCount').textContent !== pCount) $('providerCount').textContent = pCount;
  const tCount = String(snapshot?.testCount ?? 0);
  if ($('testCount').textContent !== tCount) $('testCount').textContent = tCount;
  const qCount = unavailable ? '—' : String(snapshot?.queuedCount ?? 0);
  if ($('queueCount').textContent !== qCount) $('queueCount').textContent = qCount;
  const recent = snapshot?.recentRuns || [];
  const lastRes = unavailable ? 'Unavailable' : formatStatus(snapshot?.paused ? 'paused' : (recent[0]?.status || snapshot?.running?.status));
  if ($('lastResult').textContent !== lastRes) $('lastResult').textContent = lastRes;
  if (Array.isArray(snapshot?.tests)) catalogueTests = snapshot.tests;
  renderProjectSelect(startupState?.projects || []);
  renderRunPanel();
  renderRecent(recent);
  updateFilterOptions(catalogueTests);
  renderTests(filterCatalogue(catalogueTests));
  setRunPane(runPane);
  refreshLog().catch(console.error);
}

function renderProjectSelect(projects) {
  const active = startupState?.projectRoot || '';
  const key = `${active}:${projects.map(p => `${p.root}:${p.name}`).join('|')}`;
  if (key === lastProjectSelectKey) return;
  lastProjectSelectKey = key;
  const select = $('projectSelect');
  select.innerHTML = projects.length
    ? projects.map(project => `<option value="${escapeAttr(project.root)}" ${same(project.root, active) ? 'selected' : ''}>${escapeHtml(project.name || project.root)}</option>`).join('')
    : `<option value="${escapeAttr(active)}">${escapeHtml(startupState?.projectName || 'No project')}</option>`;
}

function renderRunPanel() {
  const panel = $('runPanel');
  if (statusUnavailable()) {
    if (lastRunPanelKey !== 'unavailable') {
      lastRunPanelKey = 'unavailable';
      panel.className = 'run-panel empty';
      panel.textContent = 'Status unavailable. Waiting for a status update.';
    }
    return;
  }
  const run = snapshot?.running;
  if (!run) {
    if (lastRunPanelKey === 'empty') return;
    lastRunPanelKey = 'empty';
    panel.className = 'run-panel empty';
    panel.innerHTML = 'No active run.';
    return;
  }
  const item = currentItem();
  const testTitle = item?.testName || (item?.providerName ? `Running ${item.providerName}` : 'Running tests');
  const progress = workItemProgress(run);
  const key = `${run.id}:${run.status}:${run.currentIndex}:${run.passed}:${run.failed}:${run.skipped}:${snapshot?.paused}:${testTitle}:${progress?.completed}`;
  if (key === lastRunPanelKey) return;
  lastRunPanelKey = key;
  panel.className = 'run-panel';
  panel.innerHTML = `
    <div class="widget-heading">
      <span class="eyebrow">${escapeHtml(run.id)}</span>
      <span class="pill ${run.status}">${escapeHtml(formatStatus(run.status))}</span>
    </div>
    <h3 title="${escapeAttr(testTitle)}">${escapeHtml(testTitle)}</h3>
    <div class="compact-stats">
      <span class="pass-stat"><strong>${run.passed}</strong> passed</span>
      <span class="fail-stat"><strong>${run.failed}</strong> failed</span>
      <span class="subtle"><strong>${run.skipped}</strong> skipped</span>
    </div>
    ${progress ? `<div class="progress-shell" role="progressbar" aria-label="Work items finished" aria-valuemin="0" aria-valuemax="${progress.total}" aria-valuenow="${progress.completed}" aria-valuetext="${progress.completed} of ${progress.total} work items finished"><div class="progress" style="width:${progress.percent}%"></div></div><span class="subtle">${progress.completed}/${progress.total} items finished</span>` : '<span class="subtle">Work item progress unavailable</span>'}
    <div class="run-controls-inline">
      <button class="button" data-control="pause" ${snapshot?.paused ? 'disabled' : ''}>Pause</button>
      <button class="button" data-control="resume" ${snapshot?.paused ? '' : 'disabled'}>Resume</button>
      <button class="button ghost" data-control="skip">Skip</button>
      <button class="button danger" data-control="stop">Stop</button>
    </div>
  `;
  panel.querySelectorAll('[data-control]').forEach(button => button.addEventListener('click', async () => {
    const action = button.dataset.control;
    if (action === 'stop' && !await confirmAction('Stop active run?', 'TestDesk will request a safe stop for the active test queue. Captured output and history are retained.', 'Stop Run')) return;
    try {
      await call('Control', action);
      showToast(action === 'stop' ? 'Stop requested.' : `${action[0].toUpperCase()}${action.slice(1)} requested.`);
      await refresh(true);
    } catch (error) { showToast(error.message || String(error)); }
  }));
}

function renderRecent(runs) {
  const key = `${selectedRunId}:${runs.map(r => `${r.id}:${r.status}`).join('|')}`;
  if (key === lastRecentRunsKey) return;
  lastRecentRunsKey = key;
  $('recentRuns').innerHTML = runs.length ? runs.map(run => `
    <button class="recent-item ${selectedRunId === run.id ? 'active' : ''}" data-run-id="${escapeAttr(run.id)}">
      <span>${escapeHtml(run.id)}</span>
      <span class="pill ${run.status}">${escapeHtml(formatStatus(run.status))}</span>
    </button>
  `).join('') : '<div class="empty">No run history yet.</div>';
  $('recentRuns').querySelectorAll('[data-run-id]').forEach(button => button.addEventListener('click', async () => {
    selectedRunId = button.dataset.runId;
    lastRecentRunsKey = '';
    renderRecent(snapshot?.recentRuns || []);
    await refreshLog(true);
    setRunPane('active');
  }));
}

function updateFilterOptions(tests) {
  const configuredProviders = (startupState?.providers || []).map(p => [p.id, p.name || p.id]);
  const discoveredProviders = (tests || []).map(test => [test.providerId, test.providerName || test.providerId]);
  const combined = new Map([...configuredProviders, ...discoveredProviders].filter(([id]) => Boolean(id)));
  const providers = [...combined.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  const modules = [...new Set((tests || []).map(test => test.module).filter(Boolean))].sort();
  const key = JSON.stringify({
    providers,
    modules
  });
  if (key === filterOptionsKey) return;
  filterOptionsKey = key;
  const providerSelect = $('providerFilter');
  const providerCurrent = providerSelect.value;
  providerSelect.innerHTML = '<option value="">All Providers</option>' + providers.map(([id, name]) => `<option value="${escapeAttr(id)}">${escapeHtml(name)}</option>`).join('');
  providerSelect.value = providers.some(([id]) => id === providerCurrent) ? providerCurrent : '';

  const moduleSelect = $('moduleFilter');
  const moduleCurrent = moduleSelect.value;
  moduleSelect.innerHTML = '<option value="">All Modules</option>' + modules.map(module => `<option value="${escapeAttr(module)}">${escapeHtml(module)}</option>`).join('');
  moduleSelect.value = modules.includes(moduleCurrent) ? moduleCurrent : '';
}

function renderTests(tests) {
  lastTests = tests;
  filteredTests = tests;
  $('filteredCount').textContent = String(tests.length);
  $('selectedCount').textContent = `${selected.size} selected`;
  $('runSelectedBtn').disabled = selected.size === 0;
  $('clearSelectionBtn').disabled = selected.size === 0;
  $('selectFilteredBtn').disabled = tests.length === 0;
  $('runFilteredBtn').disabled = tests.length === 0;
  const visibleTests = tests.slice(0, visibleTestLimit);
  const renderKey = JSON.stringify({
    tests: visibleTests.map(test => [test.id, test.lastStatus, test.lastRunId, test.lastDurationMillis, test.lastFailure]),
    total: tests.length,
    visible: visibleTests.length,
    expanded: [...expandedTests].sort(),
    selected: [...selected].sort(),
    collapsed: [...collapsedProviders].sort()
  });
  if (renderKey === lastTestsRenderKey) return;
  lastTestsRenderKey = renderKey;

  const groups = new Map();
  for (const test of visibleTests) {
    const key = test.providerId || 'unknown';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(test);
  }
  for (const test of tests) {
    const key = test.providerId || 'unknown';
    if (!groups.has(key)) groups.set(key, []);
  }

  if (!tests.length) {
    $('testGroups').innerHTML = '<div class="empty">No tests found. Run discovery or loosen the filters.</div>';
    return;
  }
  const providerTotals = new Map();
  for (const test of tests) providerTotals.set(test.providerId || 'unknown', (providerTotals.get(test.providerId || 'unknown') || 0) + 1);

  const providerEntries = [...groups.entries()].sort((a, b) => {
    const nameA = a[1][0]?.providerName || (startupState?.providers || []).find(p => p.id === a[0])?.name || a[0];
    const nameB = b[1][0]?.providerName || (startupState?.providers || []).find(p => p.id === b[0])?.name || b[0];
    return nameA.localeCompare(nameB);
  });

  $('testGroups').innerHTML = providerEntries.map(([provider, items]) => {
    const isCollapsed = collapsedProviders.has(provider);
    const providerName = items[0]?.providerName || (startupState?.providers || []).find(p => p.id === provider)?.name || provider;
    const totalCount = providerTotals.get(provider) || items.length;
    return `
      <section class="provider-group ${isCollapsed ? 'collapsed' : ''}" data-provider-id="${escapeAttr(provider)}">
        <div class="provider-title" data-provider-header="${escapeAttr(provider)}">
          <div class="provider-title-heading">
            <span class="provider-toggle-icon" aria-hidden="true">▼</span>
            <strong>${escapeHtml(providerName)}</strong>
            <span class="subtle">${totalCount} test${totalCount === 1 ? '' : 's'}</span>
          </div>
          <button class="button ghost" data-run-provider="${escapeAttr(provider)}">Run Provider</button>
        </div>
        ${items.map(testRow).join('')}
      </section>
    `;
  }).join('') + (visibleTests.length < tests.length ? `
    <div class="catalogue-more">
      <span>Showing ${visibleTests.length} of ${tests.length} matching tests</span>
      <button class="button ghost" data-load-more-tests>Show ${Math.min(CATALOGUE_BATCH_SIZE, tests.length - visibleTests.length)} More</button>
      <button class="button" data-show-all-tests>Show All</button>
    </div>` : '');

  $('testGroups').querySelectorAll('[data-provider-header]').forEach(header => {
    header.addEventListener('click', (event) => {
      if (event.target.closest('button,input,a,label,select')) return;
      const provider = header.dataset.providerHeader;
      if (collapsedProviders.has(provider)) {
        collapsedProviders.delete(provider);
      } else {
        collapsedProviders.add(provider);
      }
      lastTestsRenderKey = '';
      renderTests(filteredTests);
    });
  });

  const toggleAllBtn = $('toggleAllProvidersBtn');
  if (toggleAllBtn) {
    const allProviders = providerEntries.map(e => e[0]);
    const allCollapsed = allProviders.length > 0 && allProviders.every(p => collapsedProviders.has(p));
    toggleAllBtn.textContent = allCollapsed ? 'Expand All' : 'Collapse All';
  }

  document.querySelectorAll('[data-test-check]').forEach(input => {
    input.addEventListener('change', (event) => {
      const id = event.currentTarget.dataset.testCheck;
      event.currentTarget.checked ? selected.add(id) : selected.delete(id);
      $('selectedCount').textContent = `${selected.size} selected`;
      $('runSelectedBtn').disabled = selected.size === 0;
      $('clearSelectionBtn').disabled = selected.size === 0;
    });
  });
  document.querySelectorAll('[data-test-row]').forEach(row => {
    row.addEventListener('click', (event) => {
      if (event.target.closest('button,input,a,label,select')) return;
      toggleTestDetails(event.currentTarget.dataset.testRow);
    });
  });
  document.querySelectorAll('[data-test-toggle]').forEach(button => {
    button.addEventListener('click', (event) => {
      toggleTestDetails(event.currentTarget.dataset.testToggle);
    });
  });
  document.querySelectorAll('[data-run-test]').forEach(button => {
    button.addEventListener('click', async (event) => {
      await runRequest({ testIds: [event.currentTarget.dataset.runTest] });
    });
  });
  document.querySelectorAll('[data-copy-test]').forEach(button => {
    button.addEventListener('click', async (event) => {
      const test = lastTests.find(t => t.id === event.currentTarget.dataset.copyTest);
      await copyText(JSON.stringify(test, null, 2));
    });
  });
  document.querySelectorAll('[data-run-provider]').forEach(button => {
    button.addEventListener('click', async (event) => {
      await runRequest({ providerIds: [event.currentTarget.dataset.runProvider] });
    });
  });
  $('testGroups').querySelectorAll('[data-run-id]').forEach(button => {
    button.addEventListener('click', async (event) => {
      selectedRunId = event.currentTarget.dataset.runId;
      await refreshLog(true);
    });
  });
  $('testGroups').querySelector('[data-load-more-tests]')?.addEventListener('click', () => {
    visibleTestLimit += CATALOGUE_BATCH_SIZE;
    lastTestsRenderKey = '';
    renderTests(filteredTests);
  });
  $('testGroups').querySelector('[data-show-all-tests]')?.addEventListener('click', () => {
    visibleTestLimit = filteredTests.length;
    lastTestsRenderKey = '';
    renderTests(filteredTests);
  });
}

function setRunPane(next) {
  runPane = next === 'recent' ? 'recent' : 'active';
  const active = runPane === 'active';
  $('activeRunTabBtn').classList.toggle('active', active);
  $('activeRunTabBtn').setAttribute('aria-selected', String(active));
  $('recentRunsTabBtn').classList.toggle('active', !active);
  $('recentRunsTabBtn').setAttribute('aria-selected', String(!active));
  $('activeRunPane').classList.toggle('hidden', !active);
  $('activeRunPane').classList.toggle('active', active);
  $('recentRunsPane').classList.toggle('hidden', active);
  $('recentRunsPane').classList.toggle('active', !active);
}

function filterCatalogue(tests) {
  const criteria = filter();
  const query = criteria.query.trim().toLocaleLowerCase();
  const tags = criteria.tags.map(tag => tag.toLocaleLowerCase());
  return tests.filter(test => {
    if (criteria.providerIds.length && !criteria.providerIds.includes(test.providerId)) return false;
    if (criteria.layers.length && !criteria.layers.includes(test.layer)) return false;
    if (criteria.modules.length && !criteria.modules.includes(test.module)) return false;
    if (criteria.statuses.length && !criteria.statuses.includes(test.lastStatus || 'unknown')) return false;
    const testTags = (test.tags || []).map(tag => String(tag).toLocaleLowerCase());
    if (tags.length && !tags.every(tag => testTags.includes(tag))) return false;
    if (!query) return true;
    return [test.name, test.fullName, test.selector, test.providerName, test.module, test.layer, ...(test.tags || [])]
      .some(value => String(value || '').toLocaleLowerCase().includes(query));
  });
}

function showCatalogueLoader(label) {
  $('catalogLoaderLabel').textContent = label || 'Loading tests…';
  $('catalogLoader').classList.remove('hidden');
  $('testGroups').setAttribute('aria-busy', 'true');
}

function hideCatalogueLoader() {
  $('catalogLoader').classList.add('hidden');
  $('testGroups').setAttribute('aria-busy', 'false');
}

function scheduleFilterRender() {
  showCatalogueLoader('Filtering tests…');
  visibleTestLimit = CATALOGUE_BATCH_SIZE;
  if (filterRenderFrame) cancelAnimationFrame(filterRenderFrame);
  filterRenderFrame = requestAnimationFrame(() => {
    filterRenderFrame = requestAnimationFrame(() => {
      filterRenderFrame = 0;
      lastTestsRenderKey = '';
      renderTests(filterCatalogue(catalogueTests));
      hideCatalogueLoader();
    });
  });
}

function toggleTestDetails(id) {
  if (!id) return;
  expandedTests.has(id) ? expandedTests.delete(id) : expandedTests.add(id);
  renderTests(lastTests);
}

function testRow(test) {
  const tags = (test.tags || []).slice(0, 4).join(', ');
  const expanded = expandedTests.has(test.id);
  const status = test.lastStatus || 'unknown';
  const statusCell = status === 'unknown'
    ? '<span class="status-placeholder" aria-label="No result recorded">—</span>'
    : `<button class="button pill ${escapeAttr(status)}" data-test-toggle="${escapeAttr(test.id)}">${escapeHtml(formatStatus(status))}</button>`;
  return `
    <article class="test-card ${expanded ? 'expanded' : ''}">
      <div class="test-row" data-test-row="${escapeAttr(test.id)}">
        <input class="check" type="checkbox" data-test-check="${escapeAttr(test.id)}" ${selected.has(test.id) ? 'checked' : ''}/>
        ${statusCell}
        <span class="meta">${escapeHtml(test.layer || '--')}</span>
        <button class="name-cell" data-test-toggle="${escapeAttr(test.id)}"><strong>${escapeHtml(test.name || test.fullName)}</strong><small>${escapeHtml(test.fullName || test.selector || '')}</small></button>
        <span class="meta">${escapeHtml(test.module || 'general')}</span>
        <span class="meta">${escapeHtml(tags || formatMs(test.lastDurationMillis))}</span>
      </div>
      ${expanded ? testDetails(test) : ''}
    </article>`;
}

function testDetails(test) {
  return `
    <div class="test-details">
      <div class="detail-grid">
        <div><span>Provider</span><strong>${escapeHtml(test.providerName || test.providerId)}</strong></div>
        <div><span>Kind</span><strong>${escapeHtml(test.providerKind || 'custom')}</strong></div>
        <div><span>Module</span><strong>${escapeHtml(test.module || 'general')}</strong></div>
        <div><span>Last duration</span><strong>${formatMs(test.lastDurationMillis)}</strong></div>
        <div><span>Selector</span><code>${escapeHtml(test.selector || '--')}</code></div>
        <div><span>File</span><code>${escapeHtml(test.file || '--')}${test.line ? ':' + test.line : ''}</code></div>
      </div>
      ${test.lastFailure ? `<pre class="failure">${escapeHtml(test.lastFailure)}</pre>` : ''}
      <div class="detail-actions">
        <button class="button primary" data-run-test="${escapeAttr(test.id)}">Run This Test</button>
        <button class="button" data-copy-test="${escapeAttr(test.id)}">Copy Test Metadata</button>
        ${test.lastRunId ? `<button class="button" data-run-id="${escapeAttr(test.lastRunId)}">View Last Output</button>` : ''}
      </div>
    </div>`;
}

async function renderSettings() {
  const ui = startupState?.ui || {};
  if ($('widgetSizeSelect')) $('widgetSizeSelect').value = ui.widgetSize || 'medium';
  $('widgetModeSelect').value = ui.widgetMode || 'when_active';
  $('topmostModeSelect').value = ui.topmostMode || (ui.alwaysOnTop === false ? 'off' : 'always');
  if ($('logRetentionSelect')) $('logRetentionSelect').value = String(ui.logRetentionDays ?? 7);
  $('settingsProjectName').textContent = startupState?.projectName || 'No active project';
  $('settingsProjectPath').textContent = startupState?.projectRoot || '—';
  $('settingsConfigPath').textContent = startupState?.configPath || '—';
  $('manualTargetSelect').innerHTML = '<option value="">Loading...</option>';
  $('mcpTargets').innerHTML = '<div class="empty">Detecting local agent workspaces...</div>';
  try {
    mcpTargets = await call('MCPWorkspaces');
  } catch (error) {
    $('mcpTargets').innerHTML = `<div class="empty">${escapeHtml(error.message || String(error))}</div>`;
    return;
  }
  if (!mcpSelectedTarget && mcpTargets.length) mcpSelectedTarget = String(mcpTargets[0].id || '');
  renderMCPTargets();
  renderManualMCP();
}

function setSettingsPane(next) {
  settingsPane = next === 'agents' ? 'agents' : 'general';
  const generalActive = settingsPane === 'general';
  $('generalSettingsPane').classList.toggle('active', generalActive);
  $('agentSettingsPane').classList.toggle('active', !generalActive);
  $('generalSettingsTabBtn').classList.toggle('active', generalActive);
  $('agentSettingsTabBtn').classList.toggle('active', !generalActive);
  $('generalSettingsTabBtn').setAttribute('aria-selected', String(generalActive));
  $('agentSettingsTabBtn').setAttribute('aria-selected', String(!generalActive));
}

function renderMCPTargets() {
  const workspaceTargets = mcpTargets.filter(target => !target.manualOnly && target.id !== 'universal' && target.id !== 'cli');
  const installed = workspaceTargets.filter(target => target.installed).length;
  const detected = workspaceTargets.filter(target => target.detected).length;
  if ($('mcpCounts')) {
    $('mcpCounts').textContent = `${installed} enabled configuration${installed === 1 ? '' : 's'} · ${detected} detected CLI/workspace${detected === 1 ? '' : 's'}`;
  }
  if ($('mcpActiveProject')) {
    $('mcpActiveProject').textContent = `Active project: ${startupState?.projectName || 'none'} • Runtime usage remains controlled by each agent client.`;
  }
  $('mcpInstallStatus').textContent = mcpInstallStatus;
  $('mcpTargets').innerHTML = workspaceTargets.length ? workspaceTargets.map(target => `
    <article class="mcp-target ${target.installed ? 'installed' : ''} ${target.detected ? 'detected' : ''}">
      <div class="mcp-main">
        <div>
          <strong>${escapeHtml(target.name)}</strong>
          <small>${escapeHtml(target.description)}</small>
        </div>
        <span class="pill ${target.installed ? 'passed' : (target.detected ? 'queued' : 'unknown')}">${target.installed ? 'installed' : (target.detected ? 'detected' : 'available')}</span>
      </div>
      <div class="mcp-meta"><span>${escapeHtml(target.scope)}</span><code>${escapeHtml(target.configPath)}</code></div>
      ${(target.notes || []).length ? `<div class="hint">${escapeHtml(target.notes.join('  /  '))}</div>` : ''}
      <div class="detail-actions">
        <button class="button primary" data-install-mcp="${escapeAttr(target.id)}">${target.installed ? 'Reinstall / Update' : 'One-Click Install'}</button>
        <button class="button" data-manual-mcp="${escapeAttr(target.id)}">Manual Config</button>
      </div>
    </article>`).join('') : '<div class="empty">No MCP targets are available yet.</div>';

  $('manualTargetSelect').innerHTML = mcpTargets.map(target => `<option value="${escapeAttr(target.id)}" ${target.id === mcpSelectedTarget ? 'selected' : ''}>${escapeHtml(target.name)}</option>`).join('');
  $('mcpTargets').querySelectorAll('[data-install-mcp]').forEach(button => button.addEventListener('click', async () => {
    const original = button.textContent;
    button.textContent = 'Installing...';
    button.disabled = true;
    try {
      const updated = await call('InstallMCP', button.dataset.installMcp);
      mcpTargets = mcpTargets.map(target => target.id === updated.id ? updated : target);
      mcpSelectedTarget = updated.id;
      mcpInstallStatus = updated.backupPath ? `Installed ${updated.name}. Backup created: ${updated.backupPath}` : `Installed ${updated.name}. No prior config needed a backup.`;
      renderMCPTargets();
      renderManualMCP();
    } catch (error) {
      button.textContent = error.message || 'Failed';
      setTimeout(() => { button.textContent = original; button.disabled = false; }, 1800);
    }
  }));
  $('mcpTargets').querySelectorAll('[data-manual-mcp]').forEach(button => button.addEventListener('click', () => {
    mcpSelectedTarget = button.dataset.manualMcp;
    renderManualMCP();
  }));
}

function renderManualMCP() {
  const target = mcpTargets.find(item => item.id === mcpSelectedTarget) || mcpTargets[0];
  if (!target) {
    $('mcpManualConfig').textContent = 'No MCP target selected.';
    return;
  }
  mcpSelectedTarget = target.id;
  $('manualTargetSelect').value = target.id;
  $('mcpManualConfig').textContent = target.manual || '';
}

function selectedMCPCommand() {
  const target = mcpTargets.find(item => item.id === mcpSelectedTarget) || mcpTargets[0];
  return target?.commandPreview?.length ? target.commandPreview.map(part => /\s/.test(part) ? `"${part}"` : part).join(' ') : '';
}

function filter() {
  const statuses = $('statusFilter').value ? [$('statusFilter').value] : [];
  return {
    providerIds: $('providerFilter').value ? [$('providerFilter').value] : [],
    layers: $('layerFilter').value ? [$('layerFilter').value] : [],
    modules: $('moduleFilter').value ? [$('moduleFilter').value] : [],
    statuses,
    tags: splitCSV($('tagInput').value),
    query: $('queryInput').value || '',
    limit: 10000
  };
}

function splitCSV(value) {
  return String(value || '').split(',').map(part => part.trim()).filter(Boolean);
}

function formatLog(raw) {
  if (!raw || typeof raw !== 'string') return 'No captured output yet.';
  const lines = raw.split('\n');
  const formatted = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
      try {
        const obj = JSON.parse(trimmed);
        if (obj.Output !== undefined) {
          const out = obj.Output.replace(/\r?\n$/, '');
          if (out.trim()) formatted.push(out);
          continue;
        } else if (obj.Action && obj.Test) {
          if (obj.Action === 'pass' || obj.Action === 'fail' || obj.Action === 'skip') {
            formatted.push(`[${obj.Action.toUpperCase()}] ${obj.Test}`);
            continue;
          }
        }
      } catch (_) {}
    }
    formatted.push(line);
  }
  return formatted.length > 0 ? formatted.join('\n') : 'No captured output yet.';
}

async function refresh(includeTests = false) {
  if (refreshInFlight || frontendLoadingOperations.has('switch-project')) return;
  refreshInFlight = true;
  const generation = projectSwitchGeneration;
  const shouldFetchTests = includeTests && catalogueTests.length === 0;
  if (shouldFetchTests && mode === 'manager') showCatalogueLoader('Fetching tests…');
  try {
    if (!startupState?.configured) {
      renderPlayer();
      return;
    }
    if (mode === 'manager' && includeTests) void refreshStartup(generation, startupState);
    void refreshActiveProjects(generation);
    const nextSnapshot = await call('Snapshot', shouldFetchTests ? { limit: 10000 } : { limit: 0 });
    if (generation !== projectSwitchGeneration) return;
    if (!shouldFetchTests && catalogueTests.length) nextSnapshot.tests = catalogueTests;
    acceptSnapshot(nextSnapshot);
    renderCompactProjects();
    renderPlayer();
    if (mode === 'manager') renderManager();
  } catch (error) {
    console.error(error);
    if (generation === projectSwitchGeneration) {
      statusRefreshFailed = true;
      renderPlayer();
    }
  } finally {
    refreshInFlight = false;
    if (shouldFetchTests && mode === 'manager') hideCatalogueLoader();
  }
}

async function refreshStartup(generation, previousState) {
  if (startupRefreshInFlight) return;
  startupRefreshInFlight = true;
  try {
    const nextStartup = await call('Startup');
    if (generation !== projectSwitchGeneration || startupState !== previousState || !nextStartup) return;
    startupState = nextStartup;
    renderCompactProjects();
    renderPlayer();
    if (mode === 'manager') renderManager();
  } catch (error) {
    console.error(error);
  } finally {
    startupRefreshInFlight = false;
  }
}

async function refreshActiveProjects(generation) {
  if (activeProjectsInFlight) return;
  activeProjectsInFlight = true;
  try {
    const projects = await call('ActiveProjects');
    if (generation !== projectSwitchGeneration) return;
    if (!Array.isArray(projects)) throw new Error('Invalid active-project response');
    activeProjects = projects;
    lastActiveProjectsUpdate = Date.now();
    activeProjectsRefreshFailed = false;
    renderCompactProjects();
  } catch (error) {
    console.error(error);
    if (generation === projectSwitchGeneration) {
      activeProjectsRefreshFailed = true;
      renderCompactProjects();
    }
  } finally {
    activeProjectsInFlight = false;
  }
}

function renderCompactProjects() {
  const select = $('compactProjectSelect');
  const unavailable = Boolean(startupState?.configured) && (activeProjectsRefreshFailed || Date.now() - lastActiveProjectsUpdate > 6000);
  $('widgetProjectsStatus').classList.toggle('hidden', !unavailable);
  const runningProjects = activeProjects.filter(project => project.active);
  const multiple = runningProjects.length > 1;
  select.classList.toggle('hidden', !multiple);
  $('providerLabel').classList.toggle('hidden', multiple);
  if (!multiple) return;
  const current = startupState?.projectRoot || '';
  const key = JSON.stringify([current, runningProjects, unavailable]);
  if (select.dataset.projectsKey !== key) {
    select.dataset.projectsKey = key;
    select.innerHTML = runningProjects.map(project =>
      `<option value="${escapeAttr(project.root)}" ${same(project.root, current) ? 'selected' : ''}>${escapeHtml(project.name || project.root)}${unavailable ? ' • unknown' : (project.active ? ' • active' : ' • idle')}</option>`
    ).join('');
  }
}

function debounce(fn, ms) {
  let timeout;
  return (...args) => { clearTimeout(timeout); timeout = setTimeout(() => fn(...args), ms); };
}

async function refreshLog(force = false) {
  if (logRefreshInFlight) return;
  if (mode !== 'manager' && !force) return;
  const runId = activeRunId();
  if (!runId) {
    if ($('logOutput').textContent !== 'No output yet.' && !lastDiagnostics?.log) {
      $('logOutput').textContent = 'No output yet.';
    }
    return;
  }
  logRefreshInFlight = true;
  try {
    lastDiagnostics = await call('Diagnostics', runId, 48000);
    const log = lastDiagnostics?.log || '';
    const output = $('logOutput');
    const followingTail = output.scrollTop + output.clientHeight >= output.scrollHeight - 12;
    output.textContent = formatLog(log);
    if (followingTail) output.scrollTop = output.scrollHeight;
  } catch (error) {
    $('logOutput').textContent = error.message || String(error);
  } finally {
    logRefreshInFlight = false;
  }
}

async function copyDiagnostics() {
  const runId = activeRunId();
  const diag = await call('Diagnostics', runId, 64000);
  const text = JSON.stringify(diag, null, 2);
  await copyText(text);
  flashButton('copyDiagBtn', 'Copied');
  showToast('Diagnostics copied to the clipboard.');
}

async function copyOutput() {
  if (!lastDiagnostics) await refreshLog(true);
  await copyText(lastDiagnostics?.log || $('logOutput').textContent || '');
  flashButton('copyOutputBtn', 'Copied');
  showToast('Output copied to the clipboard.');
}

async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const area = document.createElement('textarea');
  area.value = text;
  document.body.appendChild(area);
  area.select();
  document.execCommand('copy');
  area.remove();
}

function flashButton(id, label) {
  const button = $(id);
  const original = button.textContent;
  button.textContent = label;
  setTimeout(() => { button.textContent = original; }, 1100);
}

async function previewProject() {
  const root = $('projectPathInput').value.trim();
  if (!root) return;
  const btn = $('previewProjectBtn');
  const original = btn ? btn.textContent : '';
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Scanning...';
  }
  $('setupHint').textContent = 'Scanning repository for test frameworks…';
  $('detectedProviders').innerHTML = '<div class="chip"><strong>Scanning…</strong><small>Checking frameworks</small></div>';
  try {
    const cfg = await call('PreviewProject', root);
    startupState = { ...(startupState || {}), suggestedProjectRoot: root, detectedPreview: cfg, ui: cfg.ui || startupState?.ui };
    renderSetup();
    $('setupHint').textContent = (cfg.providers || []).length
      ? `Found ${cfg.providers.length} test framework${cfg.providers.length === 1 ? '' : 's'}. Ready to add project.`
      : 'No common test frameworks detected automatically. You can still add the project and configure custom providers.';
  } catch (error) {
    $('setupHint').textContent = error.message || String(error);
    $('detectedProviders').innerHTML = '';
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = original || 'Preview Detection';
    }
  }
}

async function chooseProject() {
  const chosen = await call('ChooseProject');
  if (!chosen) return;
  $('projectPathInput').value = chosen;
  await previewProject();
}

async function setupProject() {
	if (setupBusy) return;
	setupBusy = true;
	const returnMode = mode;
	frontendLoadingOperations.add('setup-project');
	$('setupProjectBtn').textContent = 'Setting Up...';
	try {
    const response = await call('SetupProject', {
      projectRoot: $('projectPathInput').value.trim(),
      force: false,
      autoDiscover: $('setupAutoDiscover').checked,
      widgetMode: startupState?.ui?.widgetMode || 'when_active',
      topmostMode: startupState?.ui?.topmostMode || 'always'
    });
    startupState = await call('Startup');
		acceptSnapshot(response.snapshot);
		if (response.discoverWarning) $('setupHint').textContent = `Setup complete with warning: ${response.discoverWarning}`;
		leaveLoading('manager');
		await call('SetWindowMode', 'manager');
		setAddProjectSheet(false);
		renderManager();
	} catch (error) {
		leaveLoading(returnMode);
		$('setupHint').textContent = error.message || String(error);
  } finally {
	frontendLoadingOperations.delete('setup-project');
    setupBusy = false;
    $('setupProjectBtn').textContent = 'Add Project';
  }
}

async function switchProject(root) {
	if (!root || same(root, startupState?.projectRoot)) return;
	projectSwitchGeneration++;
	const returnMode = mode;
	frontendLoadingOperations.add('switch-project');
	presentLoading({
		active: true,
		operation: 'switch-project',
		title: 'Loading project workspace',
		detail: 'Reading providers, recent run history and catalog status…'
	});
	applyMode('loading');
	try {
		const response = await call('SwitchProject', root, returnMode === 'manager');
		startupState = response.startup;
		acceptSnapshot(response.snapshot);
		catalogueTests = [];
		collapsedProviders.clear();
		filterOptionsKey = '';
		selected.clear();
		expandedTests.clear();
		selectedRunId = '';
		lastTestsRenderKey = '';
		leaveLoading(returnMode);
		if (returnMode === 'setup') renderSetup();
		if (returnMode === 'manager') renderManager();
		renderPlayer();
		renderCompactProjects();
	} catch (error) {
		leaveLoading(returnMode);
		throw error;
	} finally {
		frontendLoadingOperations.delete('switch-project');
	}
}

async function refreshProject(root, button) {
	const returnMode = mode;
	frontendLoadingOperations.add('refresh-project');
	const original = button.textContent;
  button.disabled = true;
  button.textContent = 'Refreshing...';
	presentLoading({
		active: true,
		operation: 'refresh-project',
		title: 'Refreshing project workspace',
		detail: 'Scanning workspace files and test definitions…'
	});
	applyMode('loading');
	try {
		const response = await call('RefreshProject', root, returnMode === 'manager');
		startupState = response.startup;
		acceptSnapshot(response.snapshot);
		catalogueTests = [];
		collapsedProviders.clear();
		filterOptionsKey = '';
		leaveLoading(returnMode);
		renderSetup(startupState.detectedPreview?.providers || startupState.providers || []);
		if (returnMode === 'manager') renderManager();
	} finally {
		frontendLoadingOperations.delete('refresh-project');
		if (mode === 'loading') leaveLoading(returnMode);
    button.disabled = false;
    button.textContent = original;
  }
}
async function renameProject(root, currentName) {
  const name = await requestProjectName(currentName);
  if (name === null || !name.trim()) return;
  const trimmed = name.trim();

  // Optimistic UI update: instantly reflect name across setup, manager, player & dropdown
  if (startupState) {
    if (same(startupState.projectRoot, root)) {
      startupState.projectName = trimmed;
    }
    if (Array.isArray(startupState.projects)) {
      for (const project of startupState.projects) {
        if (same(project.root, root)) {
          project.name = trimmed;
        }
      }
    }
    renderSetup();
    renderProjectSelect(startupState.projects || []);
    renderManager();
    renderPlayer();
  }

  try {
    const updated = await call('RenameProject', root, trimmed);
    if (updated) {
      startupState = updated;
      renderSetup();
      renderProjectSelect(startupState.projects || []);
      renderManager();
      renderPlayer();
    }
    showToast('Project name saved.');
  } catch (error) {
    showToast(error.message || String(error));
  }
}
async function forgetProject(root) {
  const project = (startupState?.projects || []).find(item => same(item.root, root));
  if (!await confirmAction('Forget project?', `Remove ${project?.name || root} from TestDesk’s known projects? This does not delete repository files or testdesk.json.`, 'Forget Project')) return;

  // Optimistic UI update: remove from local state immediately (0ms visual latency)
  if (startupState?.projects) {
    startupState.projects = startupState.projects.filter(item => !same(item.root, root));
    renderSetup();
    if (mode === 'manager') renderProjectSelect(startupState.projects);
  }

  try {
    startupState = await call('ForgetProject', root);
    selected.clear();
    expandedTests.clear();
    selectedRunId = '';
    lastTestsRenderKey = '';
    renderSetup();
    if (mode === 'manager') renderProjectSelect(startupState.projects || []);
    showToast('Project removed from TestDesk. Repository files were unchanged.');
  } catch (error) {
    showToast(error.message || String(error));
    await refreshProjects();
  }
}

async function refreshProjects() {
  startupState = await call('Startup');
  renderSetup();
  if (mode === 'manager') renderProjectSelect(startupState.projects || []);
}

async function saveUI() {
  const retentionDays = Number.parseInt($('logRetentionSelect')?.value || '7', 10);
  const widgetSize = $('widgetSizeSelect')?.value || 'medium';
  const ui = await call('SaveUI', $('widgetModeSelect').value, $('topmostModeSelect').value, retentionDays, widgetSize);
  startupState = { ...(startupState || {}), ui };
  await call('ApplyWidgetVisibility', runIsActive()).catch(() => {});
  applyMode(mode);
  $('widgetSettingsStatus').textContent = 'Preferences saved.';
  showToast('Preferences saved.');
}

async function resetWidgetPosition() {
  await call('ResetWidgetPosition');
  $('widgetSettingsStatus').textContent = 'Compact controller reset to the bottom-left of the current display.';
  showToast('Compact controller position reset.');
}

function setAddProjectSheet(open) {
  addProjectSheetOpen = Boolean(open);
  $('addProjectSheet').classList.toggle('hidden', !addProjectSheetOpen);
  $('addProjectBackdrop').classList.toggle('hidden', !addProjectSheetOpen);
  document.body.classList.toggle('sheet-open', addProjectSheetOpen);
  if (addProjectSheetOpen) {
    requestAnimationFrame(() => $('projectPathInput').focus());
  } else {
    $('openAddProjectBtn').focus();
  }
}

async function runRequest(request) {
  if (mode === 'manager') {
    runInitiatedFromManager = true;
  }
  const run = await call('RunTests', request);
  selectedRunId = run.id || '';
  selected.clear();
  await refresh(false);
  await refreshLog(true);
  if (startupState?.ui?.widgetMode !== 'off') setMode('compact');
}

async function discover() {
	const returnMode = mode;
	frontendLoadingOperations.add('discover');
	$('discoverBtn').textContent = 'Refreshing...';
	presentLoading({
		active: true,
		operation: 'discover',
		title: 'Discovering test catalog',
		detail: 'Enumerating test cases across configured providers…'
	});
	applyMode('loading');
	try {
		const response = await call('Discover', true);
		startupState = response.startup;
		acceptSnapshot(response.snapshot);
		catalogueTests = snapshot?.tests || [];
		visibleTestLimit = CATALOGUE_BATCH_SIZE;
		lastTestsRenderKey = '';
		leaveLoading(returnMode);
		renderManager();
	} finally {
		frontendLoadingOperations.delete('discover');
		if (mode === 'loading') leaveLoading(returnMode);
	$('discoverBtn').textContent = 'Refresh Discovery';
  }
}

function selectAllFiltered() {
  lastTests.forEach(test => selected.add(test.id));
  renderTests(lastTests);
}

function clearSelection() {
  selected.clear();
  renderTests(lastTests);
}

function same(a, b) {
  return String(a || '').toLowerCase() === String(b || '').toLowerCase();
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>'"]/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
}

function escapeAttr(value) {
  return escapeHtml(value).replace(/`/g, '&#96;');
}

async function init() {
	frontendLoadingOperations.add('startup');
	if (window.runtime?.EventsOn) {
		window.runtime.EventsOn('testdesk:loading', presentLoading);
		window.runtime.EventsOn('testdesk:window-mode', (next) => {
			if (typeof next === 'string' && next) applyMode(next);
		});
	}
	try {
		const currentLoading = await call('LoadingState');
		if (currentLoading?.active) presentLoading(currentLoading);
		const response = await call('Initialise', true);
		startupState = response.startup;
		acceptSnapshot(response.snapshot);
	} catch (error) {
		startupState = await call('Startup');
		presentLoading({ active: false, operation: 'startup', title: 'TestDesk needs attention', detail: error.message || String(error) });
	} finally {
		frontendLoadingOperations.delete('startup');
	}
	$('projectPathInput').value = startupState.suggestedProjectRoot || startupState.projectRoot || '';
	initInspectorResizer();
	if (!startupState.configured) {
		await setMode('setup');
	} else {
		renderManager();
		renderPlayer();
		if (startupState.ui?.startMinimized) {
			await call('SetWindowMode', 'manager_hidden');
			applyMode('manager');
		} else {
			await setMode('manager');
		}
	}
}

function initInspectorResizer() {
  const resizer = $('inspectorResizer');
  if (!resizer) return;
  const savedWidth = localStorage.getItem('testdesk_runs_width');
  if (savedWidth) {
    const parsed = Number.parseInt(savedWidth, 10);
    if (!Number.isNaN(parsed) && parsed >= 320) {
      document.documentElement.style.setProperty('--runs-width', `${parsed}px`);
    }
  }

  let isDragging = false;
  let startX = 0;
  let startWidth = 0;

  resizer.addEventListener('mousedown', (e) => {
    isDragging = true;
    startX = e.clientX;
    const runsEl = document.querySelector('.runs');
    startWidth = runsEl ? runsEl.getBoundingClientRect().width : 360;
    resizer.classList.add('dragging');
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  });

  document.addEventListener('mousemove', (e) => {
    if (!isDragging) return;
    const delta = e.clientX - startX;
    const newWidth = Math.max(320, Math.min(700, startWidth + delta));
    document.documentElement.style.setProperty('--runs-width', `${newWidth}px`);
  });

  document.addEventListener('mouseup', () => {
    if (!isDragging) return;
    isDragging = false;
    resizer.classList.remove('dragging');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    const currentWidth = getComputedStyle(document.documentElement).getPropertyValue('--runs-width');
    if (currentWidth) {
      localStorage.setItem('testdesk_runs_width', String(Number.parseInt(currentWidth, 10)));
    }
  });
}

let widgetPaused = false;
$('widgetActionBtn')?.addEventListener('click', async () => {
  if (widgetPaused) {
    const action = 'resume';
    try {
      await call('Control', action);
      widgetPaused = false;
      showToast('Queue resumed.');
      await refresh(false);
    } catch (error) { showToast(error.message || String(error)); }
  } else if (runIsActive()) {
    const action = 'pause';
    try {
      await call('Control', action);
      widgetPaused = true;
      showToast('Queue paused.');
      await refresh(false);
    } catch (error) { showToast(error.message || String(error)); }
  } else {
    await runRequest({});
  }
  updateWidgetActionButton();
});

$('closeWidgetBtn')?.addEventListener('click', async () => { await call('CloseWidget'); });
$('openBtn')?.addEventListener('click', () => setMode(startupState?.configured ? 'manager' : 'setup'));
$('collapseBtn')?.addEventListener('click', () => setMode('compact'));
$('projectsBtn')?.addEventListener('click', () => setMode('setup'));
$('settingsBtn')?.addEventListener('click', () => setMode('settings'));
$('setupBackBtn')?.addEventListener('click', () => setMode(startupState?.configured ? 'manager' : 'compact'));
$('settingsBackBtn')?.addEventListener('click', () => setMode(startupState?.configured ? 'manager' : 'compact'));
$('skipBtn')?.addEventListener('click', async () => { try { await call('Control', 'skip'); showToast('Skip requested.'); } catch (error) { showToast(error.message || String(error)); } });
$('stopBtn')?.addEventListener('click', async () => { if (!await confirmAction('Stop active run?', 'TestDesk will request a safe stop for the active test queue. Captured output and history are retained.', 'Stop Run')) return; try { await call('Control', 'stop'); showToast('Stop requested.'); } catch (error) { showToast(error.message || String(error)); } });
$('testTitle')?.addEventListener('click', () => { compactDetailsOpen = !compactDetailsOpen; renderPlayer(); call('SetCompactExpanded', compactDetailsOpen).catch(console.error); });
$('discoverBtn')?.addEventListener('click', discover);
$('runSelectedBtn')?.addEventListener('click', async () => {
  if (!selected.size) return;
  await runRequest({ testIds: [...selected] });
});
$('runFilteredBtn')?.addEventListener('click', async () => { await runRequest({ filter: filter() }); });
$('selectFilteredBtn')?.addEventListener('click', selectAllFiltered);
$('clearSelectionBtn')?.addEventListener('click', clearSelection);
$('toggleAllProvidersBtn')?.addEventListener('click', () => {
  const currentProviders = [...new Set(filteredTests.map(t => t.providerId || 'unknown'))];
  if (collapsedProviders.size >= currentProviders.length) {
    collapsedProviders.clear();
    const btn = $('toggleAllProvidersBtn');
    if (btn) btn.textContent = 'Collapse All';
  } else {
    for (const p of currentProviders) collapsedProviders.add(p);
    const btn = $('toggleAllProvidersBtn');
    if (btn) btn.textContent = 'Expand All';
  }
  lastTestsRenderKey = '';
  renderTests(filteredTests);
});
$('copyDiagBtn')?.addEventListener('click', copyDiagnostics);
$('copyOutputBtn')?.addEventListener('click', copyOutput);
$('chooseProjectBtn')?.addEventListener('click', chooseProject);
$('chooseFolderBtn')?.addEventListener('click', chooseProject);
$('previewProjectBtn')?.addEventListener('click', previewProject);
$('setupProjectBtn')?.addEventListener('click', setupProject);
$('refreshProjectsBtn')?.addEventListener('click', refreshProjects);
$('openAddProjectBtn')?.addEventListener('click', () => setAddProjectSheet(true));
$('closeAddProjectBtn')?.addEventListener('click', () => setAddProjectSheet(false));
$('addProjectBackdrop')?.addEventListener('click', () => setAddProjectSheet(false));
$('activeRunTabBtn')?.addEventListener('click', () => setRunPane('active'));
$('recentRunsTabBtn')?.addEventListener('click', () => setRunPane('recent'));
$('generalSettingsTabBtn')?.addEventListener('click', () => setSettingsPane('general'));
$('agentSettingsTabBtn')?.addEventListener('click', () => setSettingsPane('agents'));
$('projectSelect')?.addEventListener('change', (event) => switchProject(event.target.value).catch(console.error));
$('compactProjectSelect')?.addEventListener('change', (event) => switchProject(event.target.value).catch(error => {
  showToast(error.message || String(error));
  const selected = activeProjects.find(project => same(project.root, startupState?.projectRoot));
  if (selected) event.target.value = selected.root;
}));
$('widgetSizeSelect')?.addEventListener('change', () => saveUI().catch(console.error));
$('widgetModeSelect')?.addEventListener('change', () => saveUI().catch(console.error));
$('topmostModeSelect')?.addEventListener('change', () => saveUI().catch(console.error));
$('logRetentionSelect')?.addEventListener('change', () => saveUI().catch(console.error));
['providerFilter', 'layerFilter', 'moduleFilter', 'statusFilter'].forEach(id => $(id)?.addEventListener('change', scheduleFilterRender));
['queryInput', 'tagInput'].forEach(id => $(id)?.addEventListener('input', debounce(scheduleFilterRender, 60)));
$('projectPathInput')?.addEventListener('change', previewProject);
$('mcpRefreshBtn')?.addEventListener('click', () => renderSettings().catch(console.error));
$('manualTargetSelect')?.addEventListener('change', (event) => { mcpSelectedTarget = event.target.value; renderManualMCP(); });
$('copyMcpManualBtn')?.addEventListener('click', async () => { await copyText($('mcpManualConfig')?.textContent || ''); flashButton('copyMcpManualBtn', 'Copied'); });
$('copyMcpCommandBtn')?.addEventListener('click', async () => { await copyText(selectedMCPCommand()); flashButton('copyMcpCommandBtn', 'Copied'); });
$('resetWidgetBtn')?.addEventListener('click', () => resetWidgetPosition().catch(error => showToast(error.message || String(error))));
$('openProjectsFromSettingsBtn')?.addEventListener('click', () => setMode('setup'));
if ($('installSkillsBtn')) {
  $('installSkillsBtn').addEventListener('click', async () => {
    const btn = $('installSkillsBtn');
    const original = btn.textContent;
    btn.textContent = 'Installing Skills...';
    btn.disabled = true;
    try {
      const result = await call('InstallSkills');
      if ($('skillsInstallStatus')) $('skillsInstallStatus').textContent = result.message || 'Skills installed successfully.';
      btn.textContent = 'Skills Installed ✓';
      showToast(result.message || 'TestDesk skills and instructions installed.');
      setTimeout(() => { btn.textContent = 'Reinstall / Refresh Skills'; btn.disabled = false; }, 2200);
    } catch (error) {
      if ($('skillsInstallStatus')) $('skillsInstallStatus').textContent = error.message || 'Failed to install skills.';
      btn.textContent = 'Install Failed';
      showToast(error.message || 'Failed to install skills');
      setTimeout(() => { btn.textContent = original; btn.disabled = false; }, 2200);
    }
  });
}
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && addProjectSheetOpen) setAddProjectSheet(false);
  if ((event.altKey && (event.key === 'm' || event.key === 'M')) || event.key === 'F10') {
    setMode('manager').catch(console.error);
  }
});
document.querySelectorAll('.brand-lockup').forEach(el => {
  el.addEventListener('dblclick', () => {
    setMode('manager').catch(console.error);
  });
});

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => init().catch(console.error));
} else {
  init().catch(console.error);
}
setInterval(() => refresh(false), 1200);
setInterval(() => refreshLog(), 900);
setInterval(() => {
  if (mode === 'compact') {
    if (runIsActive() || statusUnavailable()) renderPlayer();
    renderCompactProjects();
  } else if (mode === 'manager' && $('managerStatusUnavailable').classList.contains('hidden') === statusUnavailable()) {
    renderManager();
  }
}, 250);

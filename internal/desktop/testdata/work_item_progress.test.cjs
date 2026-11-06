const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '..', 'assets', 'app.js'), 'utf8');
const start = source.indexOf('function workItemProgress(');
const end = source.indexOf('\nfunction renderWidgetProgress(', start);
assert.ok(start >= 0 && end > start, 'work item progress function is available');
const workItemProgress = vm.runInNewContext(`(${source.slice(start, end)})`);

test('started items and elapsed estimates are not counted as completed work', () => {
  const result = workItemProgress({ total: 1, currentIndex: 0, items: [{ status: 'running', estimatedMillis: 1 }] });
  assert.equal(result.completed, 0);
  assert.equal(result.percent, 0);
});

test('only terminal work items count and an incomplete item list is unknown', () => {
  const result = workItemProgress({ total: 4, items: [{ status: 'passed' }, { status: 'failed' }, { status: 'cancelled' }, { status: 'running' }] });
  assert.equal(result.completed, 3);
  assert.equal(result.percent, 75);
  assert.equal(workItemProgress({ total: 4, items: [{ status: 'passed' }] }), null);
  assert.equal(workItemProgress({ total: 0, items: [] }), null);
});

test('an idle widget presents the latest failed run outcome', () => {
  const start = source.indexOf('function renderPlayer(');
  const end = source.indexOf('\nfunction setCompactState(', start);
  assert.ok(start >= 0 && end > start);
  const elements = new Map();
  const getElement = id => {
    if (!elements.has(id)) elements.set(id, { classList: { add() {}, remove() {}, toggle() {} }, style: {}, textContent: '' });
    return elements.get(id);
  };
  const context = {
    $: getElement, snapshot: { recentRuns: [{ status: 'failed', items: [], total: 0 }] },
    startupState: { configured: true, projectName: 'fixture', ui: {} }, mode: 'compact',
    widgetPaused: false, wasRunning: false, runInitiatedFromManager: false,
    statusUnavailable() { return false; },
    updateWidgetActionButton() {}, renderCompactStats() {}, currentItem() { return null; },
    renderWidgetProgress() {}, renderCompactDetails() {}, compactStateTone(status) { return status; },
    formatStatus(status) { return status === 'failed' ? 'Failed' : status; },
    setCompactState(label, tone) { context.label = label; context.tone = tone; }
  };
  const activeStart = source.indexOf('function runIsActive(');
  const activeEnd = source.indexOf('\nfunction workItemProgress(', activeStart);
  vm.runInNewContext(`${source.slice(activeStart, activeEnd)}(${source.slice(start, end)})()`, context);
  assert.equal(context.label, 'Failed');
  assert.equal(context.tone, 'failed');
  assert.equal(getElement('testTitle').textContent, 'Last run: Failed');
});

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '..', 'assets', 'app.js'), 'utf8');
const start = source.indexOf('function renderRunPanel(');
const end = source.indexOf('\nfunction ', start + 1);
assert.ok(start >= 0 && end > start);

test('manager does not present stale activity as current and restores it after recovery', () => {
  const panel = { className: '', innerHTML: '', set textContent(value) { this.innerHTML = value; }, querySelectorAll: () => [] };
  const context = {
    $: () => panel, lastRunPanelKey: '', unavailable: false,
    statusUnavailable() { return context.unavailable; },
    snapshot: { running: { id: 'run', status: 'running', passed: 1, failed: 0, skipped: 0 }, paused: false },
    currentItem: () => ({ testName: 'fixture' }), workItemProgress: () => null,
    escapeHtml: value => value, escapeAttr: value => value, formatStatus: value => value
  };
  vm.runInNewContext(source.slice(start, end), context);
  context.renderRunPanel();
  assert.match(panel.innerHTML, /fixture/);
  context.unavailable = true;
  context.renderRunPanel();
  assert.match(panel.innerHTML, /status unavailable/i);
  assert.doesNotMatch(panel.innerHTML, /data-control|fixture/);
  context.unavailable = false;
  context.renderRunPanel();
  assert.match(panel.innerHTML, /fixture/);
  assert.match(panel.innerHTML, /data-control/);
});

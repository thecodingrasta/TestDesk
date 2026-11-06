const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '..', 'assets', 'app.js'), 'utf8');
const start = source.indexOf('async function call(');
const end = source.indexOf('\nfunction applyMode(', start);

test('missing desktop bridge rejects reads and installation instead of fabricating success', async () => {
  const call = vm.runInNewContext(`(${source.slice(start, end).trim()})`, { bridge: () => null });
  await assert.rejects(call('Snapshot'), /desktop connection is unavailable/);
  await assert.rejects(call('InstallMCP'), /desktop connection is unavailable/);
});

test('desktop calls retain arguments and propagate backend errors', async () => {
  const api = {
    Snapshot: async value => value,
    InstallMCP: async () => { throw new Error('configuration is read only'); },
  };
  const call = vm.runInNewContext(`(${source.slice(start, end).trim()})`, { bridge: () => api });
  assert.equal(await call('Snapshot', 'selected-project'), 'selected-project');
  await assert.rejects(call('InstallMCP'), /configuration is read only/);
});

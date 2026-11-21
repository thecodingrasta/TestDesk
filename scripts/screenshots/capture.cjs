// Regenerates the product screenshots in docs/assets/screenshots.
//
// Usage (from the repository root):
//   node scripts/screenshots/capture.cjs              # all shots
//   node scripts/screenshots/capture.cjs --only compact-running
//   node scripts/screenshots/capture.cjs --check      # render to a temp dir, write nothing
//
// How it works: each shot loads the real desktop UI (internal/desktop/assets/index.html
// and app.js) from disk. A fixture bridge replaces the Go backend (window.go.desktop.App),
// the shot's steps drive the same functions a user would reach, and headless Microsoft
// Edge writes a PNG at the shot's window size. No server, no npm packages, no Wails build.
//
// Requires Microsoft Edge (or set TESTDESK_BROWSER_PATH to another Chromium binary).

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { spawnSync } = require('node:child_process');
const { buildFixtures, SHOTS } = require('./scenarios.cjs');

const repoRoot = path.resolve(__dirname, '..', '..');
const assetsDir = path.join(repoRoot, 'internal', 'desktop', 'assets');
const outDir = path.join(repoRoot, 'docs', 'assets', 'screenshots');
// Browser resolution order: TESTDESK_BROWSER_PATH, then the newest Playwright headless shell
// already installed in the local ms-playwright cache, then Microsoft Edge. The headless shell
// is a console-subsystem binary, so its DOM dump and screenshot output reach this process.
// Microsoft Edge (a GUI-subsystem binary) returned before writing output on the host used here.
function resolveBrowser() {
  if (process.env.TESTDESK_BROWSER_PATH) return process.env.TESTDESK_BROWSER_PATH;
  const cacheRoot = path.join(process.env.LOCALAPPDATA || '', 'ms-playwright');
  if (fs.existsSync(cacheRoot)) {
    const shells = fs.readdirSync(cacheRoot)
      .filter((name) => /^chromium_headless_shell-\d+$/.test(name))
      .sort((a, b) => Number(b.split('-')[1]) - Number(a.split('-')[1]));
    for (const dir of shells) {
      const exe = path.join(cacheRoot, dir, 'chrome-headless-shell-win64', 'chrome-headless-shell.exe');
      if (fs.existsSync(exe)) return exe;
    }
  }
  return 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe';
}
const browserPath = resolveBrowser();
const SCALE = Number(process.env.TESTDESK_SHOT_SCALE || 2);

function parseArgs(argv) {
  const args = { only: null, check: false };
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--only') args.only = argv[++i];
    else if (argv[i] === '--check') args.check = true;
    else throw new Error(`unknown argument: ${argv[i]}`);
  }
  return args;
}

// The bridge answers every call the page makes. Unknown methods resolve to undefined
// instead of throwing, so an unexpected call shows up as a visible gap in the screenshot
// rather than a silent test pass.
function bridgeScript(fixtures, snapshot) {
  const payload = JSON.stringify({ ...fixtures, snapshot });
  return `<script>
(() => {
  const data = ${payload};
  const api = {
    LoadingState: async () => ({ active: false }),
    Initialise: async () => ({ startup: data.startup, snapshot: data.snapshot }),
    Startup: async () => data.startup,
    Snapshot: async () => JSON.parse(JSON.stringify(data.snapshot)),
    ActiveProjects: async () => data.activeProjects,
    ListProjects: async () => data.projects,
    MCPWorkspaces: async () => data.mcpTargets,
    SetWindowMode: async () => {},
    SetCompactExpanded: async () => {},
    ApplyWidgetVisibility: async () => {},
    SaveUI: async () => data.ui,
    Control: async () => {},
    SwitchProject: async () => ({ startup: data.startup, snapshot: data.snapshot }),
    Diagnostics: async (runId) => ({ run: [data.snapshot.running, ...data.snapshot.recentRuns].find((r) => r && r.id === runId) || null, log: data.runLogs[runId] || '', path: '' }),
    Discover: async () => data.snapshot,
    WindowSize: async () => ({ width: 1280, height: 820 }),
  };
  window.go = { desktop: { App: new Proxy(api, { get: (target, prop) => (prop in target ? target[prop] : async () => undefined) }) } };
})();
</script>`;
}

function buildHtml(shot, fixtures) {
  let html = fs.readFileSync(path.join(assetsDir, 'index.html'), 'utf8');
  const base = `<base href="${pathToFileURL(assetsDir + path.sep).href}">`;
  const snapshot = shot.snapshot === 'live' ? fixtures.liveSnapshot : shot.snapshot === 'failed' ? fixtures.failedSnapshot : fixtures.idleSnapshot;
  // The scenario waits for app.js to finish booting, then performs the shot's steps.
  const scenario = `<script>
window.addEventListener('load', () => setTimeout(async () => {
  try {
    await new Promise((resolve) => setTimeout(resolve, 1200));
    ${shot.js}
    document.body.dataset.shotReady = 'true';
  } catch (error) {
    document.body.dataset.shotError = String(error && error.stack || error);
  }
}, 0));
</script>`;
  html = html.replace('<head>', `<head>${base}${bridgeScript(fixtures, snapshot)}`);
  html = html.replace('</body>', `${scenario}</body>`);
  return html;
}

function edgeArgs(shot, htmlPath, workDir, extra) {
  return [
    '--headless',
    '--disable-gpu',
    '--hide-scrollbars',
    '--no-first-run',
    '--disable-background-networking',
    // 2x device pixels keep text and borders sharp on high-DPI screens. The CSS layout is
    // unchanged, so the same screen is rendered at twice the resolution.
    `--force-device-scale-factor=${SCALE}`,
    `--window-size=${shot.width},${shot.height}`,
    `--user-data-dir=${path.join(workDir, 'profile')}`,
    '--virtual-time-budget=6000',
    ...extra,
    pathToFileURL(htmlPath).href,
  ];
}

// Returns stdout and the browser's console lines (written to stderr with logging on).
function runEdge(args) {
  const result = spawnSync(browserPath, args, { encoding: 'utf8', timeout: 90000, maxBuffer: 20 * 1024 * 1024 });
  if (result.error) throw new Error(result.error.message);
  if (result.status !== 0) throw new Error(`Edge exited ${result.status}\n${result.stderr}`);
  const consoleLines = (result.stderr || '').split('\n').filter((line) => /CONSOLE/.test(line)).join('\n');
  return { dom: result.stdout, console: consoleLines };
}

// A shot only counts when its steps finished. The DOM dump carries the markers that
// the scenario sets on <body>, so a failed step stops the run instead of producing a
// screenshot of a half-navigated screen.
function assertScenarioCompleted(shot, run) {
  const body = /<body[^>]*>/.exec(run.dom);
  const error = /data-shot-error="([^"]*)"/.exec(run.dom);
  if (error) throw new Error(`${shot.name}: scenario step failed: ${error[1].replace(/&#10;/g, '\n')}\n${run.console}`);
  if (!/data-shot-ready="true"/.test(run.dom)) {
    const preview = run.dom.slice(0, 600).replace(/\s+/g, ' ');
    throw new Error(`${shot.name}: scenario did not finish before the capture budget. body: ${body ? body[0] : 'missing'}\ndom(${run.dom.length} chars): ${preview}\n${run.console}`);
  }
}

function renderShot(shot, fixtures, outPath, workDir) {
  const htmlPath = path.join(workDir, `${shot.name}.html`);
  fs.writeFileSync(htmlPath, buildHtml(shot, fixtures));
  assertScenarioCompleted(shot, runEdge(edgeArgs(shot, htmlPath, workDir, ['--dump-dom', '--enable-logging=stderr', '--v=0'])));
  runEdge(edgeArgs(shot, htmlPath, workDir, [`--screenshot=${outPath}`]));
  if (!fs.existsSync(outPath) || fs.statSync(outPath).size === 0) throw new Error(`${shot.name}: no image written`);
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  if (!fs.existsSync(browserPath)) throw new Error(`browser not found: ${browserPath} (set TESTDESK_BROWSER_PATH)`);
  const shots = SHOTS.filter((s) => !args.only || s.name === args.only);
  if (!shots.length) throw new Error(`no shot named ${args.only}`);

  const destination = args.check ? fs.mkdtempSync(path.join(os.tmpdir(), 'testdesk-shots-')) : outDir;
  fs.mkdirSync(destination, { recursive: true });
  const workDir = fs.mkdtempSync(path.join(os.tmpdir(), 'testdesk-shot-work-'));
  try {
    const fixtures = buildFixtures(new Date());
    for (const shot of shots) {
      const outPath = path.join(destination, `${shot.name}.png`);
      renderShot(shot, fixtures, outPath, workDir);
      console.log(`${shot.name}: ${outPath}`);
    }
  } finally {
    fs.rmSync(workDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
  }
}

main();

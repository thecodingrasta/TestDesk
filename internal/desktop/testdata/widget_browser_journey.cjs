// Optional Windows browser journey: node internal/desktop/testdata/widget_browser_journey.cjs
// Uses the installed Edge and a disposable profile; no native desktop is started.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { spawnSync } = require('node:child_process');

const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'testdesk-widget-journey-'));
try {
  const assets = path.resolve(__dirname, '..', 'assets');
  let html = fs.readFileSync(path.join(assets, 'index.html'), 'utf8');
  html = html.replace('<head>', `<head><base href="${pathToFileURL(assets + path.sep).href}">`);
  const probe = `<script>
window.addEventListener('load', () => setTimeout(async () => {
  const record = value => { document.body.dataset.widgetProbe = btoa(unescape(encodeURIComponent(JSON.stringify(value)))); };
  try {
    const startup = root => ({configured:true, projectRoot:root, projectName:root === 'A' ? 'Alpha' : 'Beta', ui:{widgetSize:'small',widgetMode:'always'}});
    const makeSnapshot = root => ({running:{id:root,currentIndex:0,total:1,passed:0,failed:0,items:[{status:'running',testName:'Test '+root}]},recentRuns:[],queuedCount:0});
    let root = 'A'; let fail = false; let hangSnapshot = false; let releaseSnapshot; let hangProjects = false; let releaseProjects; let hangStartup = false; let releaseStartup; const controls = [];
    const projects = [{root:'A',name:'Alpha',active:true},{root:'B',name:'Beta',active:true}];
    window.go = {desktop:{App:{Snapshot:async()=>{if(fail) throw Error('fixture disconnected'); if(hangSnapshot) return new Promise(resolve => {releaseSnapshot=resolve}); return makeSnapshot(root)},Startup:()=>hangStartup ? new Promise(resolve => {releaseStartup=resolve}) : Promise.resolve(startup(root)),ActiveProjects:()=>hangProjects ? new Promise(resolve => {releaseProjects=resolve}) : Promise.resolve(projects),ApplyWidgetVisibility:async()=>{},Control:async(action)=>{controls.push({root,action})},SwitchProject:async(next)=>{root=next; return {startup:startup(root),snapshot:makeSnapshot(root)}}}}};
    startupState = startup(root); activeProjects = projects; acceptSnapshot(makeSnapshot(root));
    document.getElementById('app').style.width = '380px';
    applyMode('compact'); renderCompactProjects();
    projects[1].active = false; renderCompactProjects();
    const idleProjectHidden = document.getElementById('compactProjectSelect').classList.contains('hidden');
    projects[1].active = true; renderCompactProjects();
    const twoActiveProjectsVisible = !document.getElementById('compactProjectSelect').classList.contains('hidden');
    const before = document.getElementById('widgetStateLabel').textContent;
    fail = true; await refresh(false);
    const unavailable = document.getElementById('widgetStateLabel').textContent;
    const hidden = document.getElementById('runClock').classList.contains('hidden');
    const count = document.getElementById('widgetPassed').textContent;
    const heading = document.querySelector('.widget-heading').getBoundingClientRect();
    const badge = document.getElementById('widgetState').getBoundingClientRect();
    const badgeVisible = badge.left >= heading.left && badge.right <= heading.right && badge.width > 0;
    fail = false; await refresh(false);
    const recovered = document.getElementById('widgetStateLabel').textContent;
    const select = document.getElementById('compactProjectSelect'); select.value = 'B'; select.dispatchEvent(new Event('change',{bubbles:true}));
    await new Promise(resolve => setTimeout(resolve,20));
    const selected = startupState.projectRoot; const title = document.getElementById('testTitle').textContent;
    document.getElementById('widgetActionBtn').click();
    await new Promise(resolve => setTimeout(resolve,20));
    const previous = {status:'failed',total:1,failed:1,items:[{status:'failed'}]};
    wasRunning = true; runInitiatedFromManager = true;
    acceptSnapshot({running:null,recentRuns:[previous],queuedCount:1,paused:false}); renderPlayer();
    const queued = document.getElementById('widgetStateLabel').textContent;
    const queuedTitle = document.getElementById('testTitle').textContent;
    const trackingQueued = wasRunning && runInitiatedFromManager;
    acceptSnapshot({running:null,recentRuns:[],queuedCount:1,paused:true}); renderPlayer();
    const paused = document.getElementById('widgetStateLabel').textContent;
    const terminal = {};
    for (const status of ['passed','failed','cancelled']) {
      acceptSnapshot({running:null,recentRuns:[{...previous,status}],queuedCount:0,paused:false}); renderPlayer();
      terminal[status] = document.getElementById('widgetStateLabel').textContent;
    }
    const trackingFinished = wasRunning || runInitiatedFromManager;
    await new Promise(resolve => setTimeout(resolve,20));
    hangProjects = true; lastActiveProjectsUpdate = Date.now() - 7000;
    await refresh(false); await refresh(false); renderCompactProjects();
    const warning = document.getElementById('widgetProjectsStatus');
    const projectsUnavailable = !warning.classList.contains('hidden');
    const unknownProjects = Array.from(select.options).every(option => option.textContent.includes('unknown'));
    const warningRect = warning.getBoundingClientRect();
    const staleBadgeRect = document.getElementById('widgetState').getBoundingClientRect();
    const staleIndicatorsVisible = warningRect.left >= heading.left && staleBadgeRect.right <= heading.right && warningRect.width > 0;
    hangProjects = false; releaseProjects(projects);
    await new Promise(resolve => setTimeout(resolve,20));
    const projectsRecovered = warning.classList.contains('hidden') && Array.from(select.options).every(option => option.textContent.includes('active'));
    hangStartup = true; mode = 'manager';
    await refresh(true); mode = 'compact'; root = 'A'; await refresh(false);
    const startupDoesNotBlock = snapshot.running.id === 'A';
    const currentState = startupState; releaseStartup({...startup('B'), projectName:'Updated Beta'});
    await new Promise(resolve => setTimeout(resolve,20));
    const startupRecovered = startupState !== currentState && startupState.projectName === 'Updated Beta';
    mode = 'manager'; hangSnapshot = true; const pendingSnapshot = refresh(false);
    lastStatusUpdate = Date.now() - 7000;
    await new Promise(resolve => setTimeout(resolve,300));
    const managerUnavailable = !document.getElementById('managerStatusUnavailable').classList.contains('hidden') && document.getElementById('runPanel').textContent.includes('Status unavailable') && document.getElementById('queueCount').textContent === '—';
    hangSnapshot = false; releaseSnapshot(makeSnapshot(root)); await pendingSnapshot;
    const managerRecovered = document.getElementById('managerStatusUnavailable').classList.contains('hidden') && document.getElementById('runPanel').textContent.includes('Test A');
    record({idleProjectHidden,twoActiveProjectsVisible,before,unavailable,hidden,count,badgeVisible,recovered,selected,title,controls,queued,queuedTitle,trackingQueued,paused,terminal,trackingFinished,projectsUnavailable,unknownProjects,staleIndicatorsVisible,projectsRecovered,startupDoesNotBlock,startupRecovered,managerUnavailable,managerRecovered});
  } catch (error) { record({error:String(error),stack:error.stack}); }
},0));
</script>`;
  html = html.replace('</body>', `${probe}</body>`);
  const fixture = path.join(temp, 'fixture.html');
  fs.writeFileSync(fixture, html);
  const edge = process.env.TESTDESK_BROWSER_PATH || 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe';
  const result = spawnSync(edge, ['--headless', '--disable-gpu', '--disable-background-networking', '--no-first-run', '--window-size=380,160', '--virtual-time-budget=3000', `--user-data-dir=${path.join(temp, 'profile')}`, '--dump-dom', pathToFileURL(fixture).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 4000000 });
  assert.equal(result.error, undefined, result.error?.message);
  assert.equal(result.status, 0, `Edge exited ${result.status}`);
  const match = result.stdout.match(/data-widget-probe="([^"]+)"/);
  assert.ok(match, 'widget probe marker missing');
  const observed = JSON.parse(Buffer.from(match[1], 'base64').toString('utf8'));
  console.log(JSON.stringify(observed));
  assert.equal(observed.error, undefined, observed.stack);
  assert.deepEqual(observed, { idleProjectHidden:true,twoActiveProjectsVisible:true,before: 'Running', unavailable: 'Status unavailable', hidden: true, count: '—', badgeVisible: true, recovered: 'Running', selected: 'B', title: 'Test B', controls: [{root:'B',action:'pause'}], queued:'Queued', queuedTitle:'Tests queued', trackingQueued:true, paused:'Paused', terminal:{passed:'Passed',failed:'Failed',cancelled:'Stopped'}, trackingFinished:false,projectsUnavailable:true,unknownProjects:true,staleIndicatorsVisible:true,projectsRecovered:true,startupDoesNotBlock:true,startupRecovered:true,managerUnavailable:true,managerRecovered:true });
} finally {
  fs.rmSync(temp, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });
}

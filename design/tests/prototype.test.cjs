const {test, before, after} = require('node:test');
const assert = require('node:assert/strict');
const {chromium} = require('playwright');
const {pathToFileURL} = require('node:url');
const path = require('node:path');
let browser;
before(async () => {browser = await chromium.launch({headless:true});});
after(async () => {await browser?.close();});
const url = pathToFileURL(path.resolve(__dirname,'../index.html')).href;
async function fixture(fn) {
  const page = await browser.newPage({viewport:{width:1500,height:1100}});
  const errors = [];
  page.on('pageerror',error => errors.push(error.message));
  await page.goto(url);
  await page.getByTestId('gg-terminal').waitFor();
  await page.getByTestId('gg-sidebar-group-1-session-2-row').click();
  await page.getByTestId('gg-next-panel').click();
  page.setDefaultTimeout(5000);
  await audit(page);
  try {await fn(page); await audit(page); assert.deepEqual(errors,[]);} finally {await page.close();}
}
async function audit(page) {
  assert.deepEqual(await page.evaluate(() => {
    const seen = new Set(), errors = [];
    for (const n of document.querySelectorAll('*')) {
      const id = n.getAttribute('data-testid');
      if (!id || !id.trim()) errors.push(`missing: ${n.tagName}`);
      else if (seen.has(id)) errors.push(`duplicate: ${id}`);
      seen.add(id);
    }
    return errors;
  }),[]);
}
async function scenario(page,id) {await page.getByTestId('gg-scenario').selectOption(id); await audit(page);}
async function press(page,key) {await page.getByTestId('gg-session-list').focus(); await page.keyboard.press(key); await audit(page);}
async function click(page,id) {await page.getByTestId(id).click(); await audit(page);}
const waitCreated = (page, name) => page.waitForFunction(name => window.GG.state.sessions.some(s => s.name === name), name);
const snapshot = page => page.evaluate(() => JSON.parse(JSON.stringify(window.GG.state)));

test('every scenario and dialog has complete unique selectors', () => fixture(async page => {
  const ids = await page.getByTestId('gg-scenario').locator('option').evaluateAll(nodes => nodes.map(n => n.value));
  for (const id of ids) {
    await scenario(page,id);
    await press(page,'?'); await audit(page);
    await page.keyboard.press('Escape');
    await press(page,','); await audit(page);
    await page.keyboard.press('Escape');
    await press(page,'n'); await audit(page);
    await page.keyboard.press('Tab'); await audit(page);
    await page.keyboard.press('Escape'); await page.keyboard.press('Escape');
  }
}));
test('keyboard browsing preserves open, single click opens, and cycling reveals the open row', () => fixture(async page => {
  const original = await snapshot(page);
  await press(page,'ArrowDown');
  let s = await snapshot(page); assert.equal(s.open,original.open); assert.equal(s.selected,'1-3');
  await click(page,'gg-sidebar-group-2-session-2-row');
  s = await snapshot(page); assert.equal(s.open,'2-2'); assert.equal(s.selected,'2-2'); assert.equal(s.focus,'terminal');
  await click(page,'gg-next-panel'); assert.equal((await snapshot(page)).focus,'list');
  await press(page,'a');
  await click(page,'gg-next-panel');
  await click(page,'gg-next-panel');
  assert.equal((await snapshot(page)).selected,'2-2');
  assert.equal(await page.getByTestId('gg-session-list').evaluate(n => n === document.activeElement),true);
}));
test('create validates branches, picker cancellation preserves inputs, and IDs survive localization', () => fixture(async page => {
  await press(page,'n'); await page.keyboard.press('Enter');
  assert.match(await page.getByTestId('gg-dialog-error').innerText(),/required/);
  await page.getByTestId('gg-create-name').fill('feat/payments'); await page.keyboard.press('Tab');
  await page.keyboard.press('ArrowDown'); await page.keyboard.press('Escape');
  assert.equal(await page.getByTestId('gg-create-name').inputValue(),'feat/payments');
  assert.match(await page.getByTestId('gg-create-template').innerText(),/shell/);
  await page.keyboard.press('Tab'); await click(page,'gg-template-codex'); await click(page,'gg-create-submit');
  await waitCreated(page,'feat/payments');
  let s = await snapshot(page); const created = s.sessions.find(s => s.name === 'feat/payments');
  assert.equal(created.branch,''); assert.ok(!s.branches.includes('1:feat/payments'));
  assert.equal(created.template,'codex'); assert.equal(s.open,`1-${created.id}`); assert.equal(s.focus,'terminal');
  assert.equal(created.path,'/workspace/atlas__sessions/feat~payments');
  await click(page,'gg-next-panel'); await press(page,'n');
  await page.getByTestId('gg-create-name').fill('feat/payments'); await click(page,'gg-create-submit');
  assert.match(await page.getByTestId('gg-dialog-error').innerText(),/already/);
  await page.keyboard.press('Escape'); await press(page,',');
  await page.getByTestId('gg-setting-language').selectOption('ru'); await click(page,'gg-settings-save');
  assert.equal(await page.locator('html').getAttribute('lang'),'ru');
  assert.equal(await page.getByTestId(`gg-sidebar-group-1-session-${created.id}-row`).count(),1);
}));
test('ordinary directories allow duplicate names, retain files on deletion, and only offer global settings', () => fixture(async page => {
  await scenario(page,'directory'); await press(page,'n'); await click(page,'gg-create-submit');
  let s = await snapshot(page); assert.equal(s.sessions.filter(s => s.group === 3 && s.name === 'scratch').length,3);
  const created = s.selected, files = s.files;
  await click(page,'gg-next-panel'); await press(page,',');
  assert.deepEqual(await page.getByTestId('gg-setting-scope').locator('option').evaluateAll(n => n.map(x => x.value)),['global']);
  await page.keyboard.press('Escape'); await press(page,'d'); await click(page,'gg-confirm-yes');
  s = await snapshot(page); assert.ok(!s.sessions.some(x => `${x.group}-${x.id}` === created)); assert.deepEqual(s.files,files);
}));

test('deleting a detached commit preserves the worktree', () => fixture(async page => {
  await scenario(page,'detached-commit');
  const before = await snapshot(page);
  await press(page,'d'); await click(page,'gg-confirm-yes');
  const after = await snapshot(page);
  assert.deepEqual(after.sessions,before.sessions);
  assert.match(after.notice.message[0],/git switch -c/);
}));

test('long worktree names support independent branches and use a single hashed folder', () => fixture(async page => {
  const branch = 'данные/'.repeat(22) + 'отчёт';
  const digest = require('node:crypto').createHash('sha256').update(branch).digest('hex');
  await press(page,'n');
  await page.getByTestId('gg-create-name').fill(branch);
  await page.keyboard.press('ArrowDown'); await page.keyboard.type('feat/explicit');
  await page.keyboard.press('Enter');
  await page.getByTestId('gg-dialog').waitFor({state:'hidden'});
  await waitCreated(page,branch);
  const created = (await snapshot(page)).sessions.at(-1);
  assert.equal(created.name, branch); assert.equal(created.branch, 'feat/explicit');
  assert.equal(created.path, '/workspace/atlas__sessions/~' + digest);
}));
test('split cancellation, placements and cross-project switches preserve terminal identity and output', () => fixture(async page => {
  const original = (await snapshot(page)).sessions.find(s => s.id === 2 && s.group === 1).panes[0];
  await press(page,'t'); await page.keyboard.press('Escape');
  assert.equal((await snapshot(page)).sessions.find(s => s.id === 2 && s.group === 1).panes.length,1);
  await press(page,'t'); await click(page,'gg-split-right'); await click(page,'gg-next-panel');
  await press(page,'t'); await click(page,'gg-split-below');
  let s = await snapshot(page), task = s.sessions.find(s => s.id === 2 && s.group === 1);
  assert.equal(task.panes.length,3); assert.equal(task.layout.direction,'right'); assert.equal(task.layout.second.direction,'below');
  assert.equal(task.panes[1].title,'zsh · feat/auth');
  await click(page,'gg-next-panel'); await click(page,'gg-sidebar-group-3-session-1-row');
  await click(page,'gg-next-panel'); await click(page,'gg-sidebar-group-1-session-2-row');
  task = (await snapshot(page)).sessions.find(s => s.id === 2 && s.group === 1);
  assert.deepEqual(task.panes[0],original); assert.equal(task.panes.length,3);
  await page.getByTestId(`gg-pane-${original.id}-input`).fill('n'); await page.keyboard.press('Enter');
  assert.equal((await snapshot(page)).modal,null);
}));
test('stop background keeps the open session; start replaces panes; delete retains branch', () => fixture(async page => {
  await scenario(page,'lifecycle'); const initial = await snapshot(page);
  const previousPane = initial.sessions.find(s => s.group === 1 && s.id === 4).panes[0].id;
  await press(page,'s'); await click(page,'gg-confirm-yes');
  let s = await snapshot(page); assert.equal(s.open,initial.open); assert.equal(s.focus,'list');
  assert.equal(s.sessions.find(s => s.group === 1 && s.id === 4).panes.length,0);
  await press(page,'s'); s = await snapshot(page); assert.equal(s.open,'1-4');
  assert.notEqual(s.sessions.find(s => s.group === 1 && s.id === 4).panes[0].id,previousPane);
  await click(page,'gg-next-panel'); await press(page,'d'); await click(page,'gg-confirm-yes');
  s = await snapshot(page); assert.equal(s.open,null); assert.ok(s.branches.includes('1:refactor/cache'));
  assert.ok(!s.files.includes('/workspace/atlas__sessions/refactor~cache'));
}));
test('dirty and protected sessions cannot be deleted; disappeared confirmation never targets the neighbor', () => fixture(async page => {
  await scenario(page,'dirty'); const initial = await snapshot(page);
  await press(page,'d'); await click(page,'gg-confirm-yes');
  assert.deepEqual((await snapshot(page)).sessions,initial.sessions);
  assert.match(await page.getByTestId('gg-notice').innerText(),/prevent deletion/);
  await scenario(page,'protected'); assert.equal(await page.getByTestId('gg-delete').count(),0);
  await press(page,'?'); assert.equal(await page.getByTestId('gg-menu-delete').count(),0); await page.keyboard.press('Escape');
  const files = (await snapshot(page)).files; await press(page,'s'); await click(page,'gg-confirm-yes');
  assert.ok(!(await snapshot(page)).sessions.some(s => s.group === 2 && s.id === 3)); assert.deepEqual((await snapshot(page)).files,files);
  await scenario(page,'vanished'); await press(page,'d'); await click(page,'gg-simulate');
  const survivors = (await snapshot(page)).sessions; await click(page,'gg-confirm-yes');
  assert.deepEqual((await snapshot(page)).sessions,survivors); assert.match(await page.getByTestId('gg-notice').innerText(),/no longer exists/);
}));
test('search threshold, applied and empty queries, activity filter and Escape priority', () => fixture(async page => {
  await scenario(page,'search'); assert.equal(await page.getByTestId('gg-search').count(),1);
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('release'); await page.keyboard.press('Enter');
  await press(page,'d'); await click(page,'gg-confirm-yes');
  let s = await snapshot(page); assert.equal(s.sessions.length,10); assert.equal(s.query,''); assert.equal(await page.getByTestId('gg-search').count(),0);
  await press(page,'/'); assert.equal((await snapshot(page)).searching,false);
  await scenario(page,'no-matches'); assert.equal(await page.getByTestId('gg-no-matches').count(),1);
  await press(page,'Escape'); assert.equal((await snapshot(page)).query,'');
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('atlas'); await page.keyboard.press('Enter');
  await press(page,'?'); await page.keyboard.press('Escape'); assert.equal((await snapshot(page)).query,'atlas');
  await press(page,'a'); s = await snapshot(page); assert.equal(s.query,''); assert.equal(s.onlyActive,true);
  await press(page,'a'); assert.equal(await page.getByTestId('gg-search').count(),1);
}));
test('settings draft cancels, binds to open project and preserves existing session templates', () => fixture(async page => {
  await scenario(page,'settings'); await press(page,',');
  assert.match(await page.getByTestId('gg-dialog-settings-context').innerText(),/atlas/);
  await page.getByTestId('gg-setting-language').selectOption('ru'); await page.keyboard.press('Escape');
  assert.equal(await page.locator('html').getAttribute('lang'),'en');
  const before = (await snapshot(page)).sessions;
  await press(page,','); await page.getByTestId('gg-setting-template').selectOption('codex');
  await page.getByTestId('gg-setting-scope').selectOption('local'); await click(page,'gg-settings-save');
  const s = await snapshot(page); assert.equal(s.groups[0].config.template,'codex'); assert.equal(s.global.template,'shell'); assert.deepEqual(s.sessions,before);
}));
test('unavailable group and missing template keep the existing terminal intact', () => fixture(async page => {
  for (const id of ['unavailable','missing-template']) {
    await scenario(page,id); const before = await snapshot(page);
    if (id === 'unavailable') await click(page,'gg-sidebar-group-2-error'); else await press(page,'Enter');
    const after = await snapshot(page); assert.equal(after.open,before.open); assert.deepEqual(after.sessions,before.sessions); assert.equal(after.notice.kind,'error');
  }
}));
test('notification deadlines survive rendering and replacement; errors do not expire', () => fixture(async page => {
  await page.clock.install(); await scenario(page,'notices'); await click(page,'gg-simulate');
  await page.clock.fastForward(4000); await page.getByTestId('gg-size').selectOption('160x42');
  assert.equal((await snapshot(page)).notice.kind,'success');
  await click(page,'gg-simulate'); await page.clock.fastForward(1500);
  assert.match((await snapshot(page)).notice.message[0],/2/);
  await page.clock.fastForward(3500); assert.equal((await snapshot(page)).notice,null);
  await click(page,'gg-simulate'); await page.clock.fastForward(20000); assert.equal((await snapshot(page)).notice.kind,'error');
}));
test('launch and quit preserve sessions without opening or creating a shell', () => fixture(async page => {
  await scenario(page,'workspace'); const before = await snapshot(page);
  assert.equal(before.open,null); assert.equal(before.focus,'list');
  assert.ok(before.sessions.every(s => s.kind !== 'temporary'));
  await press(page,'q'); assert.deepEqual((await snapshot(page)).sessions,before.sessions);
  await click(page,'gg-resume'); const after = await snapshot(page);
  assert.equal(after.open,null); assert.deepEqual(after.sessions,before.sessions);
}));
test('responsive boundary, compact access, short menus, localization and colorless indicators', () => fixture(async page => {
  for (const [size,isCompact] of [['160x42',false],['100x16',false],['99x24',true],['120x15',true]]) {
    await page.getByTestId('gg-size').selectOption(size); await audit(page);
    const s = await snapshot(page); if (s.focus === 'list') await click(page,'gg-next-panel');
    assert.equal(await page.getByTestId('gg-sidebar').count(),isCompact ? 0 : 1);
    assert.equal(await page.getByTestId('gg-status').isVisible(),true);
    await click(page,'gg-next-panel'); assert.equal(await page.getByTestId('gg-sidebar').isVisible(),true);
    await press(page,'?'); await page.keyboard.press('End'); await audit(page); await page.keyboard.press('Escape');
    if (isCompact) {await press(page,'Escape'); assert.equal(await page.getByTestId('gg-sidebar').count(),0); await click(page,'gg-next-panel');}
  }
  await scenario(page,'long'); await page.getByTestId('gg-size').selectOption('120x15'); await press(page,'?');
  for (let i=0;i<10;i++) await page.keyboard.press('ArrowDown');
  const item = page.getByTestId('gg-menu-quit'); assert.equal(await item.isVisible(),true);
  const itemBounds = await item.boundingBox(), dialogBounds = await page.getByTestId('gg-dialog').boundingBox();
  assert.ok(itemBounds.y >= dialogBounds.y && itemBounds.y+itemBounds.height <= dialogBounds.y+dialogBounds.height);
  await page.keyboard.press('Escape'); await click(page,'gg-color'); assert.match(await page.getByTestId('gg-terminal').getAttribute('class'),/no-color/);
  assert.ok(await page.getByTestId('gg-sidebar-group-1-session-2-open').innerText());
}));

test('agent states do not change activity, and persistent splits retain their record after the last shell exits', () => fixture(async page => {
  await scenario(page,'agent');
  const initial = await snapshot(page);
  for (const code of ['ready','working','waiting','done','interrupted','shell','unknown']) {
    await click(page,'gg-simulate');
    const session = (await snapshot(page)).sessions.find(s => s.group === 1 && s.id === 2);
    assert.equal(session.agent,code);
    assert.deepEqual(session.panes,initial.sessions.find(s => s.group === 1 && s.id === 2).panes);
  }
  await scenario(page,'shell-exit'); await press(page,'t'); await click(page,'gg-split-right');
  await click(page,'gg-simulate');
  let s = await snapshot(page); assert.equal(s.open,'1-1'); assert.equal(s.sessions.find(s => s.group === 1 && s.id === 1).panes.length,1);
  await click(page,'gg-simulate'); s = await snapshot(page); assert.equal(s.open,null); assert.equal(s.detached,false);
}));

test('keyboard modal choices follow arrows and native Tab focus', () => fixture(async page => {
  await press(page,'t'); await page.keyboard.press('ArrowRight'); await page.keyboard.press('Enter');
  let s = await snapshot(page); assert.equal(s.sessions.find(s => s.group === 1 && s.id === 2).layout.direction,'right');
  await click(page,'gg-next-panel'); await press(page,'?');
  await page.keyboard.press('Tab'); await page.keyboard.press('Tab'); await page.keyboard.press('Enter');
  assert.equal((await snapshot(page)).modal.type,'split');
  await page.keyboard.press('Escape');
  await press(page,'n'); await page.getByTestId('gg-create-name').fill('feat/keyboard');
  await page.keyboard.press('Tab'); await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
  assert.match(await page.getByTestId('gg-create-template').innerText(),/codex/);
  await page.keyboard.press('Enter'); await waitCreated(page,'feat/keyboard'); s = await snapshot(page);
  assert.equal(s.sessions.find(s => s.name === 'feat/keyboard').template,'codex');
}));

test('worktrees and ordinary terminals share a list and hover previews without switching', () => fixture(async page => {
  assert.equal(await page.getByTestId('gg-sidebar-group-1-name').innerText(),'atlas');
  assert.equal(await page.locator('.session-section').count(),0);
  const session = await page.getByTestId('gg-sidebar-group-1-session-2-row').boundingBox();
  const terminal = await page.getByTestId('gg-sidebar-group-1-session-1-row').boundingBox();
  assert.ok(terminal.y < session.y);
  assert.equal(await page.getByTestId('gg-sidebar-group-1-session-1-kind').innerText(),'>_');
  assert.equal(await page.getByTestId('gg-sidebar-group-1-session-2-kind').innerText(),'⎇');
  const initial = await snapshot(page);
  await page.getByTestId('gg-sidebar-group-2-session-2-row').hover(); await audit(page);
  assert.equal(await page.getByTestId('gg-details-path').getAttribute('title'),'/workspace/beacon__sessions/feat~auth');
  assert.equal(await page.getByTestId('gg-details-git-value').innerText(),'modified');
  assert.deepEqual(await snapshot(page),initial);
  await page.getByTestId('gg-sidebar-group-3-heading').hover();
  assert.equal(await page.getByTestId('gg-details-path').getAttribute('title'),'/workspace/scratch');
  assert.equal(await page.getByTestId('gg-details-sessions-value').innerText(),'3');
  await page.getByTestId('gg-masthead').hover();
  assert.equal(await page.getByTestId('gg-details-path').getAttribute('title'),'/workspace/atlas__sessions/feat~auth');
  await click(page,'gg-sidebar-group-1-session-3-row');
  const s = await snapshot(page); assert.equal(s.open,'1-3'); assert.equal(s.focus,'terminal');
  assert.equal(s.sessions.find(s => s.group === 1 && s.id === 3).panes.length,1);
}));

test('right-click menus bind to worktrees and terminals, switch start/stop and dismiss without opening', () => fixture(async page => {
  const initial = await snapshot(page);
  await page.getByTestId('gg-sidebar-group-1-session-3-row').click({button:'right'}); await audit(page);
  let s = await snapshot(page); assert.equal(s.open,initial.open); assert.equal(s.modal.target,'1-3');
  assert.equal(await page.getByTestId('gg-menu-stop-label').innerText(),'Start');
  for (const id of ['gg-menu-new','gg-menu-delete','gg-menu-open']) assert.equal(await page.getByTestId(id).count(),1);
  assert.equal(await page.getByTestId('gg-menu-quit').count(),0);
  await click(page,'gg-menu-stop');
  assert.equal((await snapshot(page)).open,'1-3');
  await page.getByTestId('gg-sidebar-group-1-session-3-row').click({button:'right'});
  assert.equal(await page.getByTestId('gg-menu-stop-label').innerText(),'Stop');
  await click(page,'gg-menu-stop'); assert.equal((await snapshot(page)).modal.target,'1-3');
  await click(page,'gg-confirm-yes'); assert.equal((await snapshot(page)).open,null);
  await page.getByTestId('gg-sidebar-group-3-session-1-row').click({button:'right'});
  await click(page,'gg-menu-delete'); await click(page,'gg-confirm-yes');
  s = await snapshot(page); assert.ok(!s.sessions.some(s => s.group === 3 && s.id === 1));
  assert.deepEqual(s.files,initial.files); assert.deepEqual(s.branches,initial.branches);
  await page.getByTestId('gg-sidebar-group-2-session-3-row').click({button:'right'});
  assert.equal(await page.getByTestId('gg-menu-delete').count(),0);
  await page.keyboard.press('Escape');
  await page.getByTestId('gg-sidebar-group-1-session-2-row').click({button:'right'});
  await page.getByTestId('gg-dialog-wrap').click({position:{x:800,y:20}});
  assert.equal((await snapshot(page)).modal,null); assert.equal((await snapshot(page)).open,null);
}));

test('folder keyboard focus offers group actions and creates a plain shell or a Git worktree', () => fixture(async page => {
  await press(page,'ArrowUp'); await press(page,'ArrowUp');
  assert.equal((await snapshot(page)).selected,'group-1');
  assert.equal(await page.getByTestId('gg-sidebar-group-1-heading').getAttribute('aria-selected'),'true');
  await press(page,'Enter'); assert.equal((await snapshot(page)).modal.target,'group-1');
  await click(page,'gg-menu-terminal');
  let s = await snapshot(page), created = s.sessions.at(-1);
  assert.equal(created.kind,'terminal'); assert.equal(created.path,'/workspace/atlas');
  assert.equal(created.branch,''); assert.equal(created.template,'shell'); assert.equal(created.git,'n/a');
  assert.equal(s.open,`1-${created.id}`);
  assert.equal(await page.getByTestId(`gg-sidebar-group-1-session-${created.id}-row`).count(),1);
  await page.getByTestId('gg-sidebar-group-1-heading').click({button:'right'});
  await click(page,'gg-menu-new');
  assert.equal((await snapshot(page)).modal.path,'/workspace/atlas');
  await page.getByTestId('gg-create-name').fill('feat/from-folder'); await click(page,'gg-create-submit');
  await waitCreated(page,'feat/from-folder');
  created = (await snapshot(page)).sessions.at(-1);
  assert.equal(created.kind,'worktree'); assert.equal(created.path,'/workspace/atlas__sessions/feat~from-folder');
  await page.getByTestId('gg-sidebar-group-3-heading').click({button:'right'});
  assert.equal(await page.getByTestId('gg-menu-new').count(),1);
  assert.equal(await page.getByTestId('gg-menu-terminal').count(),1);
  assert.equal(await page.getByTestId('gg-menu-stop').count(),1);
  assert.equal(await page.getByTestId('gg-menu-delete').count(),1);
}));

test('relative paths require confirmation on the selected Git project', () => fixture(async page => {
  const heading = page.getByTestId('gg-sidebar-group-2-heading');
  await heading.click({button:'right'});
  await click(page,'gg-menu-relative-paths');
  assert.match(await page.getByTestId('gg-dialog-target').innerText(), /beacon/);
  assert.match(await page.getByTestId('gg-dialog-compatibility').innerText(), /2\.48/);
  await page.keyboard.press('Escape');
  assert.deepEqual((await snapshot(page)).relativeWorkspaces, []);
  await press(page,'r');
  await click(page,'gg-confirm-yes');
  assert.deepEqual((await snapshot(page)).relativeWorkspaces, [2]);
  await page.getByTestId('gg-sidebar-group-3-heading').click({button:'right'});
  assert.equal(await page.getByTestId('gg-menu-relative-paths').count(), 0);
}));

test('bulk actions retain files and branches, preserve other groups and keep confirmation targets fixed', () => fixture(async page => {
  const initial = await snapshot(page);
  await page.getByTestId('gg-sidebar-group-3-heading').click({button:'right'});
  await click(page,'gg-menu-stop');
  assert.deepEqual((await snapshot(page)).modal.targets,['3-1','3-3']);
  await click(page,'gg-confirm-yes');
  let s = await snapshot(page); assert.equal(s.open,initial.open);
  assert.ok(s.sessions.filter(s => s.group === 3).every(s => s.panes.length === 0));
  assert.equal(s.sessions.filter(s => s.group === 3).length,3); assert.deepEqual(s.files,initial.files);
  await page.getByTestId('gg-sidebar-group-1-heading').click({button:'right'});
  await click(page,'gg-menu-delete');
  const targets = (await snapshot(page)).modal.targets;
  await page.evaluate(() => {
    const state = GG.state, source = GG.session('1-3');
    state.sessions.push({...source,id:99,name:'new-after-confirmation',panes:[],layout:null});
    state.sessions = state.sessions.filter(s => GG.key(s) !== '1-3');
  });
  await click(page,'gg-confirm-yes'); s = await snapshot(page);
  assert.ok(s.sessions.some(s => s.group === 1 && s.id === 99));
  assert.ok(s.sessions.every(s => !targets.includes(`${s.group}-${s.id}`)));
  assert.deepEqual(s.sessions.filter(s => s.group === 2),initial.sessions.filter(s => s.group === 2));
  assert.deepEqual(s.branches,initial.branches); assert.ok(s.files.includes('/workspace/atlas'));
  assert.equal(s.open,null); assert.equal(s.selected,'group-1');
  assert.equal(await page.getByTestId('gg-sidebar-group-1-heading').count(),1);
}));

test('bulk deletion checks modified worktrees before any stop and preserves protected source files', () => fixture(async page => {
  const initial = await snapshot(page);
  await page.getByTestId('gg-sidebar-group-2-heading').click({button:'right'});
  await click(page,'gg-menu-delete'); await click(page,'gg-confirm-yes');
  let s = await snapshot(page); assert.deepEqual(s.sessions,initial.sessions); assert.deepEqual(s.files,initial.files);
  await page.evaluate(() => {GG.session('2-2').git = 'clean';});
  await page.getByTestId('gg-sidebar-group-2-heading').click({button:'right'});
  await click(page,'gg-menu-delete'); await click(page,'gg-confirm-yes');
  s = await snapshot(page); assert.ok(s.sessions.every(s => s.group !== 2));
  assert.ok(s.files.includes('/workspace/beacon')); assert.deepEqual(s.branches,initial.branches);
  assert.equal(s.open,initial.open);
}));

test('Up enters search, search hints are exclusive, Down leaves and Escape restores the open selection', () => fixture(async page => {
  await press(page,'ArrowUp'); await press(page,'ArrowUp'); assert.equal((await snapshot(page)).selected,'group-1');
  await page.keyboard.press('ArrowUp'); await audit(page);
  assert.equal((await snapshot(page)).searching,true);
  assert.equal(await page.getByTestId('gg-search-input').evaluate(n => n === document.activeElement),true);
  assert.deepEqual(await page.getByTestId('gg-sidebar-footer').locator('button').evaluateAll(nodes => nodes.map(n => n.dataset.testid)),['gg-search-clear','gg-search-down']);
  await page.getByTestId('gg-search-input').fill('no matching session');
  await click(page,'gg-search-clear');
  assert.equal((await snapshot(page)).query,''); assert.equal((await snapshot(page)).searching,true);
  assert.equal(await page.getByTestId('gg-search-input').evaluate(n => n === document.activeElement),true);
  await page.getByTestId('gg-search-input').fill('scratch');
  await page.keyboard.press('ArrowDown'); await audit(page);
  let s = await snapshot(page); assert.equal(s.searching,false); assert.equal(s.query,'scratch');
  assert.equal(await page.getByTestId('gg-session-list').evaluate(n => n === document.activeElement),true);
  await page.keyboard.press('Escape');
  s = await snapshot(page); assert.equal(s.query,''); assert.equal(s.searching,false); assert.equal(s.selected,s.open); assert.equal(s.focus,'list');
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('beacon');
  await page.keyboard.press('Escape'); assert.equal((await snapshot(page)).selected,'1-2');
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('atlas');
  await click(page,'gg-search-down'); assert.equal((await snapshot(page)).searching,false);
  await page.getByTestId('gg-search-input').click();
  assert.equal((await snapshot(page)).searching,true);
  await page.keyboard.press('Escape');
  await page.getByTestId('gg-size').selectOption('99x24');
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('nothing'); await page.keyboard.press('Escape');
  assert.equal(await page.getByTestId('gg-sidebar').count(),1); assert.equal((await snapshot(page)).selected,'1-2');
}));

test('right-click menus fit short Russian and compact windows and support keyboard actions', () => fixture(async page => {
  await scenario(page,'long');
  for (const size of ['120x15','99x24']) {
    await page.getByTestId('gg-size').selectOption(size);
    await page.getByTestId('gg-sidebar-group-1-heading').click({button:'right'}); await audit(page);
    const menu = await page.getByTestId('gg-dialog').boundingBox(), screen = await page.getByTestId('gg-terminal').boundingBox();
    assert.ok(menu.x >= screen.x && menu.y >= screen.y);
    assert.ok(menu.x+menu.width <= screen.x+screen.width && menu.y+menu.height <= screen.y+screen.height-19);
    await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
    assert.equal((await snapshot(page)).sessions.at(-1).kind,'terminal');
    if ((await snapshot(page)).focus === 'terminal') await click(page,'gg-next-panel');
  }
  await press(page,'Shift+F10'); assert.equal((await snapshot(page)).modal.type,'actions');
  await page.keyboard.press('Escape');
}));

test('folder and session focus stays visible through short lists and panel changes leave search editing', () => fixture(async page => {
  for (const size of ['160x42','100x16','120x15']) {
    await scenario(page,'long'); await page.getByTestId('gg-size').selectOption(size);
    await press(page,'ArrowUp'); await press(page,'ArrowUp');
    const items = await page.evaluate(() => GG.items());
    for (const id of items) {
      assert.equal((await snapshot(page)).selected,id);
      const row = await page.locator(`[id="row-${id}"]`).boundingBox();
      const list = await page.getByTestId('gg-session-list').boundingBox();
      assert.ok(row.y >= list.y-1 && row.y+row.height <= list.y+list.height+1,`${size}: ${id} is outside the list`);
      await page.keyboard.press('ArrowDown');
    }
    assert.equal((await snapshot(page)).open,'1-2');
  }
  await scenario(page,'workspace'); await click(page,'gg-sidebar-group-1-session-2-row'); await click(page,'gg-next-panel'); await press(page,'/');
  await page.getByTestId('gg-search-input').fill('atlas');
  await click(page,'gg-next-panel');
  let s = await snapshot(page); assert.equal(s.focus,'terminal'); assert.equal(s.searching,false); assert.equal(s.query,'atlas');
  assert.equal(await page.getByTestId(`gg-pane-${s.activePane}-input`).evaluate(n => n === document.activeElement),true);
  await click(page,'gg-next-panel');
  await press(page,'/'); await page.getByTestId('gg-search-input').fill('auth');
  await page.getByTestId('gg-pane-2-title').click();
  s = await snapshot(page); assert.equal(s.focus,'terminal'); assert.equal(s.searching,false);
  assert.equal(await page.getByTestId(`gg-pane-${s.activePane}-input`).evaluate(n => n === document.activeElement),true);
}));


test('empty dashboard adds exact-path sections, deduplicates and removes only empty sections', () => fixture(async page => {
  await scenario(page,'empty');
  assert.equal((await snapshot(page)).sessions.length,0);
  await page.getByTestId('gg-size').selectOption('99x24');
  await press(page,'Escape'); await click(page,'gg-next-panel');
  assert.equal(await page.getByTestId('gg-sidebar').isVisible(),true);
  await press(page,'+');
  await page.getByTestId('gg-section-path').fill('/missing'); await click(page,'gg-section-submit');
  assert.match(await page.getByTestId('gg-dialog-error').innerText(),/not found/);
  await page.getByTestId('gg-section-path').fill('/workspace/atlas/src'); await click(page,'gg-section-submit');
  let s = await snapshot(page); assert.equal(s.groups.length,1); assert.equal(s.sessions.length,0); assert.equal(s.open,null);
  const src = s.groups[0]; assert.equal(src.path,'/workspace/atlas/src');
  await press(page,'+'); await click(page,'gg-section-submit'); assert.equal((await snapshot(page)).groups.length,1);
  await press(page,'+'); await page.getByTestId('gg-section-path').fill('/workspace/atlas'); await click(page,'gg-section-submit');
  s = await snapshot(page); const root = s.groups.find(g => g.path === '/workspace/atlas'); assert.equal(s.groups.length,2);
  await press(page,'x'); assert.equal((await snapshot(page)).groups.length,1);
  await press(page,'t'); s = await snapshot(page); assert.equal(s.sessions[0].path,src.path);
  await click(page,'gg-next-panel'); await click(page,`gg-sidebar-group-${src.id}-heading`); await press(page,'x');
  assert.equal((await snapshot(page)).groups.length,1);
  await press(page,'n'); await page.getByTestId('gg-create-name').fill('feat/nested'); await click(page,'gg-create-submit');
  await waitCreated(page,'feat/nested');
  const worktree = (await snapshot(page)).sessions.find(s => s.name === 'feat/nested');
  assert.equal(worktree.path,'/workspace/atlas__sessions/feat~nested'); assert.equal(worktree.group,src.id);
  assert.notEqual(root.id,src.id);
}));

test('copy progress survives detach and completion preserves later terminal focus', () => fixture(async page => {
  await scenario(page,'copy'); await press(page,'n');
  await page.getByTestId('gg-create-name').fill('background'); await click(page,'gg-create-submit');
  const job = (await snapshot(page)).jobs[0];
  const row = page.getByTestId(`gg-sidebar-group-1-session-${job.id}-row`);
  assert.match(await row.innerText(), /Copying/);
  assert.equal((await snapshot(page)).sessions.some(s => s.name === 'background'),false);
  await press(page,'q'); await click(page,'gg-resume');
  assert.match(await row.innerText(), /Copying/);
  await page.getByTestId('gg-size').selectOption('99x24');
  await click(page,'gg-sidebar-group-1-session-2-row');
  assert.match(await page.getByTestId('gg-status-copy').innerText(), /Copying/);
  await click(page,'gg-simulate');
  const state = await snapshot(page);
  assert.equal(state.jobs.length,0); assert.equal(state.open,'1-2'); assert.equal(state.focus,'terminal');
  assert.equal(state.sessions.find(s => s.name === 'background').panes.length,1);
}));

test('copy failure retains its draft and retry opens the newly ready worktree', () => fixture(async page => {
  await scenario(page,'copy-error'); await press(page,'n');
  await page.getByTestId('gg-create-name').fill('retry'); await page.getByTestId('gg-create-branch').fill('feat/retry');
  await click(page,'gg-create-submit'); await click(page,'gg-simulate');
  assert.match(await page.getByTestId('gg-details-state-value').innerText(), /Cannot read/);
  await press(page,'Enter');
  assert.equal(await page.getByTestId('gg-create-name').inputValue(),'retry');
  assert.equal(await page.getByTestId('gg-create-branch').inputValue(),'feat/retry');
  await click(page,'gg-create-submit'); await click(page,'gg-simulate');
  const state = await snapshot(page);
  assert.equal(state.jobs.length,0); assert.equal(state.focus,'terminal'); assert.equal(state.open,state.selected);
  assert.equal(state.sessions.at(-1).name,'retry');
}));

/* Browser-local session model. It never invokes Git, tmux or a shell. */
(() => {
  'use strict';
  const templates = ['shell', 'codex', 'claude', 'npm-dev'];
  const scenarios = [
    ['workspace', 'Workspace', 'Рабочая среда', 'Click a row to open it. Use arrows to browse folders and sessions, hover for details, or right-click for actions.', 'Клик открывает терминал. Стрелки перемещают фокус по папкам и сессиям, наведение показывает детали, правая кнопка открывает действия.'],
    ['create', 'Create a Git session', 'Создание Git-сессии', 'Press N. Enter clone1 to start with detached HEAD. Tab opens templates; Esc restores your previous choice.', 'Нажмите N. Введите clone1, чтобы начать без ветки. Tab открывает шаблоны; Esc возвращает прежний выбор.'],
    ['copy', 'Background worktree copy', 'Фоновое копирование', 'Create a worktree with N, switch terminals, then use Simulate event to finish copying.', 'Создайте worktree через N, переключитесь в другой терминал и завершите копирование кнопкой «Смоделировать событие».'],
    ['copy-error', 'Copy failure and retry', 'Ошибка копирования и повтор', 'Create a worktree and simulate a copy error. Press Enter on its row to retry, then simulate completion.', 'Создайте worktree и смоделируйте ошибку копирования. Нажмите Enter на строке, повторите создание и смоделируйте завершение.'],
    ['directory', 'Ordinary directory', 'Обычная директория', 'Create a session with the default name. Duplicate names are allowed. Delete removes the record and preserves the directory.', 'Создайте сессию с исходным именем. Одинаковые имена допустимы. Удаление записи сохраняет директорию.'],
    ['split', 'Terminal placement', 'Размещение терминалов', 'Press T and choose Below or Right. Cycle panels, switch sessions, then return: output and layout remain.', 'Нажмите T и выберите Внизу или Справа. Переключите панели и сессии: вывод и расположение сохраняются.'],
    ['lifecycle', 'Stop, start and delete', 'Остановка, запуск, удаление', 'Stop this background session while another remains open. Start opens a new shell. Delete preserves its Git branch.', 'Остановите фоновую сессию: открытый терминал останется. Запуск создаёт новый процесс. Удаление сохраняет Git-ветку.'],
    ['dirty', 'Modified worktree', 'Изменённая рабочая копия', 'Press D and confirm. Changes block deletion; the record, terminals and files remain.', 'Нажмите D и подтвердите. Изменения блокируют удаление: запись, терминалы и файлы сохраняются.'],
    ['detached-commit', 'Detached commit', 'Коммит без ветки', 'Press D and confirm. The worktree keeps a commit outside branch and tag history.', 'Нажмите D и подтвердите. В дереве есть коммит, который не входит в историю веток и тегов.'],
    ['protected', 'Protected source', 'Защищённый источник', 'Open Actions: Delete is absent. Stop is available and retires the legacy source record without removing its files.', 'Откройте Действия: удаления нет. Остановка доступна и удаляет старую запись источника, сохраняя файлы.'],
    ['search', 'Search: eleven → ten', 'Поиск: одиннадцать → десять', 'Search is available with eleven candidates. Delete the selected stopped session: the query clears and search disappears.', 'При одиннадцати записях доступен поиск. Удалите выбранную остановленную сессию: поиск очистится и исчезнет.'],
    ['no-matches', 'No search results', 'Нет результатов поиска', 'An eligible query matches nothing. Esc clears it. Toggle Active to see the threshold use activity-filtered candidates.', 'Поиск доступен, но совпадений нет. Esc очищает запрос. Фильтр активности меняет число кандидатов.'],
    ['settings', 'Language and templates', 'Язык и шаблоны', 'Press comma. Settings belong to the open project, even while browsing another group. Save or cancel; existing templates remain.', 'Нажмите запятую. Настройки относятся к открытому проекту, даже при выборе другой группы. Сохраните или отмените изменения.'],
    ['unavailable', 'Unavailable project', 'Недоступный проект', 'Open the selected session. Its server is unavailable; the previous terminal remains open and healthy groups stay usable.', 'Откройте выбранную сессию. Сервер недоступен; прежний терминал остаётся открыт, другие группы доступны.'],
    ['missing-template', 'Missing saved template', 'Отсутствующий шаблон', 'Start the stopped session. Its saved template is missing; no replacement shell or terminal switch occurs.', 'Запустите остановленную сессию. Сохранённый шаблон отсутствует; новый терминал не создаётся.'],
    ['vanished', 'Confirmation target disappears', 'Цель подтверждения исчезла', 'Press D, then Simulate event to remove that record externally. Confirm: the neighboring session is unaffected.', 'Нажмите D, затем Смоделировать событие для внешнего удаления записи. Подтвердите: соседняя сессия не пострадает.'],
    ['empty', 'Fresh dashboard', 'Новый dashboard', 'Add a section, choose a folder, then create a terminal or worktree. Launch creates no sessions.', 'Добавьте раздел, выберите папку и создайте терминал или worktree. Запуск не создаёт сессии.'],
    ['stopped', 'All sessions stopped', 'Все сессии остановлены', 'There are no live terminals. Open any persistent session to start its saved template.', 'Нет работающих терминалов. Откройте постоянную сессию для запуска сохранённого шаблона.'],
    ['shell-exit', 'Persistent shell exit', 'Завершение постоянного shell', 'Simulate shell exit. The session remains available for restarting; Q preserves running sessions.', 'Смоделируйте выход из shell. Сессия остаётся доступной для запуска; Q сохраняет работающие сессии.'],
    ['notices', 'Notification deadlines', 'Срок уведомлений', 'Simulate events: success, replacement success, persistent error. Successful notices expire five seconds after completion.', 'Смоделируйте события: успех, новый успех, постоянная ошибка. Успешные уведомления исчезают через пять секунд.'],
    ['duplicates', 'Duplicate group names', 'Одинаковые имена групп', 'Paths distinguish equal group names. Equal worktree names address separate group/session pairs.', 'Пути различают группы с одинаковыми именами. Одинаковые имена worktree относятся к разным парам группа/сессия.'],
    ['compact', 'Compact and short windows', 'Узкие и низкие окна', 'Use Next panel to open the full-screen list. Esc closes it. Try 99×24 and 120×15; pane titles and status remain.', 'Следующая панель открывает список на весь экран. Esc закрывает его. Попробуйте 99×24 и 120×15: заголовки и статус остаются.'],
    ['long', 'Long names and Russian', 'Длинные имена и русский язык', 'Inspect long names at different sizes. Selection, group headings, actions and details stay inside the terminal.', 'Проверьте длинные имена при разных размерах. Выделение, заголовки, действия и детали остаются внутри терминала.'],
    ['agent', 'Agent activity', 'Состояния агента', 'Simulate events to cycle agent states. Completed means a completed turn; activity still depends on live terminals.', 'Смоделируйте события для смены статуса агента. Завершение означает конец хода; активность определяется работающими терминалами.'],
    ['no-color', 'Without color', 'Без цвета', 'Focus, open session and activity remain identifiable by marks and borders with color disabled.', 'Фокус, открытая сессия и активность различимы по меткам и рамкам при отключённых цветах.']
  ];
  let state;
  const key = s => `${s.group}-${s.id}`;
  const group = id => state.groups.find(g => g.id === id);
  const groupKey = g => `group-${g.id}`;
  const selectedGroup = (id = state.selected) => group(session(id)?.group) || state.groups.find(g => groupKey(g) === id);
  const isWorktree = s => ['worktree', 'source'].includes(s.kind);
  const session = id => state.sessions.find(s => key(s) === id) || state.jobs.find(s => key(s) === id);
  const active = s => s.panes.length > 0;
  const effective = (id = state.context) => ({...state.global, ...(state.groups.find(g => g.workspace === group(id)?.workspace && g.config)?.config || {})});
  const text = pair => pair[effective().language === 'ru' ? 1 : 0];
  function pane(s, title = s.template) {
    const id = String(state.nextPane++);
    let lines;
    if (title === 'codex') lines = [
      '╭─ Codex ─────────────────────────────────────╮',
      '│  model: gpt-5     /model to change          │',
      '╰────────────────────────────────────────────╯', '',
      `directory: ${s.path}`, '',
      '› Add session authentication and refresh tokens', '',
      '• Reading the authentication handlers',
      '  Read src/auth/session.go',
      '  Read src/auth/session_test.go', '',
      '• The session store already handles expiry.',
      '  Adding a refresh path and coverage for reuse.', '',
      '  + src/auth/refresh.go',
      '  ~ src/auth/session_test.go', '',
      '  Working · 2 files changed', ''
    ];
    else if (title === 'claude') lines = ['Claude Code', '', `directory: ${s.path}`, '', '› Review the cache invalidation flow', '', 'Ready for your next instruction.'];
    else if (title === 'npm-dev') lines = ['$ npm run dev', '', '> dev', '> vite', '', '  VITE  ready', '', '  Local: http://localhost:5173/'];
    else lines = [`${s.path}  ${s.branch || ''}`, '', '$ pwd', s.path, ...(s.branch ? ['', '$ git status --short', ...(s.git === 'modified' ? [' M src/auth/session.go'] : [])] : []), '', '$'];
    return {id, title: title === 'shell' ? `zsh · ${s.name}` : title, lines, input: ''};
  }
  function make(groupId, id, name, kind, running, template = 'shell', agent = 'shell', git = 'clean', owner = '') {
    const s = {group: groupId, workspace: group(groupId).workspace, id, name, kind, template, agent, git, owner, protected: kind === 'source', branch: ['source', 'worktree'].includes(kind) ? name : '', panes: [], layout: null};
    s.path = kind === 'worktree' ? `${group(groupId).root || group(groupId).path}__sessions/${name.replaceAll('/','~')}` : group(groupId).path;
    if (kind === 'temporary' && owner === 'run-1') s.path += '/src';
    if (running) { s.panes.push(pane(s)); s.layout = {pane: s.panes[0].id}; }
    return s;
  }
  function reset(scenario = 'workspace') {
    state = {
      groups: [{id: 1, workspace: 1, name: 'atlas', path: '/workspace/atlas', type: 'git'}, {id: 2, workspace: 2, name: 'beacon', path: '/workspace/beacon', type: 'git'}, {id: 3, workspace: 3, name: 'scratch', path: '/workspace/scratch', type: 'directory'}],
      sessions: [], jobs: [], autoOpen: null, relativeWorkspaces: [], nextGroup: 4, launchCWD: '/workspace/atlas/src', nextPane: 1, nextSession: 20, nextLayout: 1, invocation: 'run-1', run: 1,
      global: {language: 'en', template: 'shell'}, context: 1, selected: '1-2', open: '1-2', focus: 'list',
      query: '', searching: false, onlyActive: false, modal: null, notice: null, detached: false,
      scenario, eventIndex: 0, color: scenario !== 'no-color', size: scenario === 'compact' ? '99x24' : 'auto',
      last: ['Click a session to open it; right-click a session or folder for actions.', 'Клик открывает сессию; правая кнопка на сессии или папке открывает действия.']
    };
    state.sessions = [
      make(1, 1, 'atlas', 'terminal', true, 'shell', 'shell', 'n/a'),
      make(1, 2, 'feat/auth', 'worktree', true, 'codex', 'working'),
      make(1, 3, 'fix/session-focus', 'worktree', false),
      make(1, 4, 'refactor/cache', 'worktree', true, 'claude', 'ready'),
      make(1, 5, 'release/next', 'worktree', false, 'npm-dev'),
      make(2, 1, 'beacon', 'terminal', true, 'shell', 'shell', 'n/a'),
      make(2, 2, 'feat/auth', 'worktree', true, 'codex', 'waiting', 'modified'),
      make(2, 3, 'main', 'source', true), make(2, 4, 'docs/api', 'worktree', false),
      make(3, 1, 'scratch', 'terminal', true, 'shell', 'shell', 'n/a'),
      make(3, 2, 'scratch', 'terminal', false, 'shell', 'stopped', 'n/a'),
      make(3, 3, 'notes', 'terminal', true, 'shell', 'shell', 'n/a')
    ];
    state.branches = state.sessions.filter(s => s.branch).map(s => `${s.workspace}:${s.branch}`);
    state.branches.push('1:release/stable');
    state.files = [...new Set(state.sessions.map(s => s.path))];
    if (scenario === 'directory') {state.selected = state.open = '3-1'; state.context = 3;}
    if (scenario === 'lifecycle') state.selected = '1-4';
    if (scenario === 'detached-commit') {
      const s = session('1-3');
      s.branch = ''; s.unreferencedHead = true; state.selected = key(s);
    }
    if (scenario === 'dirty' || scenario === 'unavailable') state.selected = '2-2';
    if (scenario === 'protected') state.selected = '2-3';
    if (scenario === 'settings') state.selected = '3-1';
    if (scenario === 'search') {state.sessions = state.sessions.filter(s => key(s) !== '3-3'); state.selected = '1-5';}
    if (scenario === 'no-matches') {state.query = 'not-a-session'; state.selected = null;}
    if (scenario === 'unavailable') group(2).error = true;
    if (scenario === 'missing-template') {state.selected = '1-3'; session('1-3').template = 'removed-template';}
    if (scenario === 'vanished') state.selected = '1-3';
    if (scenario === 'empty') {state.sessions = []; state.groups = []; state.selected = state.open = null; state.context = null;}
    if (scenario === 'workspace') {state.open = null; state.selected = 'group-1'; state.context = null;}
    if (scenario === 'stopped') {
      state.sessions = state.sessions.filter(s => !['temporary', 'source'].includes(s.kind));
      state.sessions.forEach(s => {s.panes = []; s.layout = null;}); state.open = null;
    }
    if (scenario === 'shell-exit') state.selected = state.open = '1-1';
    if (scenario === 'duplicates') group(2).name = 'atlas';
    if (scenario === 'long') {session('1-2').name = 'feat/authentication-with-a-very-long-session-name'; state.global.language = 'ru';}
    state.activePane = session(state.open)?.panes[0]?.id || null;
    normalize(); return state;
  }
  const candidates = () => state.sessions.filter(s => !group(s.group).error && (!state.onlyActive || active(s)));
  const searchAvailable = () => candidates().length + (state.onlyActive ? 0 : state.jobs.length) > 10;
  const rows = () => candidates().concat(state.onlyActive ? [] : state.jobs).filter(s => `${s.name} ${s.branch} ${group(s.group).name} ${group(s.group).path}`.toLowerCase().includes(state.query.toLowerCase())).sort((a, b) => group(a.group).name.localeCompare(group(b.group).name) || a.group - b.group || a.id - b.id);
  const visibleGroups = () => [...state.groups].sort((a,b) => a.name.localeCompare(b.name) || a.id-b.id).filter(g => rows().some(s => s.group === g.id) || g.error || (!state.query && !state.onlyActive));
  const items = () => visibleGroups().flatMap(g => [groupKey(g), ...rows().filter(s => s.group === g.id).map(key)]);
  function normalize(oldIndex = 0) {
    if (!searchAvailable()) {state.query = ''; state.searching = false;}
    const visible = items();
    if (!visible.includes(state.selected)) state.selected = visible[Math.min(oldIndex, visible.length - 1)] || null;
  }
  function notice(en, ru, kind = 'error') {
    state.notice = {message: [en, ru], kind, until: kind === 'success' ? Date.now() + 5000 : null};
    state.last = [en, ru];
  }
  function reveal() {
    if (session(state.open)) {
      if (!rows().some(s => key(s) === state.open)) {state.onlyActive = false; state.query = ''; state.searching = false;}
      state.selected = state.open;
    }
    state.focus = 'list';
  }
  function open(id) {
    const s = session(id);
    if (!s) return;
    if (s.job) {
      state.focus = 'list';
      if (s.job.error) state.modal = {...s.job.draft, retry: id, error: s.job.error};
      return;
    }
    if (group(s.group).error) {notice('Project server is unavailable. The previous terminal remains open.', 'Сервер проекта недоступен. Прежний терминал остаётся открыт.'); return;}
    if (!active(s)) {
      if (!templates.includes(s.template)) {notice(`Template not found: ${s.template}`, `Шаблон не найден: ${s.template}`); return;}
      s.panes = [pane(s)]; s.layout = {pane: s.panes[0].id}; s.agent = s.template === 'codex' ? 'ready' : 'shell';
    }
    state.open = id; state.context = s.group; state.activePane = s.panes[0].id; state.focus = 'terminal'; state.searching = false;
    state.last = [`Opened ${s.name}. Existing terminal output is preserved.`, `Открыта ${s.name}. Вывод работающих терминалов сохранён.`];
  }
  function create(id = state.selected) {
    const target = session(id), g = selectedGroup(id);
    if (!g) return;
    state.modal = {type: 'create', kind: g.type === 'git' ? 'worktree' : 'terminal', group: g.id, path: target?.path || g.path, base: target && isWorktree(target) ? target.branch || 'detached' : 'main', branch: '', name: g.type === 'directory' ? g.name : '', template: effective(g.id).template, picking: false, choice: 0, error: null};
  }
  async function submitCreate() {
    const m = state.modal, g = group(m.group), worktree = m.kind === 'worktree', name = m.name.trim() || (!worktree ? g.name : '');
    const fail = (en, ru) => {m.error = [en, ru];};
    if (!name) return fail('Worktree name is required.', 'Имя worktree обязательно.');
    if (g.error) return fail('Project is unavailable.', 'Проект недоступен.');
    if (!templates.includes(m.template)) return fail(`Template not found: ${m.template}`, `Шаблон не найден: ${m.template}`);
    let longFolder;
    if (worktree) {
      const invalid = name => !name || name === 'HEAD' || /\s|\.\.|[~^:?*\[\\\x00-\x1f\x7f]|@\{|\/\/|^[-/]|[/.]$/.test(name) || name.split('/').some(p => p.startsWith('.') || p.endsWith('.lock'));
      if (invalid(name)) return fail('Invalid worktree name.', 'Недопустимое имя worktree.');
      const reserved = state.sessions.concat(state.jobs.filter(s => !s.job.error));
      if (reserved.some(s => s.workspace === g.workspace && isWorktree(s) && s.name === name)) return fail('Worktree name already exists.', 'Имя worktree уже занято.');
      const branch = m.branch.trim();
      if (branch && invalid(branch)) return fail('Invalid Git branch name.', 'Недопустимое имя Git-ветки.');
      if (branch && (branch === 'main' || reserved.some(s => s.workspace === g.workspace && s.branch === branch))) return fail('Branch is already checked out.', 'Ветка уже занята другим worktree.');
      const bytes = new TextEncoder().encode(name);
      if (bytes.length > 255) {
        const digest = await crypto.subtle.digest('SHA-256', bytes);
        if (state.modal !== m || m.name.trim() !== name) return;
        longFolder = '~' + [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
      }
    }
    const s = make(g.id, state.nextSession++, name, m.kind, false, m.template, m.template === 'codex' ? 'ready' : 'shell', worktree ? 'clean' : 'n/a');
    if (longFolder) s.path = `${g.root || g.path}__sessions/${longFolder}`;
    if (!worktree) s.path = m.path;
    s.branch = worktree ? m.branch.trim() : '';
    s.base = m.base;
    state.query = ''; state.searching = false; state.onlyActive = false; state.selected = key(s); state.modal = null;
    if (worktree) {
      const manual = ['copy','copy-error'].includes(state.scenario);
      s.job = {draft: {...m, error: null}, error: null, due: manual ? null : Date.now() + 800};
      state.jobs = state.jobs.filter(other => key(other) !== m.retry);
      state.jobs.push(s);
      state.autoOpen = key(s); state.focus = 'list';
    } else {
      publishSession(s);
      open(key(s));
    }
  }
  function publishSession(s) {
    if (s.branch && !state.branches.includes(`${s.workspace}:${s.branch}`)) state.branches.push(`${s.workspace}:${s.branch}`);
    if (!state.files.includes(s.path)) state.files.push(s.path);
    s.panes = [pane(s)]; s.layout = {pane: s.panes[0].id};
    state.sessions.push(s);
  }
  function checkCreationFocus() {
    if (state.selected !== state.autoOpen || state.focus !== 'list' || state.modal || state.searching || state.detached) state.autoOpen = null;
  }
  function finishCopy(s, failed = false) {
    checkCreationFocus();
    if (failed) {
      s.job.error = ['Cannot read ignored file.', 'Не удалось прочитать игнорируемый файл.'];
      s.job.due = null;
      return;
    }
    delete s.job;
    state.jobs = state.jobs.filter(other => other !== s);
    publishSession(s);
    if (state.autoOpen === key(s)) open(key(s));
    state.last = [`Created ${s.name} with ${s.template}.`, `Создана ${s.name}, шаблон ${s.template}.`];
  }
  function addTerminal(id) {
    const g = selectedGroup(id);
    if (!g || g.error) return notice('Project is unavailable.', 'Проект недоступен.');
    const s = make(g.id, state.nextSession++, g.name, 'terminal', false, 'shell', 'shell', 'n/a');
    state.sessions.push(s); state.query = ''; state.searching = false; state.onlyActive = false; state.selected = key(s);
    if (!state.files.includes(s.path)) state.files.push(s.path);
    open(key(s));
  }
  function confirm(action, id = state.selected, bulk = false) {
    const s = session(id), g = selectedGroup(id);
    if (!g || (!bulk && (!s || (action === 'delete' && s.protected)))) return;
    const targets = bulk ? state.sessions.filter(s => s.group === g.id && (action === 'delete' || active(s))).map(key) : [id];
    if (!targets.length) return;
    state.modal = {type: 'confirm', action, target: bulk ? null : id, targets, bulk, name: bulk ? g.name : s.name, group: g.id, kind: s?.kind};
  }
  function applyConfirmation() {
    if (state.modal.type === 'relative-paths') {
      const {workspace, root} = state.modal;
      state.modal = null;
      if (!state.relativeWorkspaces.includes(workspace)) state.relativeWorkspaces.push(workspace);
      notice(`Relative worktree paths enabled for ${root}`, `Относительные пути worktree включены для ${root}`, 'success');
      return;
    }
    const m = state.modal, targets = m.targets.map(session).filter(Boolean); state.modal = null;
    if (!targets.length) {notice('Session no longer exists. No other session was changed.', 'Сессия больше не существует. Другие сессии не изменены.'); return;}
    if (m.action === 'delete' && targets.some(s => s.kind === 'worktree' && state.jobs.some(job => !job.job.error && job.job.draft.path === s.path))) {
      notice('This worktree is a copy source. Retry after copying finishes.', 'Из этого worktree копируются файлы. Повторите удаление после копирования.'); return;
    }
    if (m.action === 'delete' && targets.some(s => s.kind === 'worktree' && !s.branch && s.unreferencedHead)) {
      notice('Save the detached commit with git switch -c <branch> before deleting the worktree.', 'Перед удалением сохраните коммит в ветке через git switch -c <ветка>.'); return;
    }
    if (m.action === 'delete' && targets.some(s => (!m.bulk && s.protected) || (s.kind === 'worktree' && s.git === 'modified'))) {
      notice('Protected worktrees or modified files prevent deletion. No sessions were changed.', 'Защищённые рабочие копии или изменённые файлы блокируют удаление. Сессии не изменены.'); return;
    }
    const index = Math.max(0, items().indexOf(state.selected));
    for (const s of targets) {
      s.panes = []; s.layout = null; s.agent = 'stopped';
      if (state.open === key(s)) {state.open = null; state.activePane = null; state.focus = 'list';}
      if (m.action === 'delete' || ['source', 'temporary'].includes(s.kind)) state.sessions = state.sessions.filter(t => t !== s);
      if (m.action === 'delete' && s.kind === 'worktree') state.files = state.files.filter(p => p !== s.path);
    }
    normalize(index);
    const label = m.bulk ? `${m.name} (${targets.length})` : m.name;
    const preservation = m.action !== 'delete' ? ['Files preserved.', 'Файлы сохранены.'] : m.bulk
      ? ['Git branches and ordinary/protected directories preserved.', 'Git-ветки, обычные и защищённые директории сохранены.']
      : m.kind === 'worktree' ? ['Git branches preserved.', 'Git-ветки сохранены.'] : ['Directory files preserved.', 'Файлы директории сохранены.'];
    const verb = m.action === 'delete' ? (m.bulk ? 'Удалены' : 'Удалена') : (m.bulk ? 'Остановлены' : 'Остановлена');
    notice(`${m.action === 'delete' ? 'Deleted' : 'Stopped'} ${label}. ${preservation[0]}`, `${verb} ${label}. ${preservation[1]}`, 'success');
  }
  function cancelSearch() {
    state.query = ''; state.searching = false;
    reveal(); normalize();
  }
  function leaveSearch() {
    state.searching = false; state.focus = 'list'; normalize();
  }
  function split(direction) {
    const s = session(state.modal.target); state.modal = null;
    if (!s) return notice('Session no longer exists.', 'Сессия больше не существует.');
    if (!active(s)) return open(key(s));
    const p = pane({...s, template: 'shell'}, 'shell');
    const last = s.panes.at(-1).id;
    function insert(node) {return node.pane === last ? {id: state.nextLayout++, direction, first: node, second: {pane: p.id}} : node.pane ? node : {...node, first: insert(node.first), second: insert(node.second)};}
    s.layout = insert(s.layout); s.panes.push(p); open(key(s)); state.activePane = p.id;
    state.last = [`Added a plain shell ${direction}. Startup commands were not repeated.`, `Добавлен обычный терминал ${direction === 'right' ? 'справа' : 'внизу'}. Команды шаблона не повторялись.`];
  }
  function cycle() {
    if (state.modal || state.detached) return;
    state.searching = false;
    const panes = session(state.open)?.panes || [];
    if (state.focus === 'list' && panes.length) {state.focus = 'terminal'; state.activePane = panes[0].id;}
    else {
      const index = panes.findIndex(p => p.id === state.activePane);
      if (index + 1 < panes.length) {state.focus = 'terminal'; state.activePane = panes[index + 1].id;}
      else reveal();
    }
  }
  function quit() {
        state.detached = true; state.modal = null; state.last = ['Dashboard detached. Persistent sessions and other invocations remain.', 'Dashboard закрыт. Постоянные сессии и другие запуски сохранены.'];
  }
  function resume() {
    state.run++; state.detached = false; state.open = null; state.activePane = null;
    state.context = null; state.focus = 'list'; state.selected = null;
    state.query = ''; state.searching = false; state.onlyActive = false; normalize();
  }
  const directories = [
    {path:'/'}, {path:'/workspace'}, {path:'/workspace/atlas', workspace:1, root:'/workspace/atlas', type:'git'},
    {path:'/workspace/atlas/src', workspace:1, root:'/workspace/atlas', type:'git'},
    {path:'/workspace/atlas/.config', workspace:1, root:'/workspace/atlas', type:'git'},
    {path:'/workspace/beacon', workspace:2, root:'/workspace/beacon', type:'git'},
    {path:'/workspace/scratch', workspace:3, type:'directory'}, {path:'/workspace/new folder', workspace:4, type:'directory'},
    {path:'/workspace/private', error:true}, {path:'/home'}, {path:'/home/demo'}
  ];
  function resolvePath(value) {
    value = value.replace(/^~(?=\/|$)/, '/home/demo');
    if (!value.startsWith('/')) value = state.launchCWD + '/' + value;
    const parts = [];
    for (const part of value.split('/')) {if (part === '..') parts.pop(); else if (part && part !== '.') parts.push(part);}
    return '/' + parts.join('/');
  }
  function browseSection(value) {
    const m = state.modal, path = resolvePath(value), dir = directories.find(d => d.path === path);
    if (!dir || dir.error) {m.error = !dir ? ['Directory not found.', 'Директория не найдена.'] : ['Directory is not accessible.', 'Нет доступа к директории.']; return;}
    m.path = m.directory = path; m.error = null; m.choice = 0;
    m.entries = [{name:'..',path:path.slice(0,path.lastIndexOf('/')) || '/'}, ...directories.filter(d => d.path !== path && (d.path.slice(0,d.path.lastIndexOf('/')) || '/') === path).sort((a,b) => a.path.localeCompare(b.path)).map(d => ({name:d.path.split('/').at(-1),path:d.path}))];
  }
  function submitSection() {
    const m = state.modal, path = resolvePath(m.path), dir = directories.find(d => d.path === path);
    if (!dir || dir.error) {browseSection(m.path); return;}
    let g = state.groups.find(g => g.path === path);
    if (!g) {
      g = {id:state.nextGroup++, workspace:dir.workspace || 100+state.nextGroup, name:path.split('/').at(-1) || '/', path, root:dir.root || path, type:dir.type || 'directory'};
      state.groups.push(g);
    }
    state.modal = null; state.selected = groupKey(g); state.focus = 'list';
    state.query = ''; state.searching = false; state.onlyActive = false;
  }
  function removeSection(id) {
    const g = selectedGroup(id);
    if (!g || g.error || state.sessions.some(s => s.group === g.id) || state.jobs.some(s => s.group === g.id && !s.job.error)) return;
    state.jobs = state.jobs.filter(s => s.group !== g.id);
    state.groups = state.groups.filter(other => other !== g);
    if (state.context === g.id) state.context = null;
    normalize();
  }
  function simulate() {
    if (['copy','copy-error'].includes(state.scenario)) {
      const job = state.jobs.find(s => !s.job.error);
      if (job) finishCopy(job, state.scenario === 'copy-error' && state.eventIndex++ === 0);
    } else if (state.scenario === 'vanished') {
      const target = state.modal?.target || '1-3'; state.sessions = state.sessions.filter(s => key(s) !== target); normalize();
      state.last = ['Confirmation target removed externally. Confirming cannot affect its neighbor.', 'Цель подтверждения удалена извне. Подтверждение не затронет соседа.'];
    } else if (state.scenario === 'shell-exit') {
      const s = session(state.open);
      if (s?.kind === 'terminal') {
        const exiting = state.activePane;
        s.panes = s.panes.filter(p => p.id !== exiting);
        function remove(node) {
          if (node.pane) return node.pane === exiting ? null : node;
          const first = remove(node.first), second = remove(node.second);
          return !first ? second : !second ? first : {...node, first, second};
        }
        s.layout = remove(s.layout);
        if (s.panes.length) {state.activePane = s.panes[0].id; state.last = ['Shell exited. Other terminals remain.', 'Терминал завершён. Другие терминалы сохранены.'];}
        else {
          state.open = null; state.activePane = null; state.focus = 'list'; normalize();
          state.last = ['Shell exited. The session can be started again.', 'Shell завершён. Сессию можно запустить снова.'];
        }
      }
    } else if (state.scenario === 'agent') {
      const codes = ['ready','working','waiting','done','interrupted','shell','unknown'];
      session('1-2').agent = codes[state.eventIndex++ % codes.length];
      state.last = [`Agent: ${session('1-2').agent}. Terminal activity is unchanged.`, `Агент: ${session('1-2').agent}. Активность терминала не изменилась.`];
    } else if (state.scenario === 'notices') {
      state.eventIndex++;
      if (state.eventIndex % 3 === 0) notice('Operation failed. This error remains visible.', 'Операция не выполнена. Ошибка остаётся видимой.');
      else notice(`Completed operation ${state.eventIndex}.`, `Операция ${state.eventIndex} выполнена.`, 'success');
    }
  }
  function action(name, id = state.selected) {
    const s = session(id), g = selectedGroup(id), folder = g && !s;
    if (state.detached) return;
    if (s?.job && ['new','terminal','stop','delete','split'].includes(name)) return;
    if (name === 'open') {if (folder) action('actions', id); else open(id);}
    if (name === 'new') create(id);
    if (name === 'add-section') {state.modal = {type:'section', path:state.launchCWD, directory:state.launchCWD, entries:[], choice:0, error:null}; browseSection(state.launchCWD);}
    if (name === 'remove-section') removeSection(id);
    if (name === 'relative-paths' && folder && g.type === 'git' && !g.error) {
      state.modal = {type:'relative-paths', workspace:g.workspace, root:g.root || g.path};
    }
    if (name === 'terminal') addTerminal(id);
    if (name === 'stop') {if (folder) confirm('stop', id, true); else if (s && !active(s)) open(id); else confirm('stop', id);}
    if (name === 'delete') confirm('delete', id, !!folder);
    if (name === 'split') {
      if (folder) addTerminal(id);
      else if (s) {if (!active(s)) open(id); else state.modal = {type: 'split', target: id, choice: 0};}
    }
    if (name === 'filter') {state.onlyActive = !state.onlyActive; normalize();}
    if (name === 'search' && searchAvailable()) {state.searching = true; state.focus = 'list';}
    if (name === 'actions') {state.searching = false; state.modal = {type: 'actions', target: id, choice: 0};}
    if (name === 'settings') state.modal = {type: 'settings', draft: {...effective()}, scope: 'global', group: state.context, field: 0};
    if (name === 'quit') quit();
  }
  function saveSettings() {
    const m = state.modal;
    if (m.scope === 'local' && group(m.group)?.type === 'git') group(m.group).config = {...m.draft}; else state.global = {...m.draft};
    state.modal = null; state.last = ['Settings saved. Running terminals and existing session templates are preserved.', 'Настройки сохранены. Работающие терминалы и шаблоны существующих сессий сохранены.'];
  }
  window.GG = {reset, get state() {return state;}, key, group, groupKey, selectedGroup, isWorktree, visibleGroups, items, session, active, effective, text, templates, scenarios, rows, candidates, searchAvailable, normalize, reveal, cancelSearch, leaveSearch, open, action, submitCreate, applyConfirmation, split, cycle, quit, resume, simulate, saveSettings, browseSection, submitSection, checkCreationFocus, finishCopy};
  reset();
})();

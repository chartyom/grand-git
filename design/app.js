(() => {
  'use strict';
  const G = window.GG;
  const root = document.getElementById('app');
  let cols = 160, lines = 34, timer, guideOpen = true, lastModal = '', previousFocus = '';
  const S = () => G.state;
  const t = (en, ru) => G.effective().language === 'ru' ? ru : en;
  const el = (tag, id, attrs = {}, ...children) => {
    if (!id) throw new Error(`Missing test ID for ${tag}`);
    const node = document.createElement(tag);
    node.dataset.testid = id;
    for (const [key, value] of Object.entries(attrs)) {
      if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
      else if (value !== false && value !== null && value !== undefined) node.setAttribute(key, value === true ? '' : String(value));
    }
    for (const child of children.flat(Infinity)) if (child !== null && child !== undefined && child !== false) node.append(child instanceof Node ? child : document.createTextNode(String(child)));
    return node;
  };
  const button = (id, label, fn, attrs = {}) => el('button', id, {type:'button', onclick: () => {fn(); render();}, ...attrs}, label);
  const shortcut = (prefix, key, label, fn, attrs = {}) => button(prefix, [el('kbd', `${prefix}-key`, {}, `[${key}]`), ' ', el('span', `${prefix}-label`, {}, label)], fn, attrs);
  const select = (id, value, entries, change, label) => {
    const n = el('select', id, {id:id.replace(/^gg-/, ''), 'aria-label':label, onchange: e => {change(e.target.value); render();}}, entries.map(([v, title]) => el('option', `${id}-option-${v}`, {value:v}, title)));
    n.value = value; return n;
  };
  const compact = () => cols < 100 || lines < 16;
  const act = name => {G.action(name);};
  function actions(id = S().modal?.target || S().selected, contextual = false) {
    const state = S(), s = G.session(id), g = G.selectedGroup(id), list = [];
    if (g && !s) {
      list.push(['new', 'N', g.type === 'git' ? t('New worktree session', 'Новая worktree-сессия') : t('New session','Новая сессия')]);
      list.push(['terminal', 'T', t('New terminal here', 'Новый терминал в папке')]);
      const sessions = state.sessions.filter(s => s.group === g.id);
      if (sessions.some(G.active)) list.push(['stop', 'S', t('Stop all sessions', 'Остановить все сессии')]);
      if (sessions.length) list.push(['delete', 'D', t('Delete all sessions', 'Удалить все сессии')]);
      else if (!g.error) list.push(['remove-section', 'X', t('Remove section', 'Убрать раздел')]);
    } else {
      if (s) list.push(['new', 'N', t('New session', 'Новая сессия')]);
      if (s) list.push(['open', 'Enter', t('Open', 'Открыть')], ['split', 'T', t('Add terminal', 'Ещё терминал')], ['stop', 'S', G.active(s) ? t('Stop', 'Остановить') : t('Start', 'Запустить')]);
      if (s && !s.protected) list.push(['delete', 'D', t('Delete', 'Удалить')]);
    }
    if (!contextual) {
      list.push(['add-section', '+', t('Add section', 'Добавить раздел')]);
      if (G.searchAvailable()) list.push(['search', '/', t('Search', 'Поиск')]);
      list.push(['filter', 'A', t('All / active', 'Все / активные')], ['settings', ',', t('Settings', 'Настройки')], ['quit', 'Q', t('Quit dashboard', 'Выйти из dashboard')]);
    }
    return list;
  }
  function contextMenu(e, id) {
    e.preventDefault();
    if (S().modal) return;
    const bounds = root.querySelector('[data-testid="gg-terminal"]').getBoundingClientRect();
    S().selected = id; S().focus = 'list'; G.action('actions', id);
    S().modal.anchor = {x:e.clientX-bounds.left, y:e.clientY-bounds.top};
    render();
  }
  function previewDetails(id) {
    const current = root.querySelector('[data-testid="gg-details"]');
    if (current) current.replaceWith(details(id));
  }
  function scaffold() {
    const state = S(), scenario = G.scenarios.find(row => row[0] === state.scenario);
    const title = el('header', 'gg-masthead', {class:'masthead'},
      el('div', 'gg-brand', {class:'brand'}, el('span', 'gg-logo', {class:'logo', 'aria-hidden':true}, '[gg]'), el('div', 'gg-brand-text', {}, el('h1', 'gg-heading', {}, 'Grand Git'), el('div', 'gg-subtitle', {class:'eyebrow'}, t('Terminal interface / interactive design', 'Терминальный интерфейс / интерактивный дизайн')))),
      el('div', 'gg-edition', {class:'edition'}, t('DESIGN STUDY 01 · LOCAL PREVIEW', 'ДИЗАЙН 01 · ЛОКАЛЬНЫЙ ПРОСМОТР')));
    const controls = el('nav', 'gg-demo-controls', {class:'controls', 'aria-label':t('Prototype controls', 'Управление прототипом')},
      el('div', 'gg-scenario-control', {class:'control'}, el('label', 'gg-scenario-label', {for:'scenario', class:'control-label'}, t('SCENARIO', 'СЦЕНАРИЙ')), select('gg-scenario', state.scenario, G.scenarios.map(r => [r[0], t(r[1], r[2])]), value => {G.reset(value); lastModal = '';}, t('Scenario', 'Сценарий'))),
      el('div', 'gg-size-control', {class:'control'}, el('label', 'gg-size-label', {class:'control-label', for:'size'}, t('SIZE', 'РАЗМЕР')), select('gg-size', state.size, [['auto', t('Fit window', 'По размеру окна')], ['160x42','160 × 42'], ['100x16','100 × 16'], ['99x24','99 × 24'], ['120x15','120 × 15']], value => {state.size = value;}, t('Terminal size', 'Размер терминала'))),
      el('span', 'gg-toolbar-spacer', {class:'spacer'}),
      button('gg-next-panel', t('Next panel ↦', 'Следующая панель ↦'), G.cycle, {title:t('Emulate Ctrl+W without closing the browser tab', 'Эмуляция Ctrl+W без закрытия вкладки'), disabled:!!state.modal || state.detached}),
      button('gg-color', state.color ? t('Color: on', 'Цвет: вкл') : t('Color: off', 'Цвет: выкл'), () => {state.color = !state.color;}, {'aria-pressed':String(!state.color)}),
      button('gg-reset', t('Reset', 'Сбросить'), () => {G.reset(state.scenario); lastModal = '';}));
    const stage = el('div', 'gg-stage', {class:'stage'}, terminal());
    const guide = el('details', 'gg-guide', {class:'guide', open:guideOpen, ontoggle:e => {guideOpen = e.target.open;}},
      el('summary', 'gg-guide-summary', {}, t('Scenario guide & action result', 'Сценарий и результат действия')),
      el('div', 'gg-guide-content', {class:'guide-content'},
        el('section', 'gg-guide-scenario', {}, el('h2', 'gg-guide-scenario-heading', {}, t(scenario[1], scenario[2])), el('p', 'gg-guide-steps', {}, t(scenario[3], scenario[4])),
          ['vanished','shell-exit','notices','agent'].includes(state.scenario) ? button('gg-simulate', t('Simulate event', 'Смоделировать событие'), G.simulate, {style:'margin-top:12px'}) : null),
        el('section', 'gg-guide-result', {}, el('h2', 'gg-guide-result-heading', {}, t('Last action', 'Последнее действие')), el('p', 'gg-last-action', {'aria-live':'polite'}, G.text(state.last)), el('p', 'gg-guide-hint', {class:'hint'}, t('Click / Enter opens · Hover previews · Right-click / ? actions · Esc cancels', 'Клик / Enter открывает · Наведение показывает детали · Правая кнопка / ? действия · Esc отменяет')))));
    const footer = el('footer', 'gg-page-footer', {class:'page-footer'}, el('span', 'gg-demo-disclosure', {}, t('Browser-local simulation. Terminal output is illustrative; no shell commands run.', 'Модель в памяти браузера. Вывод терминалов демонстрационный; команды не исполняются.')), el('span', 'gg-cell-count', {}, `${cols} × ${lines} · ${t('character grid', 'сетка символов')}`));
    return el('main', 'gg-page', {class:'page'}, title, controls, stage, guide, footer, el('span', 'gg-measure', {class:'measure', 'aria-hidden':true}, '0000000000'));
  }
  function terminal() {
    const state = S();
    const content = [];
    if (state.detached) {
      content.push(el('div', 'gg-detached', {class:'terminal-empty'}, el('p', 'gg-detached-title', {}, t('Dashboard detached', 'Dashboard закрыт')), el('p', 'gg-detached-count', {}, `${state.sessions.length} ${t('session records preserved', 'записей сессий сохранено')}`), button('gg-resume', t('[Enter] Launch dashboard', '[Enter] Запустить dashboard'), G.resume)));
    } else {
      if (!compact() || state.focus === 'list') content.push(sidebar());
      if (!compact() || state.focus !== 'list') content.push(panes());
    }
    const healthy = state.sessions.filter(s => !G.group(s.group).error);
    const screen = el('section', 'gg-terminal', {class:`terminal${state.color ? '' : ' no-color'}`, style:`--cols:${cols};--rows:${lines};--sidebar:${Math.max(28, Math.min(56, Math.round(cols * .25)))}`, 'aria-label':t('Grand Git terminal interface', 'Терминальный интерфейс Grand Git')},
      el('div', 'gg-workspace', {class:'workspace'}, content),
      el('div', 'gg-status', {class:'status'}, el('span', 'gg-status-project', {class:'status-left'}, G.group(state.context) ? `Grand Git · ${G.group(state.context).name}` : 'Grand Git'), el('span', 'gg-status-activity', {class:'status-right'}, `${t('Active', 'Активно')} ${healthy.filter(G.active).length}/${healthy.length} · Ctrl+W ${t('Next', 'Далее')}`)));
    if (state.modal && !state.detached) screen.append(dialog());
    return screen;
  }
  function sidebar() {
    const state = S(), selected = G.session(state.selected), selectedGroup = G.selectedGroup(), rows = G.rows();
    const top = el('div', 'gg-list-top', {class:'list-top'});
    if (!state.searching) top.append(shortcut('gg-add-section', '+', t('Add section', 'Добавить раздел'), () => act('add-section')));
    if (state.onlyActive) top.append(el('div', 'gg-active-filter', {}, t('Active', 'Активные')));
    if (G.searchAvailable()) {
      if (state.searching || state.query) {
        top.append(el('input', 'gg-search-input', {class:'search-input', value:state.query, placeholder:t('/ Search sessions', '/ Поиск сессий'), 'aria-label':t('Search sessions', 'Поиск сессий'),
          onfocus:() => {if (!state.searching) {state.searching = true; queueMicrotask(render);}},
          oninput:e => {state.query = e.target.value; state.searching = true; G.normalize(); render();}}));
      } else top.append(shortcut('gg-search', '/', t('Search', 'Поиск'), () => act('search')));
    }
    const list = el('div', 'gg-session-list', {class:'sessions', tabindex:'0', role:'tree', 'aria-label':t('Folders and sessions', 'Папки и сессии'), 'aria-activedescendant':state.selected ? `row-${state.selected}` : null,
      onmouseleave:() => previewDetails(state.selected), onwheel:e => {e.preventDefault(); move(Math.sign(e.deltaY), false); render();}});
    for (const g of G.visibleGroups()) {
      const entries = rows.filter(s => s.group === g.id), prefix = `gg-sidebar-group-${g.id}`, selectedFolder = state.selected === G.groupKey(g);
      const groupTitle = el('button', `${prefix}-heading`, {id:`row-${G.groupKey(g)}`, type:'button', tabindex:'-1', role:'treeitem', 'aria-level':'1', 'aria-expanded':'true', 'aria-selected':String(selectedFolder), class:`group-heading${selectedFolder ? ' selected' : ''}`, title:g.path,
        onclick:() => {state.selected = G.groupKey(g); state.focus = 'list'; state.searching = false; render();}, onmouseenter:() => previewDetails(G.groupKey(g)), oncontextmenu:e => contextMenu(e, G.groupKey(g))},
        el('span', `${prefix}-name`, {class:'group-title'}, g.name), state.groups.some(other => other.id !== g.id && other.name === g.name) ? el('span', `${prefix}-path`, {class:'group-path'}, g.path) : null);
      const box = el('div', prefix, {class:'session-group', role:'group', 'aria-label':`${g.name} ${g.path}`}, groupTitle);
      if (g.error) box.append(button(`${prefix}-error`, t('! Server unavailable', '! Сервер недоступен'), () => G.open(`${g.id}-2`), {class:'notice'}));
      for (const s of entries) {
          const p = `${prefix}-session-${s.id}`, selectedRow = state.selected === G.key(s);
          box.append(el('button', `${p}-row`, {id:`row-${G.key(s)}`, type:'button', role:'treeitem', 'aria-level':'2', tabindex:'-1', 'aria-selected':String(selectedRow), class:`session-row${selectedRow ? ' selected' : ''}`, title:s.path,
            onclick:() => {state.selected = G.key(s); state.searching = false; G.open(G.key(s)); render();}, onmouseenter:() => previewDetails(G.key(s)), oncontextmenu:e => contextMenu(e, G.key(s))},
            el('span', `${p}-open`, {class:'open-mark', 'aria-label':state.open === G.key(s) ? t('Open', 'Открыта') : t('Not open', 'Не открыта')}, state.open === G.key(s) ? '▸' : selectedRow ? '›' : ' '),
            el('span', `${p}-activity`, {class:`activity${G.active(s) ? ' running' : ''}`, 'aria-label':G.active(s) ? t('Running', 'Работает') : t('Stopped', 'Остановлена')}, G.active(s) ? '●' : '○'),
            el('span', `${p}-kind`, {class:'session-kind', 'aria-label':G.isWorktree(s) ? t('Worktree','Worktree-сессия') : t('Terminal','Терминал')}, G.isWorktree(s) ? '⎇' : '>_'),
            el('span', `${p}-name`, {class:'session-name'}, s.name)));
      }
      list.append(box);
    }
    if (!rows.length) list.append(el('div', 'gg-no-matches', {class:'empty'}, t('No matching sessions', 'Нет подходящих сессий')));
    const children = [top, list];
    if (lines >= 24) children.push(details(state.selected));
    const foot = el('div', 'gg-sidebar-footer', {class:'sidebar-footer'});
    if (state.searching) {
      foot.append(el('div', 'gg-footer-search', {class:'footer-line'},
        button('gg-search-clear', t('Clear', 'Очистить'), () => {state.query = ''; G.normalize();}),
        shortcut('gg-search-down', '↓', t('To list', 'К списку'), G.leaveSearch)));
    } else if (selectedGroup && !selected) {
      foot.append(shortcut('gg-new', 'N', selectedGroup.type === 'git' ? t('New worktree', 'Новая worktree') : t('New session','Новая сессия'), () => act('new')));
      foot.append(el('div', 'gg-footer-group', {class:'footer-line'}, shortcut('gg-group-terminal', 'T', t('Terminal', 'Терминал'), () => act('terminal'))));
      foot.append(el('div', 'gg-footer-group-lifecycle', {class:'footer-line'},
        state.sessions.some(s => s.group === selectedGroup.id && G.active(s)) ? shortcut('gg-group-stop', 'S', t('Stop all', 'Остановить все'), () => act('stop')) : null,
        state.sessions.some(s => s.group === selectedGroup.id) ? shortcut('gg-delete', 'D', t('Delete all', 'Удалить все'), () => act('delete')) : null));
    } else if (selected) {
      foot.append(el('div', 'gg-footer-primary', {class:'footer-line'}, shortcut('gg-open', '↵', t('Open', 'Открыть'), () => act('open'), {disabled:!selected}), shortcut('gg-new', 'N', t('New', 'Новая'), () => act('new'))));
      if (selected && !selected.protected) foot.append(el('div', 'gg-footer-delete', {class:'footer-line'}, shortcut('gg-delete', 'D', t('Delete', 'Удалить'), () => act('delete'))));
    }
    if (!state.searching) foot.append(el('div', 'gg-footer-secondary', {class:'footer-line'}, shortcut('gg-actions', '?', t('Actions', 'Действия'), () => act('actions')), shortcut('gg-quit', 'Q', t('Quit', 'Выход'), () => act('quit'))));
    children.push(foot);
    if (state.notice) children.push(el('div', 'gg-notice', {class:`notice ${state.notice.kind}`, role:state.notice.kind === 'error' ? 'alert' : 'status', title:G.text(state.notice.message)}, G.text(state.notice.message)));
    return el('aside', 'gg-sidebar', {class:`sidebar${state.focus === 'list' ? ' focused' : ''}${compact() ? ' compact' : ''}`, 'aria-label':t('Session list and details', 'Список и детали сессий')}, children);
  }
  function details(id) {
    const s = G.session(id), g = G.selectedGroup(id);
    if (!g) return el('section', 'gg-details', {class:'details'}, t('No selection', 'Ничего не выбрано'));
    let pairs;
    if (s) {
      const statuses = {ready:t('ready','готов'), working:t('working','работает'), waiting:t('waiting for permission','ждёт разрешения'), done:t('turn completed','ход завершён'), interrupted:t('interrupted','прерван'), shell:t('shell','оболочка'), stopped:t('stopped','остановлен'), unknown:t('unknown','неизвестно')};
      const git = {clean:t('clean','чисто'), modified:t('modified','изменено'), unknown:t('unknown','неизвестно'), 'n/a':'—'};
      pairs = [['terminals',t('Terminals','Терминалы'),s.panes.length],['agent',t('Agent','Агент'),statuses[G.active(s) ? s.agent : 'stopped']],['git','Git',git[s.git]],['open',t('Open','Открыта'),S().open === G.key(s) ? t('yes','да') : t('no','нет')]];
    } else {
      const sessions = S().sessions.filter(s => s.group === g.id);
      pairs = [['sessions',t('Sessions','Сессии'),sessions.length],['active',t('Running','Работают'),sessions.filter(G.active).length],['type',t('Folder','Папка'),g.type === 'git' ? 'Git' : t('directory','директория')]];
    }
    const path = s?.path || g.path;
    return el('section', 'gg-details', {class:'details', 'aria-label':t('Session or folder details', 'Детали сессии или папки')},
      el('div', 'gg-details-path', {class:'detail-path', title:path}, path.length > Math.max(28,Math.min(56,Math.round(cols*.25)))-2 && !compact() ? '…'+path.slice(-(Math.max(28,Math.min(56,Math.round(cols*.25)))-4)) : path),
      pairs.map(([id,label,value]) => el('div', `gg-details-${id}`, {class:'detail-line'}, el('span', `gg-details-${id}-label`, {class:'detail-label'}, `${label}:`), el('span', `gg-details-${id}-value`, {class:'detail-value'}, value))),
      s?.protected || s?.kind === 'temporary' ? el('div', 'gg-details-lifetime', {class:'detail-note'}, s.protected ? t('Protected source','Защищённый источник') : t('Temporary · ends with dashboard','Временная · до выхода')) : null);
  }
  function panes() {
    const s = G.session(S().open);
    const area = el('div', 'gg-panes', {class:'panes'});
    if (!s || !G.active(s)) {area.append(el('div', 'gg-terminal-empty', {class:'terminal-empty'}, el('p', 'gg-terminal-empty-title', {}, t('No terminal open', 'Нет открытого терминала')), el('p', 'gg-terminal-empty-hint', {}, t('Select a session and press Enter', 'Выберите сессию и нажмите Enter')))); return area;}
    function layout(node) {
      if (!node.pane) return el('div', `gg-layout-${G.key(s)}-${node.id}`, {class:`pane-split ${node.direction}`}, layout(node.first), layout(node.second));
      const p = s.panes.find(p => p.id === node.pane), prefix = `gg-pane-${p.id}`;
      return el('section', prefix, {class:`pane${S().focus === 'terminal' && S().activePane === p.id ? ' focused' : ''}`, 'aria-label':p.title, onclick:e => {if (e.target.tagName !== 'INPUT') {S().activePane = p.id; S().focus = 'terminal'; S().searching = false; render();}}},
        el('div', `${prefix}-title`, {class:'pane-title'}, `${S().focus === 'terminal' && S().activePane === p.id ? '▸' : '─'} ${p.title}`),
        el('div', `${prefix}-content`, {class:'pane-content'}, p.lines.map((line,i) => el('div', `${prefix}-line-${i}`, {class:`terminal-line${line.startsWith('›') || line.startsWith('$') ? ' command' : line.includes('Working') || line.startsWith('  +') ? ' positive' : line.startsWith('  Read') ? ' dim' : ''}`}, line || ' '))),
        el('div', `${prefix}-prompt`, {class:'prompt'}, el('span', `${prefix}-prompt-label`, {class:'prompt-label'}, '›'), el('input', `${prefix}-input`, {class:'prompt-input', value:p.input, 'aria-label':t('Demonstration terminal input', 'Демонстрационный ввод терминала'), autocomplete:'off', spellcheck:'false', onfocus:() => {const changed = S().activePane !== p.id || S().focus !== 'terminal'; S().activePane = p.id; S().focus = 'terminal'; S().searching = false; if (changed) queueMicrotask(render);}, oninput:e => {p.input = e.target.value;}, onkeydown:e => {if (e.key === 'Enter') {e.preventDefault(); p.lines.push(`› ${p.input}`); p.input = ''; S().last = [t('Input recorded in the demonstration transcript; no command executed.', 'Ввод добавлен в демонстрационный вывод; команда не исполнена.'), 'Ввод добавлен в демонстрационный вывод; команда не исполнена.']; render();}}})));
    }
    area.append(layout(s.layout)); return area;
  }
  function dialog() {
    const m = S().modal, type = m.type;
    const titles = {section:t('Add section','Добавить раздел'), create:t('Create session','Создать сессию'), confirm:m.bulk ? (m.action === 'delete' ? t('Delete all sessions','Удалить все сессии') : t('Stop all sessions','Остановить все сессии')) : (m.action === 'delete' ? t('Delete session','Удалить сессию') : t('Stop session','Остановить сессию')), split:t('Add terminal','Добавить терминал'), actions:t('Actions','Действия'), settings:t('Settings','Настройки')};
    const d = el('section', 'gg-dialog', {class:`dialog${m.anchor ? ' context-menu' : ''}`, role:'dialog', 'aria-modal':'true', 'aria-labelledby':'dialog-title'}, el('h2', 'gg-dialog-title', {id:'dialog-title'}, titles[type]));
    const p = (id, text, cls = '') => el('p', `gg-dialog-${id}`, {class:cls}, text);
    const cancel = () => {S().modal = null;};
    if (type === 'actions') for (const [index,[name,key,label]] of actions(m.target, !!m.anchor).entries()) d.append(shortcut(`gg-menu-${name}`, key, `${m.choice === index ? '› ' : '  '}${label}`, () => {S().modal = null; G.action(name, m.target);}, {class:`menu-item${m.choice === index ? ' current' : ''}`}));
    if (type === 'section') {
      d.append(el('label','gg-section-path-label',{for:'section-path'},t('Directory','Директория')),
        el('input','gg-section-path',{id:'section-path',value:m.path,autocomplete:'off',spellcheck:'false',oninput:e => {m.path=e.target.value;}}));
      if (m.error) d.append(p('error',G.text(m.error),'error'));
      d.append(el('div','gg-section-folders',{class:'section-folders',role:'list',tabindex:'0'},m.entries.map((entry,i) => button(`gg-section-folder-${i}`,entry.name+'/',() => G.browseSection(entry.path),{class:'menu-item', 'data-path':entry.path}))));
      d.append(el('div','gg-section-actions',{class:'dialog-actions'},shortcut('gg-section-submit','Ctrl+S',t('Add','Добавить'),G.submitSection),shortcut('gg-section-cancel','Esc',t('Cancel','Отмена'),cancel)));
    }
    if (type === 'create') {
      if (m.picking) {
        d.append(p('template-heading', t('Choose template','Выберите шаблон')));
        for (const [i,name] of G.templates.entries()) d.append(button(`gg-template-${name}`, `${m.choice === i ? '› ' : '  '}${name}`, () => {m.template = name; m.picking = false;}, {class:`menu-item${m.choice === i ? ' current' : ''}`}));
        d.append(p('template-source', 'built-in', 'muted'), shortcut('gg-template-back','Esc',t('Back','Назад'),() => {m.picking = false;}));
      } else {
        const g = G.group(m.group);
        d.append(p('create-target', `${g.name} · ${m.path}`, 'muted'));
        if (m.kind === 'worktree') d.append(p('create-base', `${t('Base','Основа')}: ${m.base} HEAD`, 'muted'));
        d.append(el('div','gg-create-field',{class:'field'},el('label','gg-create-name-label',{for:'create-name'},m.kind === 'worktree' ? t('Branch name','Имя ветки') : t('Name (optional)','Имя (необязательно)')),el('input','gg-create-name',{id:'create-name',value:m.name,autocomplete:'off',spellcheck:'false',oninput:e => {m.name = e.target.value;}})),
          shortcut('gg-create-template','Tab',`${t('Template','Шаблон')}: ${m.template}`,() => {m.picking = true; m.choice = Math.max(0,G.templates.indexOf(m.template));}));
        if (m.error) d.append(p('error',G.text(m.error),'error'));
        d.append(el('div','gg-create-actions',{class:'dialog-actions'},shortcut('gg-create-submit','Enter',t('Create','Создать'),G.submitCreate),shortcut('gg-create-cancel','Esc',t('Cancel','Отмена'),cancel)));
      }
    }
    if (type === 'confirm') {
      const g = G.group(m.group);
      d.append(p('target',m.name,'danger'),p('target-group',`${g.name} · ${g.path}`,'muted'),p('processes',t('Processes will stop.','Процессы будут остановлены.')));
      if (m.bulk) d.append(p('count', `${t('Sessions','Сессий')}: ${m.targets.length}`));
      if (m.action === 'delete' && m.bulk) d.append(p('files', t('Managed worktrees will be removed. Protected folders, ordinary directory files and Git branches will remain.', 'Worktree-сессии будут удалены. Защищённые папки, файлы обычных директорий и Git-ветки сохранятся.')));
      if (m.action === 'delete' && !m.bulk) d.append(p('files',m.kind === 'worktree' ? t('Worktree will be removed. Git branch will remain.','Рабочая копия будет удалена. Git-ветка сохранится.') : t('The session record will be removed. Directory files will remain.','Запись сессии будет удалена. Файлы директории сохранятся.')));
      d.append(el('div','gg-confirm-actions',{class:'dialog-actions'},shortcut('gg-confirm-yes','Y',m.action === 'delete' ? t('Delete','Удалить') : t('Stop','Остановить'),G.applyConfirmation,{class:'danger'}),shortcut('gg-confirm-no','N / Esc',t('Cancel','Отмена'),cancel)));
    }
    if (type === 'split') {
      d.append(p('split-hint',t('Place a plain shell relative to the last terminal.','Разместите обычный терминал относительно последнего.')),
        shortcut('gg-split-below','B',`${m.choice === 0 ? '› ' : ''}${t('Below','Внизу')}`,() => G.split('below'),{class:'menu-item'}),shortcut('gg-split-right','R',`${m.choice === 1 ? '› ' : ''}${t('Right','Справа')}`,() => G.split('right'),{class:'menu-item'}),
        el('div','gg-split-actions',{class:'dialog-actions'},shortcut('gg-split-cancel','Esc',t('Cancel','Отмена'),cancel)));
    }
    if (type === 'settings') {
      const g = G.group(m.group);
      d.append(p('settings-context',`${t('Open project','Открытый проект')}: ${g?.name || t('Global','Глобально')}`, 'muted'));
      for (const [name,label,value,entries,change] of [
        ['language',t('Language','Язык'),m.draft.language,[['en','English'],['ru','Русский']],v => {m.draft.language = v;}],
        ['template',t('Default template','Шаблон по умолчанию'),m.draft.template,[...new Set([...G.templates,m.draft.template])].map(v => [v,v]),v => {m.draft.template = v;}],
        ['scope',t('Save to','Сохранить в'),m.scope,g?.type === 'git' ? [['global',t('Global','Глобально')],['local',t('Local','Локально')]] : [['global',t('Global','Глобально')]],v => {m.scope = v;}]
      ]) d.append(el('div',`gg-setting-${name}-field`,{class:'field'},el('label',`gg-setting-${name}-label`,{for:`setting-${name}`},label),select(`gg-setting-${name}`,value,entries,change,label)));
      d.append(el('div','gg-settings-actions',{class:'dialog-actions'},shortcut('gg-settings-save','Enter',t('Save','Сохранить'),G.saveSettings),shortcut('gg-settings-cancel','Esc',t('Cancel','Отмена'),cancel)));
    }
    if (type === 'actions') d.append(el('div','gg-menu-footer',{class:'dialog-actions'},shortcut('gg-menu-cancel','Esc',t('Back','Назад'),cancel)));
    return el('div','gg-dialog-wrap',{class:`modal-wrap${m.anchor ? ' context-wrap' : ''}`, onclick:e => {if (m.anchor && e.target === e.currentTarget) {cancel(); render();}}, oncontextmenu:e => {if (m.anchor) {e.preventDefault(); cancel(); render();}}},d);
  }
  function move(delta, allowSearch = true) {
    if (S().modal) return;
    const items = G.items(), i = items.indexOf(S().selected);
    if (allowSearch && delta < 0 && i <= 0 && G.searchAvailable()) {act('search'); return;}
    if (items.length) S().selected = items[Math.max(0,Math.min(items.length-1,i+delta))];
    S().focus = 'list'; S().searching = false;
  }
  function measure() {
    const probe = root.querySelector('[data-testid="gg-measure"]');
    const stage = root.querySelector('[data-testid="gg-stage"]');
    if (!probe || !stage) return;
    const cell = probe.getBoundingClientRect().width / 10;
    document.documentElement.style.setProperty('--cell',`${cell}px`);
    if (S().size === 'auto') {cols = Math.max(28,Math.floor((stage.clientWidth-2)/cell)); lines = Math.max(16,Math.min(42,Math.floor((window.innerHeight-295)/20)));}
    else [cols,lines] = S().size.split('x').map(Number);
  }
  function render() {
    const focused = document.activeElement?.dataset.testid;
    const range = document.activeElement?.tagName === 'INPUT' ? [document.activeElement.selectionStart,document.activeElement.selectionEnd] : null;
    const scrolls = new Map([...root.querySelectorAll('.pane-content,.sessions')].map(n => [n.dataset.testid,n.scrollTop]));
    if (S().notice?.until && S().notice.until <= Date.now()) S().notice = null;
    root.replaceChildren(scaffold());
    const old = `${cols}x${lines}`; measure();
    if (old !== `${cols}x${lines}`) root.replaceChildren(scaffold());
    document.documentElement.lang = G.effective().language;
    for (const n of root.querySelectorAll('.pane-content,.sessions')) if (scrolls.has(n.dataset.testid)) n.scrollTop = scrolls.get(n.dataset.testid);
    const modalKey = S().modal ? `${S().modal.type}-${!!S().modal.picking}` : '';
    let target;
    const logicalFocus = `${S().focus}-${S().open}-${S().activePane}-${S().searching}-${S().selected}`;
    const focusNode = () => S().focus === 'terminal' ? root.querySelector(`[data-testid="gg-pane-${S().activePane}-input"]`) : root.querySelector('[data-testid="gg-session-list"]');
    if (!S().modal && S().searching) target = root.querySelector('[data-testid="gg-search-input"]');
    else if (modalKey !== lastModal) {
      if (S().modal?.type === 'create' && !S().modal.picking) target = root.querySelector('[data-testid="gg-create-name"]');
      else if (S().modal) target = root.querySelector('.dialog button, .dialog input, .dialog select');
      else target = focusNode();
    } else if (logicalFocus !== previousFocus && !S().modal) target = focusNode();
    else if (focused) target = [...root.querySelectorAll('[data-testid]')].find(n => n.dataset.testid === focused);
    if (!target && S().focus === 'terminal') target = root.querySelector(`[data-testid="gg-pane-${S().activePane}-input"]`);
    if (target) {target.focus({preventScroll:true}); if (range && target.tagName === 'INPUT') target.setSelectionRange(...range);}
    lastModal = modalKey;
    previousFocus = logicalFocus;
    const row = root.querySelector('.session-row.selected,.group-heading.selected'), list = root.querySelector('.sessions');
    if (row && list) {
      const rowRect = row.getBoundingClientRect(), listRect = list.getBoundingClientRect();
      if (rowRect.bottom > listRect.bottom) list.scrollTop += rowRect.bottom-listRect.bottom;
      const heading = row.parentElement.querySelector('.group-heading');
      const inset = row === heading ? 0 : heading.offsetHeight;
      if (row.getBoundingClientRect().top < listRect.top + inset) list.scrollTop -= listRect.top + inset-row.getBoundingClientRect().top;
    }
    if (S().modal?.anchor) {
      const menu = root.querySelector('.dialog'), screen = root.querySelector('[data-testid="gg-terminal"]'), anchor = S().modal.anchor;
      menu.style.left = `${Math.max(0,Math.min(anchor.x,screen.clientWidth-menu.offsetWidth))}px`;
      menu.style.top = `${Math.max(0,Math.min(anchor.y,screen.clientHeight-20-menu.offsetHeight))}px`;
    }
    if (S().modal?.type === 'actions') {
      const menu = root.querySelector('.dialog'), item = menu.querySelectorAll('.menu-item')[S().modal.choice];
      if (item) {const a = item.getBoundingClientRect(), b = menu.getBoundingClientRect();
        if (a.bottom > b.bottom-10) menu.scrollTop += a.bottom-b.bottom+10;
        else if (a.top < b.top+10) menu.scrollTop -= b.top+10-a.top;
      }
    }
    clearTimeout(timer);
    if (S().notice?.until) timer = setTimeout(render, Math.max(0,S().notice.until-Date.now()));
  }
  document.addEventListener('keydown', e => {
    const state = S(), k = e.key.toLowerCase();
    if (e.ctrlKey && k === 's' && state.modal?.type === 'section') {e.preventDefault(); G.submitSection(); render(); return;}
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (!e.target.closest('#app')) return;
    const withinDemo = e.target.closest('.controls,.guide,.masthead');
    if (withinDemo) return;
    if (state.detached) {if (k === 'enter') {e.preventDefault(); G.resume(); render();} return;}
    const m = state.modal;
    if (m) {
      if (k === 'escape') {e.preventDefault(); if (m.type === 'create' && m.picking) m.picking = false; else state.modal = null; render(); return;}
      if (m.type === 'section') {
        if (k === 'enter' && e.target.dataset.testid === 'gg-section-path') {e.preventDefault(); G.browseSection(m.path); render(); root.querySelector('[data-testid="gg-section-folder-0"]')?.focus(); return;}
        if (e.target.closest('.section-folders') && ['arrowup','arrowdown','backspace','arrowleft'].includes(k)) {
          e.preventDefault();
          if (['backspace','arrowleft'].includes(k)) {G.browseSection(m.entries[0].path); render();}
          else {const nodes = [...root.querySelectorAll('.section-folders button')], i = nodes.indexOf(e.target); nodes[Math.max(0,Math.min(nodes.length-1,i+(k==='arrowup' ? -1 : 1)))]?.focus();}
          return;
        }
      }
      if (m.type === 'create') {
        if (m.picking) {
          if (['arrowup','k','arrowdown','j'].includes(k)) {e.preventDefault(); m.choice = Math.max(0,Math.min(G.templates.length-1,m.choice+(['arrowup','k'].includes(k) ? -1 : 1)));}
          if (k === 'enter') {e.preventDefault(); m.template = G.templates[m.choice]; m.picking = false;}
          if (['arrowup','k','arrowdown','j','enter'].includes(k)) {render(); return;}
        }
        if (k === 'tab' && !e.shiftKey && !m.picking && e.target.dataset.testid === 'gg-create-name') {e.preventDefault(); m.picking = true; m.choice = Math.max(0,G.templates.indexOf(m.template)); render(); return;}
        if (!m.picking && k === 'enter' && e.target.tagName === 'INPUT') {e.preventDefault(); G.submitCreate(); render(); return;}
      }
      if (m.type === 'confirm' && ['y','n'].includes(k)) {e.preventDefault(); if (k === 'y') G.applyConfirmation(); else state.modal = null; render(); return;}
      if (m.type === 'split') {
        if (['arrowup','arrowleft','arrowdown','arrowright'].includes(k)) {e.preventDefault(); m.choice = ['arrowup','arrowleft'].includes(k) ? 0 : 1; render(); root.querySelector(`[data-testid="gg-split-${m.choice ? 'right' : 'below'}"]`).focus(); return;}
        if (['b','r','enter'].includes(k) && (k !== 'enter' || !e.target.closest('button'))) {e.preventDefault(); G.split(k === 'r' || (k === 'enter' && m.choice === 1) ? 'right' : 'below'); render(); return;}
      }
      if (m.type === 'actions') {
        const items = actions(m.target, !!m.anchor);
        if (['arrowup','k','arrowdown','j'].includes(k)) {e.preventDefault(); m.choice = Math.max(0,Math.min(items.length-1,m.choice+(['arrowup','k'].includes(k) ? -1 : 1))); render(); root.querySelectorAll('.menu-item')[m.choice]?.focus(); return;}
        if (k === 'enter' && e.target.dataset.testid !== 'gg-menu-cancel') {e.preventDefault(); const item = items.find(row => `gg-menu-${row[0]}` === e.target.dataset.testid) || items[m.choice]; state.modal = null; G.action(item[0], m.target); render(); return;}
        const item = items.find(row => row[1].toLowerCase() === k);
        if (item) {e.preventDefault(); state.modal = null; G.action(item[0], m.target); render(); return;}
      }
      if (m.type === 'settings' && k === 'enter' && e.target.tagName === 'SELECT') {e.preventDefault(); G.saveSettings(); render(); return;}
      // Keep native Tab navigation inside the modal, including short scrollable dialogs.
      if (k === 'tab') {
        const nodes = [...root.querySelectorAll('.dialog button,.dialog input,.dialog select')];
        const i = nodes.indexOf(document.activeElement);
        if (i === nodes.length-1 && !e.shiftKey || i === 0 && e.shiftKey) {e.preventDefault(); nodes[e.shiftKey ? nodes.length-1 : 0]?.focus();}
      }
      return;
    }
    if (state.focus === 'terminal' || e.target.classList.contains('prompt-input')) return;
    if (state.searching) {
      if (k === 'escape') {e.preventDefault(); G.cancelSearch(); render();}
      else if (['enter','arrowdown'].includes(k)) {e.preventDefault(); G.leaveSearch(); render();}
      return;
    }
    if (k === 'escape') {
      e.preventDefault(); if (state.query) G.cancelSearch(); else if (compact()) G.cycle(); render(); return;
    }
    if (k === 'contextmenu' || (e.shiftKey && k === 'f10')) {e.preventDefault(); act('actions'); render(); return;}
    if (['arrowup','k','arrowdown','j'].includes(k)) {e.preventDefault(); move(['arrowup','k'].includes(k) ? -1 : 1); render(); return;}
    const commands = {'+':'add-section','x':'remove-section','n':'new','t':'split','s':'stop','d':'delete','a':'filter','/':'search','?':'actions',',':'settings','q':'quit','enter':'open'};
    if (commands[k]) {
      if (k === 'enter' && e.target.tagName === 'BUTTON' && !e.target.matches('.session-row,.group-heading')) return;
      e.preventDefault(); act(commands[k]); render();
    }
  });
  window.addEventListener('resize',render);
  render();
  root.querySelector('[data-testid="gg-session-list"]')?.focus({preventScroll:true});
})();

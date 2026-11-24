package gg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#73DACA")).Bold(true)
var dim = lipgloss.NewStyle().Foreground(lipgloss.Color("#818B9D"))
var danger = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
var green = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))

type refreshMsg struct {
	project *Project
	hidden  bool
	groups  []sessionGroup
	state   *State
	snap    Snapshot
	cfg     Settings
	git     gitCache
	err     error
	started time.Time
}
type tickMsg struct{ periodic bool }
type actionMsg struct{ err error }
type popupAction struct {
	Section   int
	Workspace int
	Client    string
	Kind      string
	ID        int
	Direction string
	Folder    bool
	IDs       []int
	Bulk      bool
}
type hit struct {
	x1, x2, y int
	key       string
	id        int
	group     int
	row       bool
	folder    bool
}
type sidebar struct {
	groups                                []sessionGroup
	cursorGroup, confirmGroup, modalGroup int
	eventClient, actionClient             string
	p                                     *Project
	task, selected, cursorID              int
	pane                                  string
	client                                string
	width, height                         int
	state                                 *State
	snap                                  Snapshot
	cfg                                   Settings
	git                                   gitCache
	active, confirming                    bool
	refreshing                            bool
	confirmID                             int
	confirmAction                         string
	message                               string
	lastKey                               time.Time
	query                                 string
	searching, onlyActive                 bool
	search                                textinput.Model
	modal                                 string
	actionIndex                           int
	splitChoice, modalID, settingField    int
	draft                                 Settings
	projectSettings                       bool
	settingTemplates                      []Template
	popup                                 bool
	resultPath                            string

	direct                           bool
	cursorFolder                     bool
	hover, menuTarget                *listTarget
	menuAnchor                       *screenPoint
	confirmIDs                       []int
	confirmBulk                      bool
	confirmName                      string
	confirmWorkspace, confirmSection int
	modalWorkspace, modalSection     int
	screenWidth, screenHeight        int
	pendingAction                    *popupAction
	spinnerFrame                     int
}

func sidebarTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{periodic: true} })
}
func requestSidebarRefresh() tea.Msg { return tickMsg{} }

func (m sidebar) collectRefresh() tea.Cmd {
	return func() tea.Msg {
		started := time.Now()
		withGit := m.groups != nil
		if !m.direct && !m.popup && m.pane != "" {
			viewers, err := m.p.tmux("display-message", "-p", "-t", m.pane, "#{window_active_clients}")
			if err == nil && viewers == "0" {
				// Compact clients still display the server's activity counter
				// while every sidebar is parked or in a background window.
				if m.task >= 0 {
					return refreshMsg{hidden: true}
				}
				clients, err := m.p.tmux("list-clients", "-F", "#{client_pid}")
				if err == nil && clients == "" {
					return refreshMsg{hidden: true}
				}
				withGit = false
			}
		}
		groups, err := m.collectGroups(withGit)
		cfg, cfgErr := m.p.settings()
		if err == nil {
			err = cfgErr
		}
		var state *State
		var git gitCache
		snap := Snapshot{}
		if m.p.WorkspaceID > 0 {
			snap = m.p.snapshot()
		}
		for _, g := range groups {
			if g.workspaceID() == m.p.WorkspaceID {
				state, git = g.State, g.Git
			}
		}
		if err == nil && m.p.WorkspaceID > 0 {
			_ = m.p.syncSettings()
			m.p.updateGroupCounter(groups, cfg)
		}
		return refreshMsg{project: m.p, groups: groups, state: state, snap: snap, cfg: cfg, git: git, err: err, started: started}
	}
}
func (m sidebar) Init() tea.Cmd      { return func() tea.Msg { return tickMsg{periodic: true} } }
func effect(fn func() error) tea.Cmd { return func() tea.Msg { return actionMsg{err: fn()} } }

func resetViewport() tea.Cmd {
	// Tmux retains an explicitly set scroll region when a service pane grows.
	return tea.Sequence(tea.Raw(ansi.SetTopBottomMargins(0, 0)), tea.ClearScreen)
}

func (m sidebar) rows() []*Task {
	candidates := m.candidates()
	if m.query == "" {
		return candidates
	}
	query := strings.ToLower(m.query)
	rows := []*Task{}
	for _, t := range candidates {
		branch := m.gitSnapshot(t).Branches[strconv.Itoa(t.ID)]
		text := t.label() + " " + m.displayName(t) + " " + branch
		if g := m.group(t.Group); g != nil {
			text += " " + g.View.Name + " " + g.View.Path
		}
		if strings.Contains(strings.ToLower(text), query) {
			rows = append(rows, t)
		}
	}
	return rows
}

// Count before text filtering and viewport clipping, but after activity filtering.
func (m sidebar) searchAvailable() bool {
	count := len(m.candidates())
	if !m.onlyActive {
		for _, g := range m.groups {
			if g.Err == nil {
				count += len(g.Jobs)
			}
		}
	}
	return count > 10
}

func (m *sidebar) clearSearch() {
	m.query, m.searching = "", false
	m.search.SetValue("")
	m.search.Blur()
}

func (m sidebar) candidates() []*Task {
	out := []*Task{}
	add := func(tasks []*Task) {
		for _, t := range tasks {
			if m.onlyActive && len(m.taskSnapshot(t).task(t.ID).live()) == 0 {
				continue
			}
			out = append(out, t)
		}
	}
	if len(m.groups) > 0 {
		for _, g := range m.groups {
			if g.Err == nil && g.State != nil {
				add(g.State.Tasks)
			}
		}
	} else if m.state != nil {
		add(m.state.Tasks)
	}
	return out
}
func (m sidebar) group(id int) *sessionGroup {
	for i := range m.groups {
		if m.groups[i].View.ID == id {
			return &m.groups[i]
		}
	}
	return nil
}
func (m sidebar) taskSnapshot(t *Task) Snapshot {
	if g := m.group(t.Group); g != nil {
		return g.Snap
	}
	return m.snap
}
func (m sidebar) findTask(group, id int) (*Task, error) {
	if g := m.group(group); g != nil {
		if g.Err != nil {
			return nil, g.Err
		}
		return taskByID(g.State, id)
	}
	if (!m.direct || len(m.groups) == 0) && (group == 0 || group == m.p.WorkspaceID) {
		if m.state != nil {
			return taskByID(m.state, id)
		}
	}
	return nil, errors.New("session no longer exists")
}
func (m sidebar) action(kind string, t *Task) popupAction {
	return popupAction{Kind: kind, Workspace: t.Group, ID: t.ID}
}
func (m sidebar) selectedTask() *Task {
	if target := m.selectedTarget(); target != nil && !target.Folder {
		t, _ := m.findTask(target.Group, target.ID)
		return t
	}
	return nil
}
func (m *sidebar) setState(s *State) {
	m.state = s
	if !m.searchAvailable() && (m.searching || m.query != "") {
		m.clearSearch()
	}
	rows := m.items()
	m.selected = max(0, min(m.selected, len(rows)-1))
	for i, t := range rows {
		if t.ID == m.cursorID && t.Group == m.cursorGroup && t.Folder == m.cursorFolder {
			m.selected = i
			return
		}
	}
	if len(rows) > 0 {
		m.cursorID, m.cursorGroup, m.cursorFolder = rows[m.selected].ID, rows[m.selected].Group, rows[m.selected].Folder
	}
}
func (m *sidebar) move(delta int) {
	rows := m.items()
	if len(rows) == 0 {
		return
	}
	m.selected = max(0, min(len(rows)-1, m.selected+delta))
	m.cursorID, m.cursorGroup, m.cursorFolder = rows[m.selected].ID, rows[m.selected].Group, rows[m.selected].Folder
	m.hover = nil
}
func (m *sidebar) focusOpenTask() {
	if m.task < 0 {
		return
	}
	if _, err := m.findTask(m.openGroup(), m.task); err != nil {
		return
	}
	visible := func() bool {
		for _, t := range m.rows() {
			if t.Group == m.openGroup() && t.ID == m.task {
				return true
			}
		}
		return false
	}
	if !visible() {
		m.clearSearch()
	}
	if !visible() {
		m.onlyActive = false
	}
	if visible() {
		m.cursorID, m.cursorGroup = m.task, m.openGroup()
		m.cursorFolder = false
		m.setState(m.state)
	}
}
func (m sidebar) perform(a popupAction) (tea.Model, tea.Cmd) {
	if m.direct {
		if g := m.group(a.Workspace); a.Section == 0 && g != nil {
			a.Section, a.Workspace = g.Section, g.workspaceID()
		}
		m.pendingAction = &a
		return m, func() tea.Msg { return directActionMsg{a} }
	}
	if a.Client == "" {
		a.Client = m.actionClient
	}
	if a.Client == "" {
		a.Client = m.client
	}
	if m.popup {
		return m, func() tea.Msg {
			b, err := json.Marshal(a)
			if err == nil {
				err = os.WriteFile(m.resultPath, b, 0600)
			}
			if err != nil {
				return actionMsg{err: err}
			}
			return tea.Quit()
		}
	}
	return m, effect(func() error { return m.p.perform(a) })
}
func (p *Project) perform(a popupAction) error {
	if a.Kind == "detach" {
		return p.quitInBackground(a.Client)
	}
	target := p
	if a.Workspace > 0 && a.Workspace != p.WorkspaceID {
		var err error
		target, err = workspaceByID(a.Workspace)
		if err != nil {
			return err
		}
	}
	switch a.Kind {
	case "terminal":
		t, err := target.createTerminal(target.Root, "", false, "", "shell")
		if err != nil {
			return err
		}
		return p.handoff(target, t.ID, a.Client)
	case "open", "start", "split":
		if a.Kind == "split" {
			if err := target.addTerminal(a.ID, a.Direction); err != nil {
				return err
			}
		}
		client := a.Client
		if client == "" {
			var err error
			client, err = p.inputClient("")
			if err != nil {
				return err
			}
		}
		return p.handoff(target, a.ID, client)
	case "create":
		cwd := target.Root
		if a.ID >= 0 {
			state, err := target.load()
			if err != nil {
				return err
			}
			t, err := taskByID(state, a.ID)
			if err != nil {
				return err
			}
			cwd = t.Path
		} else if run, err := readRun(clientPID(a.Client)); !a.Folder && err == nil && len(run.Origins) > 0 {
			origin := run.Origins[len(run.Origins)-1]
			if origin.Path != "" {
				cwd = origin.Path
			}
			if owner, e := workspaceByID(origin.Workspace); e == nil {
				if state, e := owner.load(); e == nil {
					if t, e := taskByID(state, origin.Task); e == nil {
						cwd = t.Path
					}
				}
			}
		}
		return p.createPopup(cwd, a.Client)
	case "stop", "delete":
		ids := a.IDs
		if !a.Bulk {
			ids = []int{a.ID}
		}
		return target.operationInBackground(a.Kind, ids, false, a.Bulk, a.Bulk)
	}
	return nil
}
func (m sidebar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.SetWidth(max(4, m.width-5))
		return m, resetViewport()
	case tickMsg:
		var next tea.Cmd
		if msg.periodic {
			next = sidebarTick()
		}
		if m.refreshing {
			return m, next
		}
		m.refreshing = true
		return m, tea.Batch(m.collectRefresh(), next)
	case refreshMsg:
		m.refreshing = false
		if msg.hidden || (msg.project != nil && msg.project != m.p) || msg.started.Before(m.lastKey) {
			return m, nil
		}
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		wasActive := m.active
		m.snap, m.cfg, m.git, m.groups = msg.snap, msg.cfg, msg.git, msg.groups
		m.setState(msg.state)
		m.active = m.popup
		if m.direct {
			m.active = wasActive
		} else {
			for _, pane := range m.snap {
				if !m.popup && pane.ID == m.pane {
					m.active = pane.Active && pane.Session == "dashboard"
				}
			}
		}
		if m.active && !wasActive {
			m.focusOpenTask()
		}
		return m, nil
	case actionMsg:
		m.message = ""
		if msg.err != nil {
			m.message = msg.err.Error()
		}
		return m, requestSidebarRefresh
	case tea.MouseWheelMsg:
		if m.modal == "actions" {
			step := 1
			if msg.Button == tea.MouseWheelUp {
				step = -1
			}
			m.actionIndex = max(0, min(len(m.shortcuts())-1, m.actionIndex+step))
			return m, nil
		}
		if m.modal != "" || m.confirming || m.searching {
			return m, nil
		}
		if msg.Button == tea.MouseWheelUp {
			m.move(-1)
		} else if msg.Button == tea.MouseWheelDown {
			m.move(1)
		}
		return m, nil
	case tea.MouseClickMsg:
		return m.mouseClick(msg)
	case tea.MouseMotionMsg:
		return m.mouseMotion(msg), nil
	case tea.KeyPressMsg:
		m.lastKey = time.Now()
		if m.p.Root != "" {
			groups, err := m.collectGroups(false)
			if err != nil {
				m.message = err.Error()
				return m, nil
			}
			for i := range groups {
				if old := m.group(groups[i].View.ID); old != nil {
					groups[i].Git = old.Git
				}
			}
			m.groups = groups
			m.state = nil
			for _, g := range groups {
				if g.workspaceID() == m.p.WorkspaceID {
					m.state = g.State
				}
			}
			if m.p.WorkspaceID > 0 {
				m.snap = m.p.snapshot()
			}
			m.setState(m.state)
		}
		key := strings.ToLower(msg.String())
		if m.popup && key == "ctrl+w" {
			return m, tea.Quit
		}
		inputClient := m.eventClient
		m.eventClient = ""
		if inputClient == "" {
			inputClient = m.client
		}
		if boundInputKey(msg.String()) && !m.popup && !m.direct && inputClient == "" {
			var err error
			inputClient, err = m.p.takeInputClient()
			if err != nil {
				m.message = err.Error()
				return m, nil
			}
		}
		m.actionClient = inputClient
		m.active = true
		target := m.selectedTarget()
		if m.modal == "actions" {
			if m.menuTarget != nil {
				target = m.menuTarget
			}
			if target != nil && target.Folder && m.group(target.Group) == nil {
				m.modal, m.menuAnchor = "", nil
				m.message = m.cfg.text("Section no longer exists", "Раздел больше не существует")
				return m, nil
			}
			items := m.shortcuts()
			if len(items) == 0 {
				m.modal, m.menuAnchor = "", nil
				return m, nil
			}
			m.actionIndex = min(m.actionIndex, len(items)-1)
			switch key {
			case "esc", "?":
				m.modal = ""
				m.menuAnchor = nil
				return m, nil
			case "up", "k":
				m.actionIndex = max(0, m.actionIndex-1)
				return m, nil
			case "down", "j":
				m.actionIndex = min(len(items)-1, m.actionIndex+1)
				return m, nil
			case "enter":
				key = strings.ToLower(items[m.actionIndex].key)
			case "n", "t", "s", "d", "r", "/", "a", ",", "q", "+", "x":
				available := false
				for _, item := range items {
					available = available || strings.EqualFold(item.key, key)
				}
				if !available {
					return m, nil
				}
			default:
				return m, nil
			}
			m.modal = ""
		}
		if m.searching {
			switch key {
			case "esc":
				m.clearSearch()
				m.focusOpenTask()
				m.setState(m.state)
				return m, nil
			case "enter", "down":
				m.searching = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.query = m.search.Value()
			m.setState(m.state)
			return m, cmd
		}
		if m.confirming {
			if _, err := m.findTask(m.confirmGroup, m.confirmID); !m.confirmBulk && err != nil {
				m.confirming = false
				m.message = m.cfg.text("Session no longer exists", "Сессия больше не существует")
				return m, nil
			}
			switch key {
			case "esc", "n":
				m.confirming = false
			case "y":
				m.confirming = false
				return m.perform(popupAction{Kind: m.confirmAction, ID: m.confirmID, Workspace: m.confirmWorkspace, Section: m.confirmSection, IDs: append([]int(nil), m.confirmIDs...), Bulk: m.confirmBulk})
			}
			return m, nil
		}
		if m.modal == "split" {
			switch key {
			case "esc":
				m.modal = ""
			case "b":
				m.modal = ""
				return m.perform(popupAction{Kind: "split", ID: m.modalID, Workspace: m.modalWorkspace, Section: m.modalSection, Direction: "below"})
			case "r":
				m.modal = ""
				return m.perform(popupAction{Kind: "split", ID: m.modalID, Workspace: m.modalWorkspace, Section: m.modalSection, Direction: "right"})
			case "left", "up":
				m.splitChoice = 0
			case "right", "down":
				m.splitChoice = 1
			case "enter":
				m.modal = ""
				direction := "below"
				if m.splitChoice == 1 {
					direction = "right"
				}
				return m.perform(popupAction{Kind: "split", ID: m.modalID, Workspace: m.modalWorkspace, Section: m.modalSection, Direction: direction})
			}
			return m, nil
		}
		if m.modal == "settings" {
			switch key {
			case "esc":
				m.modal = ""
				m.message = ""
			case "up":
				m.settingField = max(0, m.settingField-1)
			case "down", "tab":
				m.settingField = (m.settingField + 1) % 3
			case "left", "right", " ", "space":
				switch m.settingField {
				case 0:
					if m.draft.Language == "en" {
						m.draft.Language = "ru"
					} else {
						m.draft.Language = "en"
					}
				case 1:
					for i, t := range m.settingTemplates {
						if t.Name == m.draft.DefaultTemplate {
							step := 1
							if key == "left" {
								step = len(m.settingTemplates) - 1
							}
							m.draft.DefaultTemplate = m.settingTemplates[(i+step)%len(m.settingTemplates)].Name
							break
						}
					}
				case 2:
					if !m.p.Directory {
						m.projectSettings = !m.projectSettings
					}
				}
			case "enter":
				if err := m.p.saveSettings(m.draft, m.projectSettings); err != nil {
					m.message = err.Error()
					return m, nil
				}
				m.modal = ""
				m.cfg, _ = m.p.settings()
			}
			return m, nil
		}
		switch key {
		case "esc":
			if m.query != "" {
				m.clearSearch()
				m.focusOpenTask()
				m.setState(m.state)
				return m, nil
			}
			if m.popup {
				return m, tea.Quit
			}
			return m, nil
		case "down", "j":
			m.move(1)
		case "up", "k":
			if m.selected == 0 && m.searchAvailable() {
				m.searching = true
				m.search.SetValue(m.query)
				return m, m.search.Focus()
			}
			m.move(-1)
		case "+":
			return m.perform(popupAction{Kind: "add-section"})
		case "enter", "n", "t", "s", "d", "r", "x":
			return m.dispatch(key, target)
		case "/":
			if !m.searchAvailable() {
				return m, nil
			}
			m.searching = true
			m.search.SetValue(m.query)
			return m, m.search.Focus()
		case "?", "shift+f10", "contextmenu":
			m.openActions(target, nil)
		case "a":
			m.onlyActive = !m.onlyActive
			m.setState(m.state)
		case ",":
			entries, err := m.p.templates()
			if err != nil {
				m.message = err.Error()
				return m, nil
			}
			m.settingTemplates = entries
			found := false
			for _, t := range entries {
				if t.Name == m.cfg.DefaultTemplate {
					found = true
				}
			}
			if !found {
				m.settingTemplates = append(m.settingTemplates, Template{Name: m.cfg.DefaultTemplate})
			}
			m.modal = "settings"
			m.draft = m.cfg
			m.settingField = 0
			m.projectSettings = false
		case "q":
			client, err := inputClient, error(nil)
			if client == "" && !m.direct {
				client, err = m.p.inputClient(m.pane)
			}
			if err != nil {
				m.message = err.Error()
				return m, nil
			}
			return m.perform(popupAction{Kind: "detach", Client: client})
		}
	}
	return m, nil
}

func (m sidebar) View() tea.View {
	content, _ := m.render()
	v := tea.NewView(dashboardSurface(content, m.width, m.height))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func newSidebar(p *Project, task int) sidebar {
	cfg, _ := p.settings()
	search := textinput.New()
	search.SetWidth(24)
	search.Prompt = "/ "
	return sidebar{p: p, task: task, cursorID: max(0, task), cursorGroup: p.WorkspaceID, pane: os.Getenv("TMUX_PANE"), client: os.Getenv("GG_DASHBOARD_CLIENT"), cfg: cfg, width: 32, height: 30, search: search}
}
func runSidebar(p *Project, task int) error {
	m := newSidebar(p, task)
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		m.width, m.height = width, height
		m.search.SetWidth(max(4, width-5))
	}
	_, err := tea.NewProgram(m).Run()
	return err
}

func (p *Project) listPopup(task int, client ...string) error {
	f, err := os.CreateTemp(p.Dir, ".list-result-*")
	if err != nil {
		return err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	clientID := ""
	if len(client) > 0 {
		clientID = client[0]
	}
	args := []string{"display-popup", "-e", "GG_DASHBOARD_CLIENT=" + clientID, "-E", "-x", "0", "-y", "0", "-w", "90%", "-h", "90%", "-d", p.Root}
	if clientID != "" {
		name, e := p.clientName(clientID)
		if e != nil {
			return e
		}
		args = append(args, "-c", name)
	}
	_, err = p.tmux(append(args, p.invocation("_list-popup", "--root", p.Root, "--task", strconv.Itoa(task), "--result", path))...)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var action popupAction
	if err = json.Unmarshal(b, &action); err != nil {
		return err
	}
	return p.perform(action)
}
func (p *Project) createPopup(cwd string, client ...string) error {
	target, err := DiscoverWorkspace(cwd)
	if err != nil {
		return err
	}
	if err = target.register(); err != nil {
		return err
	}
	f, err := os.CreateTemp(p.Dir, ".create-result-*")
	if err != nil {
		return err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	args := []string{"display-popup", "-E", "-w", "80%", "-h", "70%", "-d", p.Root}
	if len(client) > 0 && client[0] != "" {
		name, e := p.clientName(client[0])
		if e != nil {
			return e
		}
		args = append(args, "-c", name)
	}
	_, err = p.tmux(append(args, target.invocation("_create-form", "--root", target.Root, "--cwd", cwd, "--result", path))...)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var a popupAction
	if err = json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(client) > 0 {
		a.Client = client[0]
	}
	return p.perform(a)
}

var errCancelled = errors.New("cancelled")

type createValues struct {
	name, branch, template string
}

type createForm struct {
	field                 int
	inputs                []textinput.Model
	labels                []string
	cfg                   Settings
	done, cancelled       bool
	message               string
	p                     *Project
	template              string
	templates             []Template
	picking, pickerOnly   bool
	choice, width, height int
	context, base         string
	retryID               int
}

func newCreateForm(settings ...Settings) createForm {
	cfg := defaults()
	if len(settings) > 0 {
		cfg = settings[0]
	}
	input := textinput.New()
	input.CharLimit = 200
	input.SetWidth(60)
	name := cfg.DefaultTemplate
	if name == "" {
		name = "shell"
	}
	branch := textinput.New()
	branch.CharLimit = input.CharLimit
	branch.SetWidth(60)
	return createForm{inputs: []textinput.Model{input, branch}, labels: []string{
		cfg.text("Worktree name", "Имя worktree"), cfg.text("Branch (optional)", "Ветка (необязательно)"),
	}, cfg: cfg, template: name, width: 80, height: 24}
}

func (m *createForm) setProject(p *Project) {
	m.p = p
	if p != nil && p.Directory {
		m.inputs = m.inputs[:1]
		m.labels = []string{m.cfg.text("Name (optional)", "Имя (необязательно)")}
	}
}

func (m createForm) values() createValues {
	v := createValues{name: strings.TrimSpace(m.inputs[0].Value()), template: m.template}
	if len(m.inputs) > 1 {
		v.branch = strings.TrimSpace(m.inputs[1].Value())
	}
	return v
}

func (m *createForm) focusField(field int) tea.Cmd {
	m.inputs[m.field].Blur()
	m.field = field
	return m.inputs[field].Focus()
}
func (m createForm) Init() tea.Cmd { return m.inputs[0].Focus() }
func (m createForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		for i := range m.inputs {
			m.inputs[i].SetWidth(max(4, min(60, size.Width-6)))
		}
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		_, hits := m.renderContent(m.width, m.height)
		for _, h := range hits {
			if !containsHit(h, click.X, click.Y) {
				continue
			}
			if strings.HasPrefix(h.key, "template:") {
				m.choice, _ = strconv.Atoi(strings.TrimPrefix(h.key, "template:"))
				return m.Update(commandKey("enter"))
			}
			return m.Update(commandKey(h.key))
		}
		return m, nil
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok && m.picking {
		step := 1
		if wheel.Button == tea.MouseWheelUp {
			step = -1
		}
		m.choice = max(0, min(len(m.templates)-1, m.choice+step))
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.picking {
			switch key.String() {
			case "ctrl+c":
				m.cancelled = true
				return m, tea.Quit
			case "esc":
				if m.pickerOnly {
					m.cancelled = true
					return m, tea.Quit
				}
				m.picking = false
			case "up", "k":
				m.choice = max(0, m.choice-1)
			case "down", "j", "tab":
				m.choice = min(len(m.templates)-1, m.choice+1)
			case "enter":
				if len(m.templates) > 0 {
					m.template = m.templates[m.choice].Name
				}
				m.picking = false
				if m.pickerOnly {
					m.done = true
					return m, tea.Quit
				}
			}
			return m, nil
		}
		switch key.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.inputs) > 1 && strings.TrimSpace(m.inputs[0].Value()) == "" {
				m.message = m.cfg.text("Worktree name is required", "Имя worktree обязательно")
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		case "up", "down":
			field := 0
			if key.String() == "down" {
				field = len(m.inputs) - 1
			}
			cmd := m.focusField(field)
			return m, cmd
		case "tab":
			if err := m.openPicker(); err != nil {
				m.message = err.Error()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.inputs[m.field], cmd = m.inputs[m.field].Update(msg)
	return m, cmd
}
func (m createForm) View() tea.View {
	content, _ := m.renderContent(m.width, m.height)
	v := tea.NewView(content)
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func (m createForm) renderContent(width, height int) (string, []hit) {
	c, footer := textCanvas{width: width}, textCanvas{width: width}
	focus := 1
	if m.picking {
		c.add(accent.Render(m.cfg.text("Choose template", "Выберите шаблон")))
		for i, template := range m.templates {
			line := "  " + template.Name
			if i == m.choice {
				line = selectedStyle.Width(width).Render(ansi.Truncate("› "+template.Name, width, "…"))
				focus = len(c.lines)
			}
			c.control("template:"+strconv.Itoa(i), line)
		}
		if len(m.templates) > 0 {
			footer.add(dim.Render(m.templates[m.choice].Source))
		}
		footer.buttons(shortcut{"Esc", m.cfg.text("Back", "Назад")})
	} else {
		c.add(accent.Render(m.cfg.text("Create session", "Создать сессию")))
		if m.context != "" {
			c.add(dim.Render(m.context))
		}
		if m.base != "" {
			c.add(dim.Render(m.cfg.text("Base: ", "Основа: ") + m.base + " HEAD"))
		}
		for i, input := range m.inputs {
			key := []string{"up", "down"}[i]
			label := m.labels[i]
			if i != m.field {
				label = []string{"↑ ", "↓ "}[i] + label
			}
			c.control(key, label)
			if i == m.field {
				focus = len(c.lines)
			}
			c.control(key, input.View())
		}
		c.buttons(shortcut{"Tab", m.cfg.text("Template: ", "Шаблон: ") + m.template})
		if m.message != "" {
			for _, line := range strings.Split(ansi.Wrap(localizeError(m.message, m.cfg.Language), width, ""), "\n") {
				c.add(danger.Render(line))
			}
		}
		if m.done {
			footer.add(dim.Render(m.cfg.text("Preparing…", "Подготовка…")))
		} else {
			footer.buttons(shortcut{"Enter", m.cfg.text("Create", "Создать")}, shortcut{"Esc", m.cfg.text("Cancel", "Отмена")})
		}
	}
	return fitDialog(c, footer, max(1, height), focus)
}

func (m *createForm) openPicker() error {
	entries, err := m.p.templates()
	if err != nil {
		return err
	}
	m.templates, m.picking, m.choice = entries, true, 0
	for i, t := range entries {
		if t.Name == m.template {
			m.choice = i
		}
	}
	return nil
}

func promptCreate(p *Project, cfg Settings, values createValues, choose bool) (createValues, error) {
	m := newCreateForm(cfg)
	m.setProject(p)
	if p != nil && p.Directory {
		if values.name == "" {
			values.name = filepath.Base(p.Root)
		}
	} else {
		m.inputs[1].SetValue(values.branch)
	}
	m.inputs[0].SetValue(values.name)
	if values.template != "" {
		m.template = values.template
	}
	m.pickerOnly = choose && values.name != ""
	if choose {
		if err := m.openPicker(); err != nil {
			return createValues{}, err
		}
	}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return createValues{}, err
	}
	m = result.(createForm)
	if m.cancelled || !m.done {
		return createValues{}, errCancelled
	}
	return m.values(), nil
}

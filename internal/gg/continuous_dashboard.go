package gg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type directActionMsg struct{ action popupAction }
type directFrameMsg struct{}

type terminalPaneFrame struct {
	Pane
	title            string
	lines            []string
	cursorX, cursorY int
	cursorVisible    bool
}

func directFrameTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return directFrameMsg{} })
}

func terminalDisconnected() bool {
	fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLHUP | unix.POLLERR}}
	n, err := unix.Poll(fds, 0)
	return err == nil && n > 0 && fds[0].Revents&(unix.POLLHUP|unix.POLLERR) != 0
}

// A dashboard invocation owns one model and one TTY for its entire lifetime.
// Project servers continue to own the actual terminal processes.
type continuousDashboard struct {
	side          sidebar
	project       *Project
	task          int
	pane          string
	frame         []terminalPaneFrame
	width, height int
	listFocus     bool
	form          *createForm
	formProject   *Project
	formCWD       string
	launchCWD     string
	folderForm    *sectionForm
	outerSocket   string
	outerSession  string
}

func (p *Project) attachContinuous() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New(p.text("dashboard requires an interactive terminal", "dashboard требует интерактивный терминал"))
	}
	if err := checkDependency("tmux", "3.4"); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	m := continuousDashboard{side: newSidebar(p, -1), project: p, task: -1, width: 120, height: 32, listFocus: true, launchCWD: cwd}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		m.outerSocket = strings.SplitN(os.Getenv("TMUX"), ",", 2)[0]
		if m.outerSocket != "" {
			m.outerSession, _ = command(p.Root, "tmux", "-S", m.outerSocket, "display-message", "-p", "-t", pane, "#{session_id}")
		}
	}
	m.side.direct, m.side.active = true, true
	m.pane = m.firstPane()
	_, err = tea.NewProgram(m).Run()
	return err
}

func (m continuousDashboard) Init() tea.Cmd {
	return tea.Batch(m.side.Init(), directFrameTick())
}

func (m continuousDashboard) firstPane() string {
	if m.task < 0 || m.project.WorkspaceID == 0 {
		return ""
	}
	for _, pane := range m.project.snapshot().task(m.task).live() {
		return pane.ID
	}
	return ""
}

func (m continuousDashboard) terminalPanes() Snapshot {
	if m.task < 0 || m.project.WorkspaceID == 0 {
		return nil
	}
	panes := m.project.snapshot().task(m.task).live()
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].Y != panes[j].Y {
			return panes[i].Y < panes[j].Y
		}
		return panes[i].X < panes[j].X
	})
	return panes
}

// Bubble Tea calls View after each input message. Sample tmux on the frame
// clock so forwarding a key does not recapture every pane in the window.
func (m *continuousDashboard) captureTerminalFrame() {
	panes := m.terminalPanes()
	m.frame = make([]terminalPaneFrame, len(panes))
	for i, pane := range panes {
		frame := &m.frame[i]
		frame.Pane = pane
		if compact(m.width, m.height) && m.listFocus {
			continue
		}
		info, err := m.project.tmux("display-message", "-p", "-t", pane.ID, "#{cursor_x} #{cursor_y} #{cursor_flag} #{pane_title}")
		if parts := strings.SplitN(info, " ", 4); err == nil && len(parts) >= 3 {
			x, xerr := strconv.Atoi(parts[0])
			y, yerr := strconv.Atoi(parts[1])
			frame.cursorX, frame.cursorY = x, y
			frame.cursorVisible = parts[2] == "1" && xerr == nil && yerr == nil
			if len(parts) == 4 {
				frame.title = parts[3]
			}
		}
		captured, err := m.project.tmux("capture-pane", "-p", "-e", "-N", "-t", pane.ID)
		if err == nil {
			// Decode the whole capture to retain styles across rows. Each
			// rendered line ends its own styling before an adjacent pane.
			canvas := lipgloss.NewCanvas(min(pane.Width, m.width), min(pane.Height, m.height))
			canvas.Compose(lipgloss.NewLayer(captured))
			frame.lines = strings.Split(canvas.Render(), "\n")
		}
	}
}

func (m continuousDashboard) syncTerminalSize() {
	if m.width < 1 || m.height < 1 {
		return
	}
	panes := m.terminalPanes()
	if len(panes) == 0 {
		return
	}
	_, _ = m.project.tmux("resize-window", "-t", panes[0].Window, "-x", strconv.Itoa(m.width), "-y", strconv.Itoa(max(2, m.height-1)))
	_, _ = m.project.tmux("set-window-option", "-t", panes[0].Window, "window-size", "latest")
	_ = m.project.resize()
}

func (m continuousDashboard) open(a popupAction) (tea.Model, tea.Cmd) {
	target := m.project
	if a.Workspace > 0 && a.Workspace != target.WorkspaceID {
		var err error
		target, err = workspaceByID(a.Workspace)
		if err != nil {
			m.side.message = err.Error()
			return m, nil
		}
	}
	if a.Kind == "split" {
		if err := target.addTerminal(a.ID, a.Direction); err != nil {
			m.side.message = err.Error()
			return m, nil
		}
	}
	if err := target.Start(a.ID); err != nil {
		m.side.message = err.Error()
		return m, nil
	}
	panes := target.snapshot().task(a.ID).live()
	if len(panes) == 0 {
		m.side.message = target.text("cannot open terminal", "не удалось открыть терминал")
		return m, nil
	}
	selected := target.returnPane(a.ID)
	if a.Kind == "open" && target.WorkspaceID == m.project.WorkspaceID && a.ID == m.task {
		for _, pane := range panes {
			if pane.ID == m.pane {
				selected = m.pane
				break
			}
		}
	}
	m.project, m.task, m.pane = target, a.ID, selected
	m.syncTerminalSize()
	m.side.p, m.side.task = target, a.ID
	m.side.message = ""
	m.listFocus = false
	m.side.active = false
	m.captureTerminalFrame()
	return m, requestSidebarRefresh
}

func (m continuousDashboard) action(a popupAction) (tea.Model, tea.Cmd) {
	switch a.Kind {
	case "detach":
		return m, tea.Quit
	case "open", "start", "split":
		return m.open(a)
	case "add-section":
		f := newSectionForm(m.launchCWD, m.side.cfg)
		m.folderForm = &f
		return m, f.path.Focus()
	case "remove-section":
		if err := removeSection(a.Section); err != nil {
			m.side.message = err.Error()
			return m, nil
		}
		m.side.message = ""
		return m, requestSidebarRefresh
	case "terminal":
		section, target, err := sectionByID(a.Section)
		if err != nil {
			m.side.message = err.Error()
			return m, nil
		}
		t, err := target.createTerminal(section.Path, "", false, "", "shell")
		if err != nil {
			m.side.message = err.Error()
			return m, nil
		}
		m.side.clearSearch()
		m.side.onlyActive = false
		m.side.cursorID, m.side.cursorGroup, m.side.cursorFolder = t.ID, a.Section, false
		return m.open(popupAction{Kind: "open", Workspace: a.Workspace, ID: t.ID})
	case "create":
		target := m.project
		if a.Workspace > 0 && a.Workspace != target.WorkspaceID {
			var err error
			target, err = workspaceByID(a.Workspace)
			if err != nil {
				m.side.message = err.Error()
				return m, nil
			}
		}
		cwd := target.Root
		if a.Section > 0 {
			section, resolved, err := sectionByID(a.Section)
			if err != nil {
				m.side.message = err.Error()
				return m, nil
			}
			target, cwd = resolved, section.Path
		}
		if a.ID >= 0 {
			state, err := target.load()
			if err != nil {
				m.side.message = err.Error()
				return m, nil
			}
			task, err := taskByID(state, a.ID)
			if err != nil {
				m.side.message = err.Error()
				return m, nil
			}
			cwd = task.Path
		} else if run, err := readRun(os.Getpid()); !a.Folder && err == nil && len(run.Origins) > 0 {
			cwd = run.Origins[len(run.Origins)-1].Path
		}
		cfg, err := target.settings()
		if err != nil {
			m.side.message = err.Error()
			return m, nil
		}
		form := newCreateForm(cfg)
		form.p = target
		form.context = target.workspaceView().Name + " · " + cwd
		if !target.Directory {
			form.base, _ = command(cwd, "git", "symbolic-ref", "--short", "HEAD")
			if form.base == "" {
				form.base, _ = command(cwd, "git", "rev-parse", "--short", "HEAD")
			}
		}
		if target.Directory {
			form.labels[0] = cfg.text("Name (optional)", "Имя (необязательно)")
			form.inputs[0].SetValue(filepath.Base(cwd))
		}
		m.form, m.formProject, m.formCWD = &form, target, cwd
		m.sizeForm()
		return m, form.Init()
	case "stop", "delete":
		target := m.project
		if a.Workspace > 0 && a.Workspace != target.WorkspaceID {
			var err error
			target, err = workspaceByID(a.Workspace)
			if err != nil {
				m.side.message = err.Error()
				return m, nil
			}
		}
		ids := a.IDs
		if !a.Bulk {
			ids = []int{a.ID}
		}
		closing := false
		for _, id := range ids {
			closing = closing || id == m.task
		}
		if closing && target.WorkspaceID == m.project.WorkspaceID {
			m.listFocus = true
			m.side.active = true
		}
		if err := target.operationInBackground(a.Kind, ids, false, a.Bulk, a.Bulk); err != nil {
			m.side.message = err.Error()
		}
		return m, requestSidebarRefresh
	}
	return m, nil
}

func (m continuousDashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(directFrameMsg); ok {
		if terminalDisconnected() {
			return m, tea.Quit
		}
		if m.outerSession != "" {
			clients, err := command(m.project.Root, "tmux", "-S", m.outerSocket, "list-clients", "-F", "#{session_id}")
			if err == nil && !strings.Contains("\n"+clients+"\n", "\n"+m.outerSession+"\n") {
				return m, tea.Quit
			}
		}
		m.captureTerminalFrame()
		if len(m.frame) == 0 {
			m.listFocus, m.side.active = true, true
			m.task, m.side.task, m.pane = -1, -1, ""
		}
		return m, directFrameTick()
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		m.side.screenWidth, m.side.screenHeight = m.width, max(1, m.height-1)
		m.side.width, m.side.height = sidebarWidth(m.width), max(1, m.height-1)
		if compact(m.width, m.height) {
			m.side.width = m.width
		}
		m.side.search.SetWidth(max(4, m.side.width-5))
		m.sizeForm()
	}
	if m.folderForm != nil {
		switch msg.(type) {
		case tickMsg, refreshMsg, actionMsg:
			return m.updateSidebar(msg)
		case tea.WindowSizeMsg:
			return m, resetViewport()
		}
		return m.updateSectionForm(msg)
	}
	if m.form != nil {
		switch msg.(type) {
		case tickMsg, refreshMsg, actionMsg:
			return m.updateSidebar(msg)
		}
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			m.syncTerminalSize()
			m.captureTerminalFrame()
			return m, resetViewport()
		}
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+w" {
			return m, nil
		}
		if mouse, ok := msg.(tea.MouseClickMsg); ok {
			frame := m.formFrame()
			for _, h := range frame.hits {
				if containsHit(h, mouse.X, mouse.Y) && mouse.Button == tea.MouseLeft {
					if strings.HasPrefix(h.key, "template:") {
						m.form.choice, _ = strconv.Atoi(strings.TrimPrefix(h.key, "template:"))
						msg = commandKey("enter")
					} else if h.key == "name" {
						return m, m.form.inputs[0].Focus()
					} else {
						msg = commandKey(h.key)
					}
					break
				}
			}
			if _, unchanged := msg.(tea.MouseClickMsg); unchanged {
				return m, nil
			}
		}
		form, cmd := m.form.Update(msg)
		updated := form.(createForm)
		m.form = &updated
		if updated.cancelled {
			m.form = nil
			return m, requestSidebarRefresh
		}
		if updated.done {
			name := strings.TrimSpace(updated.inputs[0].Value())
			task, err := m.formProject.Create(m.formCWD, name, "", false, updated.template)
			if err != nil {
				m.form.done = false
				m.form.message = err.Error()
				return m, nil
			}
			m.form = nil
			m.side.clearSearch()
			m.side.onlyActive = false
			m.side.cursorID, m.side.cursorGroup, m.side.cursorFolder = task.ID, m.formProject.SectionID, false
			return m.open(popupAction{Kind: "open", Workspace: m.formProject.WorkspaceID, ID: task.ID})
		}
		return m, cmd
	}
	if m.side.modal != "" || m.side.confirming {
		switch input := msg.(type) {
		case tea.KeyPressMsg:
			if input.String() == "ctrl+w" {
				return m, nil
			}
		case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.PasteMsg:
		default:
			goto normalInput
		}
		return m.updateSidebar(msg)
	}
normalInput:
	switch msg := msg.(type) {
	case directActionMsg:
		return m.action(msg.action)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncTerminalSize()
		m.captureTerminalFrame()
		width := sidebarWidth(m.width)
		if compact(m.width, m.height) {
			width = m.width
		}
		msg.Width = width
		msg.Height = max(1, m.height-1)
		return m.updateSidebar(msg)
	case tea.KeyPressMsg:
		if msg.String() == "esc" && m.listFocus && compact(m.width, m.height) &&
			m.side.modal == "" && !m.side.confirming && !m.side.searching && m.side.query == "" && len(m.frame) > 0 {
			m.listFocus, m.side.active = false, false
			m.captureTerminalFrame()
			return m, nil
		}
		if msg.String() == "ctrl+w" {
			m.side.searching = false
			m.side.search.Blur()
			m.side.hover = nil
			if m.listFocus {
				if len(m.frame) == 0 {
					return m, nil
				}
				m.listFocus = false
				m.pane = m.frame[0].ID
				if compact(m.width, m.height) {
					m.captureTerminalFrame()
				}
			} else {
				panes := m.frame
				found := false
				for i, pane := range panes {
					if pane.ID == m.pane && i+1 < len(panes) {
						m.pane = panes[i+1].ID
						found = true
						break
					}
				}
				if !found {
					m.listFocus = true
					m.side.focusOpenTask()
				}
			}
			m.side.active = m.listFocus
			return m, nil
		}
		if !m.listFocus {
			m.sendKey(msg.String())
			return m, nil
		}
	case tea.PasteMsg:
		if !m.listFocus && m.pane != "" {
			_, _ = m.project.tmux("send-keys", "-l", "-t", m.pane, msg.Content)
			return m, nil
		}
	case tea.MouseClickMsg:
		if (compact(m.width, m.height) && m.listFocus) ||
			(!compact(m.width, m.height) && msg.X < sidebarWidth(m.width)) {
			m.listFocus, m.side.active = true, true
			return m.updateSidebar(msg)
		}
		if len(m.terminalPanes()) == 0 {
			return m, nil
		}
		m.listFocus, m.side.active = false, false
		m.sendMouse(tea.Mouse(msg), false, false)
		return m, nil
	case tea.MouseWheelMsg:
		if (compact(m.width, m.height) && m.listFocus) ||
			(!compact(m.width, m.height) && msg.X < sidebarWidth(m.width)) {
			m.listFocus, m.side.active = true, true
			return m.updateSidebar(msg)
		}
		if !m.listFocus {
			m.sendMouse(tea.Mouse(msg), false, false)
			return m, nil
		}
	case tea.MouseReleaseMsg:
		if !m.listFocus {
			m.sendMouse(tea.Mouse(msg), true, false)
			return m, nil
		}
	case tea.MouseMotionMsg:
		if (compact(m.width, m.height) && m.listFocus) || (!compact(m.width, m.height) && msg.X < sidebarWidth(m.width)) {
			m.side = m.side.mouseMotion(msg)
			return m, nil
		}
		m.side.hover = nil
		if !m.listFocus {
			m.sendMouse(tea.Mouse(msg), false, true)
		}
		return m, nil
	}
	if m.listFocus {
		m.side.active = true
		return m.updateSidebar(msg)
	}
	if _, ok := msg.(refreshMsg); ok {
		m.side.active = false
		return m.updateSidebar(msg)
	}
	if _, ok := msg.(tickMsg); ok {
		m.side.active = false
		return m.updateSidebar(msg)
	}
	return m, nil
}

func (m continuousDashboard) sendKey(key string) {
	if m.pane == "" {
		return
	}
	if key == "space" {
		key = " "
	}
	switch key {
	case "enter", "ctrl+j", "ctrl+m":
		key = "Enter"
	case "backspace":
		key = "BSpace"
	case "delete":
		key = "DC"
	case "esc":
		key = "Escape"
	case "shift+tab":
		key = "BTab"
	case "up", "down", "left", "right", "tab", "home", "end", "pgup", "pgdown":
		key = strings.ToUpper(key[:1]) + key[1:]
	}
	key = strings.ReplaceAll(key, "ctrl+", "C-")
	key = strings.ReplaceAll(key, "alt+", "M-")
	if len(key) == 1 {
		_, _ = m.project.tmux("send-keys", "-l", "-t", m.pane, key)
	} else {
		_, _ = m.project.tmux("send-keys", "-t", m.pane, key)
	}
}

func (m *continuousDashboard) sendMouse(mouse tea.Mouse, release, motion bool) {
	left := 0
	if !compact(m.width, m.height) {
		left = sidebarWidth(m.width) + 1
	}
	x := mouse.X - left
	y := mouse.Y - 1
	if x < 0 || y < 0 {
		return
	}
	panes := m.terminalPanes()
	if len(panes) == 0 {
		return
	}
	minX, minY := panes[0].X, panes[0].Y
	for _, pane := range panes {
		minX = min(minX, pane.X)
		minY = min(minY, pane.Y)
	}
	for _, pane := range panes {
		px, py := pane.X-minX, pane.Y-minY
		if x < px || x >= px+pane.Width || y < py || y >= py+pane.Height {
			continue
		}
		if !motion {
			m.pane = pane.ID
		}
		mode, _ := m.project.tmux("display-message", "-p", "-t", pane.ID, "#{mouse_any_flag}")
		if mode != "1" {
			if mouse.Button == tea.MouseWheelUp {
				_, _ = m.project.tmux("copy-mode", "-u", "-t", pane.ID)
			} else if mouse.Button == tea.MouseWheelDown {
				_, _ = m.project.tmux("send-keys", "-X", "-t", pane.ID, "scroll-down")
			}
			return
		}
		button := 0
		switch mouse.Button {
		case tea.MouseNone:
			button = 3
		case tea.MouseMiddle:
			button = 1
		case tea.MouseRight:
			button = 2
		case tea.MouseWheelUp:
			button = 64
		case tea.MouseWheelDown:
			button = 65
		}
		if motion {
			button |= 32
		}
		if mouse.Mod&tea.ModShift != 0 {
			button |= 4
		}
		if mouse.Mod&tea.ModAlt != 0 {
			button |= 8
		}
		if mouse.Mod&tea.ModCtrl != 0 {
			button |= 16
		}
		suffix := "M"
		if release {
			suffix = "m"
		}
		sgr, _ := m.project.tmux("display-message", "-p", "-t", pane.ID, "#{mouse_sgr_flag}")
		var sequence []byte
		if sgr == "1" {
			sequence = []byte(fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x-px+1, y-py+1, suffix))
		} else {
			if release {
				button = 3
			}
			sequence = []byte{0x1b, '[', 'M', byte(32 + button), byte(33 + x - px), byte(33 + y - py)}
		}
		args := []string{"send-keys", "-H", "-t", pane.ID}
		for _, b := range sequence {
			args = append(args, fmt.Sprintf("%02x", b))
		}
		_, _ = m.project.tmux(args...)
		return
	}
}

func (m continuousDashboard) terminalView(width, height int) []string {
	lines := make([]string, height)
	if width < 1 || height < 1 {
		return lines
	}
	panes := m.frame
	if len(panes) == 0 {
		message := m.side.cfg.text("No terminal open", "Нет открытого терминала")
		lines[height/2] = strings.Repeat(" ", max(0, (width-ansi.StringWidth(message))/2)) + ansi.Truncate(message, width, "…")
		if height/2+1 < height {
			message = m.side.cfg.text("Select a session and press Enter", "Выберите сессию и нажмите Enter")
			lines[height/2+1] = strings.Repeat(" ", max(0, (width-ansi.StringWidth(message))/2)) + ansi.Truncate(message, width, "…")
		}
		return lines
	}
	minX, minY := panes[0].X, panes[0].Y
	for _, pane := range panes {
		minX = min(minX, pane.X)
		minY = min(minY, pane.Y)
	}
	type segment struct {
		x, width int
		text     string
	}
	segments := make([][]segment, height)
	for _, pane := range panes {
		x, titleY := pane.X-minX, pane.Y-minY
		span := max(0, min(pane.Width, width-x))
		if span > 0 && titleY >= 0 && titleY < height {
			title := ansi.Truncate(oneLine(pane.title), max(0, span-3), "")
			header := strings.Repeat("─", span)
			if span >= 3 {
				header = "─ " + title + " " + strings.Repeat("─", max(0, span-3-ansi.StringWidth(title)))
			}
			if pane.ID == m.pane && !m.listFocus {
				header = accent.Render("▸" + strings.TrimPrefix(header, "─"))
			} else {
				header = dim.Render(header)
			}
			segments[titleY] = append(segments[titleY], segment{x, span, header})
		}
		if span == 0 {
			continue
		}
		y := 1 + pane.Y - minY
		for i, line := range pane.lines {
			if y+i >= height || y+i < 0 {
				continue
			}
			if span < pane.Width {
				line = ansi.Truncate(line, span, "")
			}
			segments[y+i] = append(segments[y+i], segment{x, span, line})
		}
		if x > 0 {
			for row := max(0, titleY); row < min(height, y+pane.Height); row++ {
				segments[row] = append(segments[row], segment{x - 1, 1, ruleStyle.Render("│")})
			}
		}
	}
	for y, row := range segments {
		sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
		column := 0
		for _, part := range row {
			if part.x < column {
				continue
			}
			lines[y] += strings.Repeat(" ", part.x-column) + part.text
			lines[y] += strings.Repeat(" ", max(0, part.width-ansi.StringWidth(part.text)))
			column = part.x + part.width
		}
	}
	return lines
}

func (m *continuousDashboard) sizeForm() {
	if m.form == nil {
		return
	}
	framedDialog(max(1, m.width), max(1, m.height-1), 58, nil, func(width, height int) (string, []hit) {
		form, _ := m.form.Update(tea.WindowSizeMsg{Width: width, Height: height})
		updated := form.(createForm)
		m.form = &updated
		return m.form.renderContent(width, height)
	})
}

func (m continuousDashboard) formFrame() dialogFrame {
	return framedDialog(max(1, m.width), max(1, m.height-1), 58, nil, m.form.renderContent)
}

func (m continuousDashboard) View() tea.View {
	v := m.baseView()
	if m.height > 1 && m.folderForm != nil {
		v.Content = overlayDialog(v.Content, m.sectionFrame())
		v.Cursor = nil
	} else if m.height > 1 && m.form != nil {
		v.Content = overlayDialog(v.Content, m.formFrame())
		v.Cursor = nil
	} else if m.height > 1 && (m.side.modal != "" || m.side.confirming) {
		v.Content = overlayDialog(v.Content, m.side.dialogOverlay(max(1, m.width), max(1, m.height-1)))
		v.Cursor = nil
	}
	v.Content = dashboardSurface(v.Content, m.width, m.height)
	return v
}

func (m continuousDashboard) terminalCursor(origin int) *tea.Cursor {
	panes := m.frame
	if len(panes) == 0 || m.listFocus {
		return nil
	}
	minX, minY := panes[0].X, panes[0].Y
	for _, pane := range panes {
		minX = min(minX, pane.X)
		minY = min(minY, pane.Y)
	}
	for _, pane := range panes {
		if pane.ID != m.pane || !pane.cursorVisible {
			continue
		}
		return tea.NewCursor(origin+pane.X-minX+pane.cursorX, 1+pane.Y-minY+pane.cursorY)
	}
	return nil
}

func (m continuousDashboard) terminalTitle() string {
	for _, pane := range m.frame {
		if pane.ID == m.pane && pane.title != "" {
			return pane.title
		}
	}
	return "Grand Git"
}

var tmuxStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#151B12")).Background(lipgloss.Color("#9ECE6A"))

func (m continuousDashboard) statusLine(width int) string {
	active, total := 0, 0
	for _, group := range m.side.groups {
		if group.Err != nil || group.State == nil {
			continue
		}
		for _, task := range group.State.Tasks {
			total++
			if len(group.Snap.task(task.ID).live()) > 0 {
				active++
			}
		}
	}
	left := " Grand Git │ "
	if m.task >= 0 {
		left = " Grand Git · " + oneLine(filepath.Base(m.project.Root)) + " │ "
	}
	right := m.side.cfg.text("Active", "Активно") + " " + strconv.Itoa(active) + "/" + strconv.Itoa(total) + " · Ctrl+W " + m.side.cfg.text("Next", "Далее") + " "
	right = ansi.Truncate(right, width, "")
	left = ansi.Truncate(left, max(0, width-ansi.StringWidth(right)), "")
	line := left + strings.Repeat(" ", max(0, width-ansi.StringWidth(left)-ansi.StringWidth(right))) + right
	return tmuxStatusStyle.Render(line)
}

func (m continuousDashboard) baseView() tea.View {
	width, height := max(1, m.width), max(1, m.height)
	m.side.active = m.listFocus
	if height == 1 {
		v := tea.NewView(m.statusLine(width))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeAllMotion
		return v
	}
	if compact(width, height) {
		if m.listFocus {
			m.side.width, m.side.height = width, height-1
			side, _ := m.side.renderBase()
			v := tea.NewView(side + "\n" + m.statusLine(width))
			v.AltScreen = true
			v.MouseMode = tea.MouseModeAllMotion
			return v
		}
		v := tea.NewView(strings.Join(m.terminalView(width, height-1), "\n") + "\n" + m.statusLine(width))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeAllMotion
		v.Cursor = m.terminalCursor(0)
		v.WindowTitle = m.terminalTitle()
		return v
	}
	sideWidth := sidebarWidth(width)
	m.side.width, m.side.height = sideWidth, height-1
	side, _ := m.side.renderBase()
	left := strings.Split(side, "\n")
	right := m.terminalView(width-sideWidth-1, height-1)
	lines := make([]string, height-1)
	for i := range lines {
		l := ""
		if i < len(left) {
			l = ansi.Truncate(left[i], sideWidth, "")
		}
		l += strings.Repeat(" ", max(0, sideWidth-ansi.StringWidth(l)))
		lines[i] = l + "│" + right[i]
	}
	v := tea.NewView(strings.Join(lines, "\n") + "\n" + m.statusLine(width))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.Cursor = m.terminalCursor(sideWidth + 1)
	v.WindowTitle = m.terminalTitle()
	return v
}

func (m continuousDashboard) updateSidebar(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.side.Update(msg)
	m.side = next.(sidebar)
	if action := m.side.pendingAction; action != nil {
		m.side.pendingAction = nil
		return m.action(*action)
	}
	return m, cmd
}

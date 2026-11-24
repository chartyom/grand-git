package gg

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type listTarget struct {
	Group, ID int
	Folder    bool
}

type screenPoint struct{ X, Y int }

func (m sidebar) items() []listTarget {
	rows := m.rows()
	items := []listTarget{}
	if len(m.groups) == 0 {
		for _, t := range rows {
			items = append(items, listTarget{Group: t.Group, ID: t.ID})
		}
		return items
	}
	for _, g := range m.groups {
		entries := []listTarget{}
		for _, t := range rows {
			if t.Group == g.View.ID {
				entries = append(entries, listTarget{Group: t.Group, ID: t.ID})
			}
		}
		if !m.onlyActive && g.Err == nil && len(g.Jobs) != 0 {
			for _, job := range g.Jobs {
				if m.query == "" || strings.Contains(strings.ToLower(job.Name+" "+job.Branch+" "+g.View.Name+" "+g.View.Path), strings.ToLower(m.query)) {
					entries = append(entries, listTarget{Group: g.View.ID, ID: job.ID})
				}
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		}
		if len(entries) > 0 || g.Err != nil || (m.query == "" && !m.onlyActive) {
			items = append(items, listTarget{Group: g.View.ID, Folder: true})
			items = append(items, entries...)
		}
	}
	return items
}

func (m sidebar) selectedTarget() *listTarget {
	items := m.items()
	if m.selected < 0 || m.selected >= len(items) {
		return nil
	}
	target := items[m.selected]
	return &target
}

func (m *sidebar) selectTarget(target listTarget) {
	m.cursorGroup, m.cursorID, m.cursorFolder = target.Group, target.ID, target.Folder
	m.setState(m.state)
}

func (m sidebar) selectedGroup() *sessionGroup {
	if target := m.selectedTarget(); target != nil {
		return m.group(target.Group)
	}
	return nil
}

func (m sidebar) actionTarget() *listTarget {
	if m.modal == "actions" && m.menuTarget != nil {
		return m.menuTarget
	}
	return m.selectedTarget()
}

func (m *sidebar) openActions(target *listTarget, anchor *screenPoint) {
	if job := m.job(target); anchor != nil && job != nil && job.Phase != "failed" {
		return
	}
	m.modal, m.actionIndex = "actions", 0
	m.menuTarget, m.menuAnchor = target, anchor
	m.searching = false
	m.search.Blur()
	m.hover = nil
}

func (m *sidebar) beginConfirmation(action string, target listTarget) {
	m.confirmIDs = nil
	m.confirmBulk = target.Folder
	m.confirmGroup, m.confirmID = target.Group, target.ID
	m.confirmWorkspace, m.confirmSection = target.Group, 0
	if g := m.group(target.Group); g != nil {
		m.confirmWorkspace, m.confirmSection = g.workspaceID(), g.Section
	}
	m.confirmAction = action
	m.hover = nil
	if target.Folder {
		if g := m.group(target.Group); g != nil && g.Err == nil && g.State != nil {
			if action == "relative-paths" {
				m.confirmName = g.Project.Root
				m.confirming = true
				return
			}
			m.confirmName = g.View.Name
			for _, t := range g.State.Tasks {
				if action == "delete" || len(g.Snap.task(t.ID).live()) > 0 {
					m.confirmIDs = append(m.confirmIDs, t.ID)
				}
			}
		}
	} else if t, err := m.findTask(target.Group, target.ID); err == nil && (action != "delete" || !t.Protected) {
		m.confirmIDs = []int{t.ID}
		m.confirmName = m.displayName(t)
	}
	m.confirming = len(m.confirmIDs) > 0
}

func (m sidebar) dispatch(key string, target *listTarget) (tea.Model, tea.Cmd) {
	if target != nil {
		if g := m.group(target.Group); g != nil && g.Err != nil {
			m.message = g.Err.Error()
			return m, nil
		}
		if target.Folder {
			g := m.group(target.Group)
			if g == nil {
				m.message = m.cfg.text("Section no longer exists", "Раздел больше не существует")
				return m, nil
			}
			switch key {
			case "enter", "?", "shift+f10", "contextmenu":
				m.openActions(target, nil)
			case "n":
				return m.perform(popupAction{Kind: "create", Workspace: target.Group, ID: -1, Folder: true})
			case "t":
				return m.perform(popupAction{Kind: "terminal", Workspace: target.Group, ID: -1, Folder: true})
			case "x":
				if g.Section > 0 && g.State != nil && len(g.State.Tasks) == 0 && !g.hasPendingWorktree() {
					return m.perform(popupAction{Kind: "remove-section", Workspace: target.Group, Folder: true})
				}
			case "r":
				if m.direct && g.Section > 0 && g.View.Type == "git" {
					m.beginConfirmation("relative-paths", *target)
				}
			case "s":
				m.beginConfirmation("stop", *target)
			case "d":
				m.beginConfirmation("delete", *target)
			}
			return m, nil
		}
		if job := m.job(target); job != nil {
			if key == "enter" && job.Phase == "failed" {
				return m.perform(popupAction{Kind: "retry-worktree", Workspace: target.Group, ID: target.ID})
			}
			return m, nil
		}
		t, err := m.findTask(target.Group, target.ID)
		if err != nil {
			m.message = m.cfg.text("Session no longer exists", "Сессия больше не существует")
			return m, nil
		}
		switch key {
		case "enter":
			return m.perform(m.action("open", t))
		case "n":
			return m.perform(m.action("create", t))
		case "t", "s":
			if len(m.taskSnapshot(t).task(t.ID).live()) == 0 {
				return m.perform(m.action("start", t))
			}
			if key == "t" {
				m.modal, m.splitChoice = "split", 0
				m.modalID, m.modalGroup = t.ID, t.Group
				m.modalWorkspace, m.modalSection = t.Group, 0
				if g := m.group(t.Group); g != nil {
					m.modalWorkspace, m.modalSection = g.workspaceID(), g.Section
				}
			} else {
				m.beginConfirmation("stop", *target)
			}
		case "d":
			m.beginConfirmation("delete", *target)
		}
	} else if key == "n" && !m.direct {
		return m.perform(popupAction{Kind: "create", ID: -1})
	}
	return m, nil
}

func commandKey(key string) tea.KeyPressMsg {
	switch strings.ToLower(key) {
	case "enter", "\r":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	runes := []rune(key)
	if len(runes) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Code: runes[0]}
}

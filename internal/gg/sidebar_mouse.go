package gg

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func containsHit(h hit, x, y int) bool { return y == h.y && x >= h.x1 && x < h.x2 }

func (m sidebar) mouseMotion(msg tea.MouseMotionMsg) sidebar {
	if m.modal != "" || m.confirming {
		return m
	}
	m.hover = nil
	_, hits := m.renderBase()
	for _, h := range hits {
		if h.row && containsHit(h, msg.X, msg.Y) {
			m.hover = &listTarget{Group: h.group, ID: h.id, Folder: h.folder}
			break
		}
	}
	return m
}

func (m sidebar) mouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	client := m.client
	if !m.popup && !m.direct {
		var err error
		client, err = m.p.takeMouseClient(m.pane)
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		client, err = m.p.mouseActionClient(client)
		if err != nil || client == "" {
			if err != nil {
				m.message = err.Error()
			}
			return m, nil
		}
	}
	if msg.Button != tea.MouseLeft && msg.Button != tea.MouseRight {
		return m, nil
	}
	_, hits := m.render()
	modal := m.modal != "" || m.confirming
	if modal && m.direct {
		hits = m.dialogOverlay(m.screenWidth, m.screenHeight).hits
	}
	for _, h := range hits {
		if !containsHit(h, msg.X, msg.Y) {
			continue
		}
		if !modal && h.row {
			target := listTarget{Group: h.group, ID: h.id, Folder: h.folder}
			m.selectTarget(target)
			m.active, m.searching = true, false
			m.search.Blur()
			m.hover = nil
			if msg.Button == tea.MouseRight {
				m.openActions(&target, &screenPoint{msg.X, msg.Y})
				return m, nil
			}
			if !target.Folder {
				if m.job(&target) != nil {
					return m.dispatch("enter", &target)
				}
				return m.perform(popupAction{Kind: "open", Workspace: target.Group, ID: target.ID, Client: client})
			}
			return m, nil
		}
		if msg.Button != tea.MouseLeft {
			break
		}
		m.eventClient = client
		switch h.key {
		case "clear":
			m.query = ""
			m.search.SetValue("")
			m.setState(m.state)
			return m, m.search.Focus()
		case "↓":
			return m.Update(commandKey("down"))
		case "N/Esc":
			return m.Update(commandKey("esc"))
		case "Q":
			return m.perform(popupAction{Kind: "detach", Client: client})
		}
		if strings.HasPrefix(h.key, "setting:") {
			m.settingField, _ = strconv.Atoi(strings.TrimPrefix(h.key, "setting:"))
			return m.Update(commandKey("right"))
		}
		if h.key == "/" && m.searching {
			return m, m.search.Focus()
		}
		return m.Update(commandKey(h.key))
	}
	if m.modal == "actions" && m.menuAnchor != nil {
		m.modal, m.menuAnchor = "", nil
	}
	return m, nil
}

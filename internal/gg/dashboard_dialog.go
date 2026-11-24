package gg

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type dialogFrame struct {
	content             string
	x, y, width, height int
	hits                []hit
}

func framedDialog(width, height, preferred int, anchor *screenPoint, render func(int, int) (string, []hit)) dialogFrame {
	w := max(1, min(preferred, width-2))
	border := w >= 6 && height >= 5
	inset := 0
	if border {
		inset = 2
	}
	body, hits := render(max(1, w-inset*2), max(1, height-2-inset))
	if border {
		body = panelStyle.Width(w).Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#73DACA")).Render(body)
	}
	w, h := lipgloss.Width(body), lipgloss.Height(body)
	x, y := max(0, (width-w)/2), max(0, (height-h)/2)
	if anchor != nil {
		x, y = max(0, min(anchor.X, width-w)), max(0, min(anchor.Y, height-h))
	}
	for i := range hits {
		hits[i].x1 += x + inset
		hits[i].x2 += x + inset
		hits[i].y += y
		if border {
			hits[i].y++
		}
	}
	return dialogFrame{content: body, x: x, y: y, width: w, height: h, hits: hits}
}

func overlayDialog(base string, frame dialogFrame) string {
	return lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(frame.content).X(frame.x).Y(frame.y).Z(1)).Render()
}

func fitDialog(body, footer textCanvas, height, focus int) (string, []hit) {
	budget := max(1, height-len(footer.lines))
	if len(body.lines) > budget {
		start := max(1, min(len(body.lines)-budget+1, focus-budget+2))
		visible := textCanvas{width: body.width}
		visible.add(body.lines[0])
		end := min(len(body.lines), start+budget-1)
		visible.lines = append(visible.lines, body.lines[start:end]...)
		for _, h := range body.hits {
			if h.y >= start && h.y < end {
				h.y = h.y - start + 1
				visible.hits = append(visible.hits, h)
			}
		}
		body = visible
	}
	body.append(footer)
	return body.finish(height)
}

func (m sidebar) dialogOverlay(width, height int) dialogFrame {
	preferred := 58
	var anchor *screenPoint
	if m.modal == "actions" && m.menuAnchor != nil {
		preferred, anchor = 34, m.menuAnchor
	}
	return framedDialog(width, height, preferred, anchor, m.dialog)
}

func (m sidebar) dialog(width, height int) (string, []hit) {
	c, footer := textCanvas{width: width}, textCanvas{width: width}
	focus := 1
	if m.confirming && m.confirmAction == "relative-paths" {
		c.add(accent.Render(m.cfg.text("Enable relative worktree paths?", "Включить относительные пути worktree?")))
		c.add(dim.Render(oneLine(m.confirmName)))
		text := m.cfg.text(
			"Confirm Git 2.48 or newer on the host and every Git client using this repository. Git will use relative paths for all existing and future worktrees.",
			"Подтвердите, что на хосте и у всех клиентов этого репозитория Git версии 2.48 или новее. Git включит относительные пути для всех существующих и новых worktree.")
		for _, line := range strings.Split(ansi.Wrap(text, width, ""), "\n") {
			c.add(line)
		}
		footer.buttons(shortcut{"Y", m.cfg.text("Enable", "Включить")}, shortcut{"N/Esc", m.cfg.text("Cancel", "Отмена")})
	} else if m.confirming {
		verb := m.cfg.text("Stop", "Остановить")
		if m.confirmAction == "delete" {
			verb = m.cfg.text("Delete", "Удалить")
		}
		title := verb + m.cfg.text(" session", " сессию")
		if m.confirmBulk {
			title = verb + m.cfg.text(" all sessions", " все сессии")
		}
		c.add(accent.Render(title))
		c.add(danger.Render(verb + " " + oneLine(m.confirmName) + "?"))
		if g := m.group(m.confirmGroup); g != nil {
			c.add(dim.Render(g.View.Name + " · " + g.View.Path))
		}
		c.add(m.cfg.text("Processes will stop.", "Процессы будут остановлены."))
		if m.confirmBulk {
			c.add(m.cfg.text("Sessions: ", "Сессий: ") + strconv.Itoa(len(m.confirmIDs)))
		}
		if m.confirmAction == "delete" {
			text := m.cfg.text("Directory files will remain.", "Файлы директории сохранятся.")
			if m.confirmBulk {
				text = m.cfg.text("Managed worktrees will be removed. Protected folders, ordinary directory files and Git branches will remain.", "Worktree-сессии будут удалены. Защищённые папки, файлы обычных директорий и Git-ветки сохранятся.")
			} else if t, err := m.findTask(m.confirmGroup, m.confirmID); err == nil && !t.plain() {
				text = m.cfg.text("Worktree will be removed. Existing Git branches will remain.", "Рабочая копия будет удалена. Существующие ветки сохранятся.")
			}
			for _, line := range strings.Split(ansi.Wrap(text, width, ""), "\n") {
				c.add(line)
			}
		}
		footer.buttons(shortcut{"Y", verb}, shortcut{"N/Esc", m.cfg.text("Cancel", "Отмена")})
	} else {
		switch m.modal {
		case "actions":
			c.add(accent.Render(m.cfg.text("Actions", "Действия")))
			for i, item := range m.shortcuts() {
				prefix := "  "
				if i == m.actionIndex {
					prefix = "› "
					focus = len(c.lines)
				}
				line := prefix + "[" + item.key + "] " + item.label
				if i == m.actionIndex {
					line = selectedStyle.Width(width).Render(ansi.Truncate(line, width, "…"))
				}
				c.control(item.key, line)
			}
			footer.buttons(shortcut{"Esc", m.cfg.text("Back", "Назад")})
		case "split":
			c.add(accent.Render(m.cfg.text("Add terminal", "Добавить терминал")))
			for i, item := range []shortcut{{"B", m.cfg.text("Below", "Внизу")}, {"R", m.cfg.text("Right", "Справа")}} {
				line := "  [" + item.key + "] " + item.label
				if i == m.splitChoice {
					line = selectedStyle.Width(width).Render(ansi.Truncate("›"+line[1:], width, "…"))
					focus = len(c.lines)
				}
				c.control(item.key, line)
			}
			footer.buttons(shortcut{"Esc", m.cfg.text("Cancel", "Отмена")})
		case "settings":
			c.add(accent.Render(m.cfg.text("Settings", "Настройки")))
			c.add(dim.Render(m.cfg.text("Open project: ", "Открытый проект: ") + m.p.workspaceView().Name))
			scope := m.cfg.text("Global", "Глобально")
			if m.projectSettings {
				scope = m.cfg.text("Local", "Локально")
			}
			for i, line := range []string{m.cfg.text("Language: ", "Язык: ") + m.draft.Language, m.cfg.text("Template: ", "Шаблон: ") + m.draft.DefaultTemplate, m.cfg.text("Save to: ", "Сохранить в: ") + scope} {
				if i == m.settingField {
					line = selectedStyle.Width(width).Render(ansi.Truncate("› "+line, width, "…"))
					focus = len(c.lines)
				} else {
					line = "  " + line
				}
				c.control("setting:"+strconv.Itoa(i), line)
			}
			if m.message != "" {
				c.add(danger.Render(localizeError(m.message, m.cfg.Language)))
			}
			footer.buttons(shortcut{"Enter", m.cfg.text("Save", "Сохранить")}, shortcut{"Esc", m.cfg.text("Cancel", "Отмена")})
		}
	}
	return fitDialog(c, footer, height, focus)
}

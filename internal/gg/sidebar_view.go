package gg

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func (m sidebar) displayName(t *Task) string {
	if t.Kind == "temporary" || m.taskSnapshot(t).task(t.ID).temporary() {
		return "dashboard"
	}
	return oneLine(t.label())
}

func (m sidebar) gitSnapshot(t *Task) gitCache {
	if g := m.group(t.Group); g != nil {
		return g.Git
	}
	return m.git
}

func (m sidebar) gitState(t *Task) string {
	if t.plain() {
		return "n/a"
	}
	if value := m.gitSnapshot(t).States[strconv.Itoa(t.ID)]; value != "" {
		return value
	}
	return "unknown"
}

func (m sidebar) agentStatus(t *Task) string {
	owner := m.p
	if g := m.group(t.Group); g != nil {
		owner = g.Project
	}
	return owner.status(t, m.taskSnapshot(t))
}

func pathTail(path string, width int) string {
	path = oneLine(path)
	if size := ansi.StringWidth(path); size > width {
		return ansi.TruncateLeft(path, size-width+1, "…")
	}
	return path
}

func (m sidebar) groupHeading(id int, width int) []string {
	g := m.group(id)
	if g == nil {
		return nil
	}
	name := ansi.Truncate(oneLine(g.View.Name), width, "…")
	lines := []string{accent.Render(name)}
	for _, other := range m.groups {
		if other.View.ID != id && other.View.Name == g.View.Name {
			lines = append(lines, dim.Render(pathTail(g.View.Path, width)))
			break
		}
	}
	return lines
}

func (m sidebar) sessionLine(t *Task, selected bool, width int) string {
	live := len(m.taskSnapshot(t).task(t.ID).live())
	dot := dim.Render("○")
	if live > 0 {
		dot = green.Render("●")
	}
	prefix := "  "
	if t.Group == m.openGroup() && t.ID == m.task {
		prefix = "▸ "
	} else if selected {
		prefix = "› "
	}
	kind := ""
	if m.direct {
		kind = "⎇  "
		if t.plain() {
			kind = ">_ "
		}
	}
	space := max(1, width-4-ansi.StringWidth(kind))
	name := ansi.Truncate(m.displayName(t), space, "…")
	name += strings.Repeat(" ", max(0, space-ansi.StringWidth(name)))
	line := prefix + dot + " " + kind + name
	if selected {
		dotColor := dim.GetForeground()
		if live > 0 {
			dotColor = green.GetForeground()
		}
		return selectedStyle.Render(prefix) + selectedStyle.Foreground(dotColor).Render(ansi.Strip(dot)) + selectedStyle.Render(" "+kind+name)
	}
	return line
}

type shortcut struct{ key, label string }

func (m sidebar) shortcuts() []shortcut {
	items := []shortcut{}
	target := m.actionTarget()
	if target != nil && target.Folder {
		if g := m.group(target.Group); g != nil {
			if g.View.Type != "directory" {
				items = append(items, shortcut{"N", m.cfg.text("New worktree session", "Новая worktree-сессия")})
			} else {
				items = append(items, shortcut{"N", m.cfg.text("New session", "Новая сессия")})
			}
			items = append(items, shortcut{"T", m.cfg.text("New terminal here", "Новый терминал в папке")})
			if m.direct && g.Section > 0 && g.View.Type == "git" && g.Err == nil {
				items = append(items, shortcut{"R", m.cfg.text("Enable relative worktree paths", "Включить относительные пути worktree")})
			}
			if g.Section > 0 && g.Err == nil && g.State != nil && len(g.State.Tasks) == 0 && !g.hasPendingWorktree() {
				items = append(items, shortcut{"X", m.cfg.text("Remove section", "Убрать раздел")})
			}
			if g.State != nil {
				for _, t := range g.State.Tasks {
					if len(g.Snap.task(t.ID).live()) > 0 {
						items = append(items, shortcut{"S", m.cfg.text("Stop all sessions", "Остановить все сессии")})
						break
					}
				}
				if len(g.State.Tasks) > 0 {
					items = append(items, shortcut{"D", m.cfg.text("Delete all sessions", "Удалить все сессии")})
				}
			}
		}
	} else if job := m.job(target); job != nil {
		if job.Phase == "failed" {
			items = append(items, shortcut{"Enter", m.cfg.text("Retry creation", "Повторить создание")})
		}
	} else {
		if target != nil || !m.direct {
			items = append(items, shortcut{"N", m.cfg.text("New session", "Новая сессия")})
		}
		if target != nil {
			if t, err := m.findTask(target.Group, target.ID); err == nil {
				verb := m.cfg.text("Start", "Запустить")
				if len(m.taskSnapshot(t).task(t.ID).live()) > 0 {
					verb = m.cfg.text("Stop", "Остановить")
				}
				items = append(items, shortcut{"Enter", m.cfg.text("Open", "Открыть")}, shortcut{"T", m.cfg.text("Add terminal", "Ещё терминал")}, shortcut{"S", verb})
				if !t.Protected {
					items = append(items, shortcut{"D", m.cfg.text("Delete", "Удалить")})
				}
			}
		}
	}
	if m.menuAnchor == nil || m.modal != "actions" {
		if m.direct {
			items = append(items, shortcut{"+", m.cfg.text("Add section", "Добавить раздел")})
		}
		if m.searchAvailable() {
			items = append(items, shortcut{"/", m.cfg.text("Search", "Поиск")})
		}
		items = append(items, shortcut{"A", m.cfg.text("All / active", "Все / активные")}, shortcut{",", m.cfg.text("Settings", "Настройки")}, shortcut{"Q", m.cfg.text("Quit dashboard", "Выйти из dashboard")})
	}
	return items
}

func (m sidebar) noticeText() (string, bool) {
	message := m.message
	success := false
	if message == "" && m.state != nil {
		message, success = m.state.visibleNotice(time.Now())
	}
	if message == "" {
		for _, g := range m.groups {
			if g.Err != nil {
				message = g.View.Name + ": " + g.Err.Error()
				break
			}
			if g.State != nil {
				if notice, ok := g.State.visibleNotice(time.Now()); notice != "" {
					message = g.View.Name + ": " + notice
					success = ok
					break
				}
			}
		}
	}
	return localizeError(message, m.cfg.Language), success
}

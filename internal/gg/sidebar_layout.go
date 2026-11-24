package gg

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#73DACA")).Background(lipgloss.Color("#25343C"))
var panelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4CAD6")).Background(lipgloss.Color("#171B23"))
var ruleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#303745"))

func dashboardSurface(content string, width, height int) string {
	canvas := lipgloss.NewCanvas(max(1, width), max(1, height))
	canvas.Compose(lipgloss.NewLayer(content))
	for y := range canvas.Height() {
		for x := range canvas.Width() {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			if cell.Style.Fg == nil {
				cell.Style.Fg = panelStyle.GetForeground()
			}
			if cell.Style.Bg == nil {
				cell.Style.Bg = panelStyle.GetBackground()
			}
		}
	}
	return canvas.Render()
}

type textCanvas struct {
	width int
	lines []string
	hits  []hit
}

func (c *textCanvas) add(line string) {
	c.lines = append(c.lines, ansi.Truncate(line, max(1, c.width), "…"))
}

func (c *textCanvas) control(key, label string) {
	c.hits = append(c.hits, hit{x2: c.width, y: len(c.lines), key: key})
	c.add(label)
}

func (c *textCanvas) buttons(items ...shortcut) {
	line := ""
	for _, item := range items {
		label := "[" + item.key + "] " + item.label
		if item.key == "clear" {
			label = item.label
		}
		if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(label) > c.width {
			c.add(line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		x := ansi.StringWidth(line)
		label = ansi.Truncate(label, max(1, c.width-x), "…")
		c.hits = append(c.hits, hit{x1: x, x2: min(c.width, x+ansi.StringWidth(label)), y: len(c.lines), key: item.key})
		line += label
	}
	if line != "" {
		c.add(line)
	}
}

func (c *textCanvas) append(other textCanvas) {
	for _, h := range other.hits {
		h.y += len(c.lines)
		c.hits = append(c.hits, h)
	}
	c.lines = append(c.lines, other.lines...)
}

func (c textCanvas) finish(height int) (string, []hit) {
	height = max(1, height)
	lines := c.lines[:min(height, len(c.lines))]
	hits := []hit{}
	for _, h := range c.hits {
		if h.y >= 0 && h.y < len(lines) && h.x1 < c.width && h.x2 > h.x1 {
			h.x2 = min(h.x2, c.width)
			hits = append(hits, h)
		}
	}
	return strings.Join(lines, "\n"), hits
}

func (m sidebar) render() (string, []hit) {
	if m.modal != "" || m.confirming {
		return m.dialog(max(1, m.width-2), max(1, m.height))
	}
	return m.renderBase()
}

func (m sidebar) footer(width int) textCanvas {
	c := textCanvas{width: width}
	if m.searching {
		c.buttons(shortcut{"clear", m.cfg.text("Clear", "Очистить")}, shortcut{"↓", m.cfg.text("To list", "К списку")})
		return c
	}
	target := m.selectedTarget()
	if target != nil && target.Folder {
		if g := m.group(target.Group); g != nil {
			if g.View.Type != "directory" {
				c.buttons(shortcut{"N", m.cfg.text("New worktree", "Новая worktree")})
			} else {
				c.buttons(shortcut{"N", m.cfg.text("New session", "Новая сессия")})
			}
			c.buttons(shortcut{"T", m.cfg.text("Terminal", "Терминал")})
			buttons := []shortcut{}
			if g.State != nil {
				for _, t := range g.State.Tasks {
					if len(g.Snap.task(t.ID).live()) > 0 {
						buttons = append(buttons, shortcut{"S", m.cfg.text("Stop all", "Остановить все")})
						break
					}
				}
				if len(g.State.Tasks) > 0 {
					buttons = append(buttons, shortcut{"D", m.cfg.text("Delete all", "Удалить все")})
				}
			}
			c.buttons(buttons...)
		}
	} else if job := m.job(target); job != nil {
		if job.Phase == "failed" {
			c.buttons(shortcut{"Enter", m.cfg.text("Retry", "Повторить")})
		}
	} else if target != nil || !m.direct {
		c.buttons(shortcut{"Enter", m.cfg.text("Open", "Открыть")}, shortcut{"N", m.cfg.text("New", "Новая")})
		if t := m.selectedTask(); t != nil && !t.Protected {
			c.buttons(shortcut{"D", m.cfg.text("Delete", "Удалить")})
		}
	}
	c.buttons(shortcut{"?", m.cfg.text("Actions", "Действия")}, shortcut{"Q", m.cfg.text("Quit", "Выход")})
	return c
}

func (m sidebar) details(width int) []string {
	target := m.selectedTarget()
	if m.hover != nil {
		target = m.hover
	}
	if target == nil {
		return nil
	}
	lines := []string{ruleStyle.Render(strings.Repeat("─", width))}
	pair := func(label, value string) {
		label += ":"
		space := max(1, width-ansi.StringWidth(label)-ansi.StringWidth(value))
		lines = append(lines, dim.Render(label)+strings.Repeat(" ", space)+value)
	}
	if target.Folder {
		g := m.group(target.Group)
		if g == nil {
			return nil
		}
		lines = append(lines, dim.Render(pathTail(g.View.Path, width)))
		count, live := 0, 0
		if g.State != nil {
			count = len(g.State.Tasks)
			for _, t := range g.State.Tasks {
				if len(g.Snap.task(t.ID).live()) > 0 {
					live++
				}
			}
		}
		pair(m.cfg.text("Sessions", "Сессии"), strconv.Itoa(count))
		pair(m.cfg.text("Running", "Работают"), strconv.Itoa(live))
		kind := "Git"
		if g.View.Type == "directory" {
			kind = m.cfg.text("directory", "директория")
		}
		pair(m.cfg.text("Folder", "Папка"), kind)
	} else if job := m.job(target); job != nil {
		lines = append(lines, dim.Render(pathTail(job.Path, width)))
		pair(m.cfg.text("Status", "Состояние"), m.phaseLabel(job.Phase))
		if job.Phase == "failed" {
			for _, line := range strings.Split(ansi.Wrap(localizeError(oneLine(job.Error), m.cfg.Language), width, ""), "\n") {
				lines = append(lines, danger.Render(line))
			}
		}
	} else if t, err := m.findTask(target.Group, target.ID); err == nil {
		lines = append(lines, dim.Render(pathTail(t.Path, width)))
		pair(m.cfg.text("Terminals", "Терминалы"), strconv.Itoa(len(m.taskSnapshot(t).task(t.ID).live())))
		pair(m.cfg.text("Agent", "Агент"), m.p.statusLabel(m.agentStatus(t)))
		git := m.gitState(t)
		if git == "n/a" {
			git = "—"
		} else if m.cfg.Language == "ru" {
			git = map[string]string{"clean": "чисто", "modified": "изменено", "unknown": "неизвестно"}[git]
		}
		pair("Git", git)
		if !t.plain() {
			label := m.cfg.text("Branch", "Ветка")
			branch, known := m.gitSnapshot(t).Branches[strconv.Itoa(t.ID)]
			if !known {
				branch = m.cfg.text("unknown", "неизвестно")
			} else if branch == "" {
				branch = m.cfg.text("detached", "без ветки")
			}
			pair(label, ansi.Truncate(oneLine(branch), max(1, width-ansi.StringWidth(label)-2), "…"))
		}
		open := m.cfg.text("no", "нет")
		if target.Group == m.openGroup() && target.ID == m.task {
			open = m.cfg.text("yes", "да")
		}
		pair(m.cfg.text("Open", "Открыта"), open)
		if t.Protected {
			lines = append(lines, dim.Render(m.cfg.text("Protected source", "Защищённый источник")))
		} else if t.Kind == "temporary" {
			lines = append(lines, dim.Render(m.cfg.text("Temporary · ends with dashboard", "Временная · до выхода")))
		}
	}
	return lines
}

func (m sidebar) renderBase() (string, []hit) {
	width, height := max(1, m.width-2), m.height
	if height < 1 {
		height = 30
	}
	c := textCanvas{width: width}
	if m.direct && !m.searching {
		c.buttons(shortcut{"+", m.cfg.text("Add section", "Добавить раздел")})
	}
	if m.onlyActive {
		c.add(dim.Render(m.cfg.text("Active", "Активные")))
	}
	if m.searchAvailable() {
		if m.searching {
			c.control("/", m.search.View())
		} else if m.query != "" {
			c.control("/", "/ "+m.query)
		} else {
			c.buttons(shortcut{"/", m.cfg.text("Search", "Поиск")})
		}
	}
	footer := m.footer(width)
	message, success := m.noticeText()
	notices := 0
	if message != "" {
		notices = 1
	}
	details := []string{}
	if height >= 23 {
		details = m.details(width)
	}
	budget := height - len(c.lines) - len(details) - len(footer.lines) - notices
	if budget < 3 {
		details = nil
		budget = height - len(c.lines) - len(footer.lines) - notices
	}
	if budget < 2 {
		footer = textCanvas{width: width}
		footer.buttons(shortcut{"?", m.cfg.text("Actions", "Действия")}, shortcut{"Q", m.cfg.text("Quit", "Выход")})
		budget = height - len(c.lines) - len(footer.lines) - notices
	}
	list := m.listCanvas(width, max(1, budget))
	c.append(list)
	for len(c.lines) < height-len(details)-len(footer.lines)-notices {
		c.add("")
	}
	for _, line := range details {
		c.add(line)
	}
	c.append(footer)
	if message != "" {
		style := danger
		if success {
			style = green
		}
		c.add(style.Render(message))
	}
	return c.finish(height)
}

func (m sidebar) listCanvas(width, budget int) textCanvas {
	full := textCanvas{width: width}
	selected := m.selectedTarget()
	selectedLine, heading, rowCount := 0, 0, 0
	for _, item := range m.items() {
		same := selected != nil && *selected == item
		if item.Folder {
			if same || (selected != nil && selected.Group == item.Group) {
				heading = len(full.lines)
			}
			for i, line := range m.groupHeading(item.Group, width) {
				if same {
					line = selectedStyle.Width(width).Render(ansi.Truncate("› "+ansi.Strip(line), width, "…"))
				}
				full.hits = append(full.hits, hit{x2: width, y: len(full.lines), row: true, folder: true, group: item.Group})
				if same && i == 0 {
					selectedLine = len(full.lines)
				}
				full.add(line)
			}
			if g := m.group(item.Group); g != nil && g.Err != nil {
				full.add(danger.Render(m.cfg.text("! Project unavailable", "! Проект недоступен")))
			}
			continue
		}
		if same {
			selectedLine = len(full.lines)
		}
		var line string
		if job := m.job(&item); job != nil {
			line = m.worktreeJobLine(job, same, width)
		} else if t, err := m.findTask(item.Group, item.ID); err == nil {
			line = m.sessionLine(t, same, width)
		} else {
			continue
		}
		full.hits = append(full.hits, hit{x2: width, y: len(full.lines), row: true, group: item.Group, id: item.ID})
		full.add(line)
		rowCount++
	}
	if rowCount == 0 {
		full.add(dim.Render(m.cfg.text("No matching sessions", "Нет подходящих сессий")))
	}
	start := max(0, selectedLine-budget+1)
	visible := textCanvas{width: width}
	if start > heading && selected != nil && len(m.groups) > 0 && budget > 1 {
		for i, line := range m.groupHeading(selected.Group, width) {
			if i >= budget-1 {
				break
			}
			visible.hits = append(visible.hits, hit{x2: width, y: len(visible.lines), row: true, folder: true, group: selected.Group})
			visible.add(line)
		}
		start = max(start, selectedLine-(budget-len(visible.lines))+1)
	}
	end := min(len(full.lines), start+budget-len(visible.lines))
	offset := len(visible.lines)
	for _, h := range full.hits {
		if h.y >= start && h.y < end {
			h.y = h.y - start + offset
			visible.hits = append(visible.hits, h)
		}
	}
	visible.lines = append(visible.lines, full.lines[start:end]...)
	return visible
}

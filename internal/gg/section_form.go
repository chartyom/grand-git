package gg

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type sectionForm struct {
	path                    textinput.Model
	cwd, directory, message string
	entries                 []string
	selected, field         int
	cfg                     Settings
}

func newSectionForm(cwd string, cfg Settings) sectionForm {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 4096
	f := sectionForm{path: input, cwd: cwd, cfg: cfg}
	f.browse(cwd)
	return f
}

func expandSectionPath(value, cwd string) (string, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(value, "~"))
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}
	return canonicalDir(value)
}

func (f *sectionForm) browse(value string) {
	path, err := expandSectionPath(value, f.cwd)
	if err != nil {
		f.message = err.Error()
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		f.message = err.Error()
		return
	}
	names := []string{}
	for _, entry := range entries {
		info, err := os.Stat(filepath.Join(path, entry.Name()))
		if err == nil && info.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	f.entries = append([]string{".."}, names...)
	f.directory, f.message, f.selected = path, "", 0
	f.path.SetValue(path)
	f.path.CursorEnd()
}

func (f sectionForm) render(width, height int) (string, []hit) {
	c, footer := textCanvas{width: width}, textCanvas{width: width}
	c.add(accent.Render(f.cfg.text("Add section", "Добавить раздел")))
	f.path.SetWidth(max(1, width-2))
	c.control("path", f.path.View())
	focus := 1
	if f.message != "" {
		c.add(danger.Render(localizeError(f.message, f.cfg.Language)))
	}
	for i, name := range f.entries {
		line := "  " + oneLine(name) + "/"
		if f.field == 1 && f.selected == i {
			focus = len(c.lines)
			line = selectedStyle.Width(width).Render("› " + oneLine(name) + "/")
		}
		c.control("dir:"+strconv.Itoa(i), line)
	}
	add := f.cfg.text("Add", "Добавить")
	cancel := f.cfg.text("Cancel", "Отмена")
	if f.field == 2 {
		add = "› " + add
	}
	if f.field == 3 {
		cancel = "› " + cancel
	}
	footer.buttons(shortcut{"Ctrl+S", add}, shortcut{"Esc", cancel})
	return fitDialog(c, footer, height, focus)
}

func (m continuousDashboard) sectionFrame() dialogFrame {
	return framedDialog(max(1, m.width), max(1, m.height-1), 68, nil, m.folderForm.render)
}

func (m continuousDashboard) updateSectionForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.folderForm
	submit := false
	switch input := msg.(type) {
	case tea.MouseClickMsg:
		if input.Button != tea.MouseLeft {
			return m, nil
		}
		for _, h := range m.sectionFrame().hits {
			if !containsHit(h, input.X, input.Y) {
				continue
			}
			switch h.key {
			case "path":
				f.field = 0
				return m, f.path.Focus()
			case "Esc":
				m.folderForm = nil
				return m, nil
			case "Ctrl+S":
				submit = true
			default:
				if strings.HasPrefix(h.key, "dir:") {
					i, _ := strconv.Atoi(strings.TrimPrefix(h.key, "dir:"))
					if i >= 0 && i < len(f.entries) {
						f.browse(filepath.Join(f.directory, f.entries[i]))
						f.field = 1
						f.path.Blur()
					}
				}
			}
			break
		}
	case tea.MouseWheelMsg:
		f.field = 1
		f.path.Blur()
		step := 1
		if input.Button == tea.MouseWheelUp {
			step = -1
		}
		f.selected = max(0, min(len(f.entries)-1, f.selected+step))
	case tea.KeyPressMsg:
		switch input.String() {
		case "ctrl+w":
			return m, nil
		case "esc":
			m.folderForm = nil
			return m, nil
		case "ctrl+s":
			submit = true
		case "tab", "shift+tab":
			step := 1
			if input.String() == "shift+tab" {
				step = 3
			}
			f.field = (f.field + step) % 4
			f.path.Blur()
			if f.field == 0 {
				return m, f.path.Focus()
			}
		case "enter":
			switch f.field {
			case 0:
				f.browse(f.path.Value())
				if f.message == "" {
					f.field = 1
					f.path.Blur()
				}
			case 1:
				if len(f.entries) > 0 {
					f.browse(filepath.Join(f.directory, f.entries[f.selected]))
				}
			case 2:
				submit = true
			case 3:
				m.folderForm = nil
			}
		case "up", "down":
			if f.field == 0 {
				f.field = 1
				f.path.Blur()
			} else if f.field == 1 {
				step := 1
				if input.String() == "up" {
					step = -1
				}
				f.selected = max(0, min(len(f.entries)-1, f.selected+step))
			}
		case "backspace", "left":
			if f.field == 1 {
				f.browse(filepath.Dir(f.directory))
			} else if f.field == 0 {
				var cmd tea.Cmd
				f.path, cmd = f.path.Update(msg)
				return m, cmd
			}
		default:
			if f.field == 0 {
				var cmd tea.Cmd
				f.path, cmd = f.path.Update(msg)
				return m, cmd
			}
		}
	case tea.PasteMsg:
		if f.field == 0 {
			var cmd tea.Cmd
			f.path, cmd = f.path.Update(msg)
			return m, cmd
		}
	}
	if submit {
		path, err := expandSectionPath(f.path.Value(), f.cwd)
		if err != nil {
			f.message = err.Error()
			return m, nil
		}
		section, err := addSection(path)
		if err != nil {
			f.message = err.Error()
			return m, nil
		}
		m.folderForm = nil
		m.side.clearSearch()
		m.side.onlyActive = false
		m.side.cursorGroup, m.side.cursorID, m.side.cursorFolder = section.ID, 0, true
		m.side.message = ""
		m.listFocus, m.side.active = true, true
		return m, m.side.refresh()
	}
	return m, nil
}

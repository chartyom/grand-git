package gg

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func designFixture(t *testing.T) sidebar {
	t.Helper()
	m := groupedFixture(t)
	m.groups = m.groups[:2]
	m.direct = true
	m.task = 1
	m.groups[0].View.Type = "git"
	m.groups[0].State.Tasks = append(m.groups[0].State.Tasks,
		&Task{Group: 1, ID: 2, Kind: "temporary", RunToken: "other-invocation", Name: "launch", Path: "/fixture/1/child"},
		&Task{Group: 1, ID: 3, Kind: "terminal", Name: "plain", Path: "/fixture/1"})
	m.groups[0].Snap = Snapshot{{ID: "%1", Task: "1", Command: "sh"}, {ID: "%2", Task: "2", Command: "sh"}}
	m.screenWidth, m.screenHeight = 160, 41
	m.setState(m.state)
	return m
}

func rowHit(t *testing.T, m sidebar, target listTarget) hit {
	t.Helper()
	_, hits := m.renderBase()
	for _, h := range hits {
		if h.row && h.group == target.Group && h.id == target.ID && h.folder == target.Folder {
			return h
		}
	}
	t.Fatalf("target is not visible: %+v", target)
	return hit{}
}

func TestDesignSectionsFoldersAndHover(t *testing.T) {
	m := designFixture(t)
	view, _ := m.renderBase()
	plain := ansi.Strip(view)
	if strings.Contains(plain, "Worktrees") || strings.Contains(plain, "Terminals ─") || !strings.Contains(plain, "⎇") || !strings.Contains(plain, ">_") {
		t.Fatal(plain)
	}
	if len(m.candidates()) != 4 {
		t.Fatal("another invocation's temporary terminal was hidden")
	}
	before := *m.selectedTarget()
	h := rowHit(t, m, listTarget{Group: 2, Folder: true})
	next := m.mouseMotion(tea.MouseMotionMsg{X: h.x1, Y: h.y})
	if *next.selectedTarget() != before || next.task != m.task || next.active != m.active {
		t.Fatal("hover changed selection, focus or terminal")
	}
	if !strings.Contains(strings.Join(next.details(38), "\n"), "/fixture/2") {
		t.Fatal("folder preview missing")
	}
	next = next.mouseMotion(tea.MouseMotionMsg{X: 80, Y: 0})
	if next.hover != nil {
		t.Fatal("leaving the list retained preview")
	}
	m.move(-1)
	if target := m.selectedTarget(); target == nil || !target.Folder || target.Group != 1 {
		t.Fatal("folder is not keyboard selectable")
	}
	n, cmd := m.Update(commandKey("enter"))
	if cmd != nil || n.(sidebar).modal != "actions" {
		t.Fatal("folder Enter did not open actions")
	}
}

func TestSingleClickAndTargetBoundContextMenu(t *testing.T) {
	m := designFixture(t)
	h := rowHit(t, m, listTarget{Group: 1, ID: 3})
	n, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: h.x1, Y: h.y})
	if cmd == nil {
		t.Fatal("single click did not open")
	}
	a := cmd().(directActionMsg).action
	if a.Kind != "open" || a.Workspace != 1 || a.ID != 3 {
		t.Fatalf("wrong click action: %+v", a)
	}
	n, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: h.x1, Y: h.y})
	m = n.(sidebar)
	if cmd != nil || m.task != 1 || m.modal != "actions" || m.menuAnchor == nil {
		t.Fatal("right click opened the target")
	}
	for _, item := range m.shortcuts() {
		if item.key == "Q" || item.key == "A" || item.key == "," {
			t.Fatal("pointer menu contains unrelated global commands")
		}
	}
	m.selectTarget(listTarget{Group: 2, ID: 1})
	n, _ = m.Update(key('d'))
	m = n.(sidebar)
	if !m.confirming || m.confirmGroup != 1 || m.confirmID != 3 {
		t.Fatal("menu followed selection instead of its fixed target")
	}
}

func TestFolderBulkConfirmationFreezesTargetsAndPlainTerminalAction(t *testing.T) {
	m := designFixture(t)
	m.selectTarget(listTarget{Group: 1, Folder: true})
	n, cmd := m.Update(key('t'))
	if n.(sidebar).confirming || cmd == nil {
		t.Fatal("folder terminal action missing")
	}
	a := cmd().(directActionMsg).action
	if a.Kind != "terminal" || a.Workspace != 1 || !a.Folder {
		t.Fatalf("wrong terminal target: %+v", a)
	}
	n, _ = m.Update(key('s'))
	if !reflect.DeepEqual(n.(sidebar).confirmIDs, []int{1, 2}) {
		t.Fatal("Stop all must include only initially running sessions")
	}
	n, _ = m.Update(key('d'))
	m = n.(sidebar)
	m.groups[0].State.Tasks = []*Task{{Group: 1, ID: 3, Kind: "terminal", Name: "plain"}, {Group: 1, ID: 99, Branch: "later"}}
	m.onlyActive, m.query = true, "later"
	n, cmd = m.Update(key('y'))
	if cmd == nil || n.(sidebar).confirming {
		t.Fatal("bulk confirmation did not dispatch")
	}
	a = cmd().(directActionMsg).action
	if !a.Bulk || a.Workspace != 1 || !reflect.DeepEqual(a.IDs, []int{1, 2, 3}) {
		t.Fatalf("confirmation target set changed: %+v", a)
	}
}

func TestSearchEditingFooterMouseAndOpenSelection(t *testing.T) {
	m := compactFixture(t, 11)
	m.direct = true
	m.move(-20)
	n, _ := m.Update(commandKey("up"))
	m = n.(sidebar)
	if !m.searching {
		t.Fatal("Up did not enter search")
	}
	view, hits := m.renderBase()
	if strings.Contains(view, "[Q]") || strings.Contains(view, "[N]") || !strings.Contains(view, "To list") {
		t.Fatal("search footer contains session actions")
	}
	m.query = "missing"
	m.search.SetValue(m.query)
	for _, h := range hits {
		if h.key == "clear" {
			n, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: h.x1, Y: h.y})
			m = n.(sidebar)
		}
	}
	if m.query != "" || !m.searching || !m.search.Focused() {
		t.Fatal("Clear must retain input focus")
	}
	m.query = "task-02"
	m.search.SetValue(m.query)
	n, _ = m.Update(commandKey("down"))
	m = n.(sidebar)
	if m.searching || m.query == "" {
		t.Fatal("Down did not apply query")
	}
	n, _ = m.Update(commandKey("esc"))
	m = n.(sidebar)
	if m.selectedTask().ID != m.task || m.query != "" || !m.active {
		t.Fatal("Escape did not restore the open terminal's selection")
	}
}

func TestDialogFramesAndVisibleControlsFit(t *testing.T) {
	for _, language := range []string{"en", "ru"} {
		for _, size := range [][2]int{{160, 41}, {100, 15}, {99, 23}, {120, 14}, {22, 8}} {
			m := designFixture(t)
			m.cfg.Language = language
			m.openActions(m.selectedTarget(), &screenPoint{size[0] - 1, size[1] - 1})
			for _, modal := range []string{"actions", "split", "settings"} {
				m.modal = modal
				frame := m.dialogOverlay(size[0], size[1])
				if frame.x < 0 || frame.y < 0 || frame.x+frame.width > size[0] || frame.y+frame.height > size[1] {
					t.Fatalf("dialog outside screen: %v %s %+v", size, modal, frame)
				}
				for _, h := range frame.hits {
					if h.x1 < frame.x || h.x2 > frame.x+frame.width || h.y < frame.y || h.y >= frame.y+frame.height {
						t.Fatal("hit target outside dialog")
					}
				}
			}
		}
	}
}

func TestPersistentGitTerminalRegistryValidation(t *testing.T) {
	p := &Project{Root: "/fixture/repo"}
	task := &Task{ID: 1, Kind: "terminal", Path: p.Root, Name: "repo", Template: "shell"}
	if !p.validRecord(task) {
		t.Fatal("Git folder terminal rejected")
	}
	for _, path := range []string{"/fixture/other", "/fixture/repo/subdir", "relative"} {
		task.Path = path
		if p.validRecord(task) {
			t.Fatalf("terminal accepted outside its group root: %s", path)
		}
	}
}

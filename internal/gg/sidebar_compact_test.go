package gg

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func compactFixture(t *testing.T, count int) sidebar {
	t.Helper()
	m := newSidebar(&Project{Dir: t.TempDir(), WorkspaceID: 1}, 1)
	m.width, m.height, m.active, m.popup = 30, 24, true, true
	m.state = &State{Tasks: []*Task{}}
	for i := 1; i <= count; i++ {
		m.state.Tasks = append(m.state.Tasks, &Task{ID: i, Group: 1, Branch: fmt.Sprintf("task-%02d", i), Path: fmt.Sprintf("/project/task-%02d", i)})
		m.snap = append(m.snap, Pane{ID: fmt.Sprintf("%%%d", i), Task: fmt.Sprint(i), Command: "sh"})
	}
	m.setState(m.state)
	return m
}

func TestSearchThresholdUsesCandidatesAndResetsOnShrink(t *testing.T) {
	for _, count := range []int{0, 10, 11} {
		m := compactFixture(t, count)
		next, _ := m.Update(key('/'))
		m = next.(sidebar)
		if m.searching != (count > 10) {
			t.Fatalf("count=%d searching=%v", count, m.searching)
		}
		if count <= 10 && strings.Contains(m.View().Content, "[/]") {
			t.Fatal("search hint visible below threshold")
		}
	}
	m := compactFixture(t, 11)
	for _, c := range "/missing" {
		next, _ := m.Update(tea.KeyPressMsg{Code: c, Text: string(c)})
		m = next.(sidebar)
	}
	if !m.searching || len(m.rows()) != 0 || !m.searchAvailable() {
		t.Fatal("search disabled by its own results")
	}
	m.state.Tasks = m.state.Tasks[:10]
	next, _ := m.Update(refreshMsg{state: m.state, snap: m.snap, cfg: m.cfg, started: time.Now().Add(time.Second)})
	m = next.(sidebar)
	if m.searching || m.query != "" || m.search.Value() != "" || !m.active || len(m.rows()) != 10 {
		t.Fatalf("shrink: searching=%v query=%q value=%q active=%v rows=%d popup=%v", m.searching, m.query, m.search.Value(), m.active, len(m.rows()), m.popup)
	}
	m.state.Tasks = append(m.state.Tasks, &Task{ID: 12, Group: 1, Branch: "added"})
	m.setState(m.state)
	next, _ = m.Update(key('/'))
	if !next.(sidebar).searching {
		t.Fatal("search did not return at eleven rows")
	}
}

func TestSidebarUsesReadableSessionDetailsWithoutHeaderOrRowCodes(t *testing.T) {
	m := compactFixture(t, 2)
	m.width = 42
	m.git = map[string]string{"1": "modified"}
	view, _ := m.render()
	view = regexp.MustCompile(` +`).ReplaceAllString(ansi.Strip(view), " ")
	for _, old := range []string{"GRAND GIT", "All sessions", "1T", " ‹", " ↻"} {
		if strings.Contains(view, old) {
			t.Fatalf("obsolete sidebar text %q in %q", old, view)
		}
	}
	for _, want := range []string{"▸ ●", "Terminals: 1", "Agent: shell", "Git: modified", "Open: yes"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %q", want, view)
		}
	}
	m.onlyActive = true
	view, _ = m.render()
	view = regexp.MustCompile(` +`).ReplaceAllString(ansi.Strip(view), " ")
	if !strings.HasPrefix(view, "Active\n") || strings.Contains(view, "Active sessions") {
		t.Fatalf("activity filter heading: %q", view)
	}
	m.onlyActive = false
	m.cfg.Language = "ru"
	view, _ = m.render()
	view = regexp.MustCompile(` +`).ReplaceAllString(ansi.Strip(view), " ")
	for _, want := range []string{"Терминалы: 1", "Агент: ", "Git: изменено", "Открыта: да"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %q", want, view)
		}
	}
}

func TestActivityFilterDisablesAppliedSearch(t *testing.T) {
	m := compactFixture(t, 11)
	m.snap = m.snap[:10]
	m.query = "missing"
	m.search.SetValue(m.query)
	next, _ := m.Update(key('a'))
	m = next.(sidebar)
	if !m.onlyActive || m.searchAvailable() || m.query != "" || len(m.rows()) != 10 {
		t.Fatal("activity filter did not control the search threshold")
	}
}

func TestSearchCountsRowsAcrossGroupsNotHeadings(t *testing.T) {
	m := compactFixture(t, 6)
	state := &State{Tasks: []*Task{}}
	for i := 1; i <= 5; i++ {
		state.Tasks = append(state.Tasks, &Task{ID: i, Group: 2, Branch: "foreign"})
	}
	m.groups = []sessionGroup{
		{Project: m.p, State: m.state, Snap: m.snap, View: WorkspaceView{ID: 1, Name: "one"}},
		{Project: m.p, State: state, View: WorkspaceView{ID: 2, Name: "two"}},
	}
	if !m.searchAvailable() {
		t.Fatal("global rows not counted")
	}
	state.Tasks = state.Tasks[:4]
	if m.searchAvailable() {
		t.Fatal("headings counted as sessions")
	}
}

func TestSelectionSurvivesRefreshAndUsesNearestRemainingRow(t *testing.T) {
	m := compactFixture(t, 4)
	m.move(2)
	m.setState(m.state)
	if m.selectedTask().ID != 3 {
		t.Fatal("refresh moved selection")
	}
	m.state.Tasks = append(m.state.Tasks[:2], m.state.Tasks[3:]...)
	m.setState(m.state)
	if m.selectedTask().ID != 4 {
		t.Fatal("removal jumped to first row")
	}
}

func TestListFocusRevealsAndSelectsOpenTerminal(t *testing.T) {
	m := compactFixture(t, 2)
	m.task = 1
	m.active = false
	m.query = "task-02"
	m.onlyActive = true
	m.snap = Snapshot{{ID: "%side", Task: "1", Sidebar: true, Active: true, Session: "dashboard"}}
	m.pane = "%side"
	m.cursorID = 2
	m.setState(m.state)
	next, _ := m.Update(refreshMsg{state: m.state, snap: m.snap, cfg: m.cfg, started: time.Now().Add(time.Second)})
	m = next.(sidebar)
	if m.query != "" || m.onlyActive || m.selectedTask() == nil || m.selectedTask().ID != 1 {
		t.Fatalf("open terminal was not revealed and selected: query=%q active=%v selected=%v", m.query, m.onlyActive, m.selectedTask())
	}
}

func TestCompactLayoutAndActionsFitAndKeepMouseTargetsVisible(t *testing.T) {
	for _, language := range []string{"en", "ru"} {
		for _, size := range [][2]int{{28, 14}, {30, 24}, {56, 34}, {70, 20}, {22, 8}} {
			m := compactFixture(t, 12)
			m.width, m.height, m.cfg.Language = size[0], size[1], language
			m.move(11)
			for _, modal := range []string{"", "actions", "split", "settings"} {
				m.modal = modal
				m.actionIndex = len(m.shortcuts()) - 1
				view, hits := m.render()
				lines := strings.Split(view, "\n")
				if len(lines) > m.height {
					t.Fatalf("%v %s overflow", size, modal)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > m.width-2 {
						t.Fatalf("%v %s wide line: %q", size, modal, line)
					}
				}
				for _, h := range hits {
					if h.y >= len(lines) || h.x1 >= m.width-2 || h.x2 > m.width-2 {
						t.Fatal("hidden hit target")
					}
				}
				if modal == "" && !strings.Contains(view, "task-12") {
					t.Fatalf("selected row hidden: %s", view)
				}
				if modal == "actions" && !strings.Contains(view, "[Q]") {
					t.Fatal("small action menu cannot scroll to quit")
				}
			}
		}
	}
}

func TestActionMenuKeyboardMouseAndEscape(t *testing.T) {
	m := compactFixture(t, 1)
	next, _ := m.Update(key('?'))
	m = next.(sidebar)
	if m.modal != "actions" || !strings.Contains(m.View().Content, "[D]") || strings.Contains(m.View().Content, "[/]") {
		t.Fatal("wrong contextual actions")
	}
	m.query = "task"
	next, _ = m.Update(key(tea.KeyEscape))
	if next.(sidebar).modal != "" || next.(sidebar).query != "task" {
		t.Fatal("closing actions cleared underlying query")
	}
	m.query = ""
	_, hits := m.render()
	for _, h := range hits {
		if h.key == "T" {
			next, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: h.x1, Y: h.y})
			if next.(sidebar).modal != "split" {
				t.Fatal("action click did not open placement")
			}
			return
		}
	}
	t.Fatal("terminal action has no hit target")
}

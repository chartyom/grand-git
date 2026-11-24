package gg

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestWorktreeJobRowsAndActions(t *testing.T) {
	m := designFixture(t)
	g := &m.groups[0]
	g.Workspace, g.Section = 7, 1
	g.Jobs = []worktreeJob{{ID: 4, SectionID: 1, Name: "dependencies", Branch: "feat/branch", Phase: "copying", Path: "/fixture/dependencies"}}
	target := listTarget{Group: 1, ID: 4}
	m.selectTarget(target)
	m.search.SetValue("")
	known := func(group []shortcut, key string) bool {
		return slices.ContainsFunc(group, func(s shortcut) bool { return s.key == key })
	}
	for _, locale := range []struct{ lang, phase string }{{"en", "Copying"}, {"ru", "Копирование"}} {
		m.cfg.Language = locale.lang
		first, _ := m.renderBase()
		m.spinnerFrame++
		next, _ := m.renderBase()
		for _, view := range []string{first, next} {
			plain := ansi.Strip(view)
			if !strings.Contains(plain, locale.phase) || !strings.Contains(plain, "dependencies") {
				t.Fatalf("missing progress in %s: %s", locale.lang, plain)
			}
		}
		if ansi.Strip(first) == ansi.Strip(next) {
			t.Fatal("colorless frames should visibly change")
		}
		d := continuousDashboard{side: m, width: 66, height: 16, project: m.p, task: -1}
		bar := ansi.Strip(d.statusLine(d.width))
		if !strings.Contains(bar, locale.phase) || !strings.Contains(bar, "2/4") || ansi.StringWidth(bar) != d.width {
			t.Fatalf("compact status or committed session counts changed: %q", bar)
		}
	}
	for _, key := range []string{"enter", "n", "t", "s", "d"} {
		if _, cmd := m.dispatch(key, &target); cmd != nil || known(m.shortcuts(), strings.ToUpper(key)) {
			t.Errorf("pending row offered %s", key)
		}
	}
	if known(m.shortcuts(), "Enter") {
		t.Fatal("pending row offered Open")
	}
	h := rowHit(t, m, target)
	for _, button := range []tea.MouseButton{tea.MouseLeft, tea.MouseRight} {
		next, cmd := m.mouseClick(tea.MouseClickMsg{X: h.x1, Y: h.y, Button: button})
		if cmd != nil || next.(sidebar).pendingAction != nil || next.(sidebar).modal != "" {
			t.Fatal("mouse tried to open an unready worktree")
		}
	}
	m.beginConfirmation("delete", listTarget{Group: 1, Folder: true})
	if slices.Contains(m.confirmIDs, 4) {
		t.Fatal("bulk delete captured pending worktree")
	}
	g.State.Tasks = nil
	m.confirming = false
	m.selectTarget(listTarget{Group: 1, Folder: true})
	if known(m.shortcuts(), "X") {
		t.Fatal("reserved section offered removal")
	}
	g.Jobs[0].Phase, g.Jobs[0].Error = "failed", "cannot copy environment"
	if !known(m.shortcuts(), "X") {
		t.Fatal("an otherwise empty section with an error can be removed")
	}
	m.selectTarget(target)
	if !known(m.shortcuts(), "Enter") {
		t.Fatal("failed row has no retry action")
	}
	next, cmd := m.dispatch("enter", &target)
	if a := next.(sidebar).pendingAction; cmd == nil || a == nil || a.Kind != "retry-worktree" || a.Workspace != 7 || a.Section != 1 || a.ID != 4 {
		t.Fatalf("retry bound to wrong job: %+v", a)
	}
}

func TestWorktreeJobFilteringAndReadySelection(t *testing.T) {
	side := designFixture(t)
	group := &side.groups[0]
	job := worktreeJob{ID: 4, SectionID: 1, Name: "with-modules", Branch: "install-packages", Phase: "preparing"}
	group.Jobs = []worktreeJob{job}
	target := listTarget{Group: 1, ID: 4}
	for _, query := range []string{"WITH-MODULES", "install-packages", group.View.Name} {
		side.query = query
		if !slices.Contains(side.items(), target) {
			t.Fatalf("job missing for query %q", query)
		}
	}
	side.query = ""
	side.onlyActive = true
	if slices.Contains(side.items(), target) {
		t.Fatal("activity filter included an unready worktree")
	}
	side.onlyActive = false
	side.selectTarget(target)
	d := continuousDashboard{side: side, project: side.p, listFocus: true, openWhenReady: &target}
	// Leaving and returning to the row must not rearm automatic opening.
	next, _ := d.Update(commandKey("up"))
	d = next.(continuousDashboard)
	if d.openWhenReady != nil {
		t.Fatal("moving the selection left an automatic open armed")
	}
	next, _ = d.Update(commandKey("down"))
	d = next.(continuousDashboard)
	if d.openWhenReady != nil || *d.side.selectedTarget() != target {
		t.Fatal("returning armed the completed operation")
	}
	group.Jobs = nil
	group.State.Tasks = append(group.State.Tasks, &Task{ID: 4, Group: 1, Branch: job.Name})
	d.side.setState(d.side.state)
	if *d.side.selectedTarget() != target || d.side.selectedTask().label() != job.Name {
		t.Fatal("completion lost its position or identity")
	}
}

func TestWorktreeReservationSurvivesOlderRefresh(t *testing.T) {
	side := designFixture(t)
	d := continuousDashboard{side: side, project: side.p, listFocus: true, form: &createForm{}}
	old := refreshMsg{started: time.Now(), project: side.p, groups: slices.Clone(side.groups)}
	job := worktreeJob{ID: 5, SectionID: 1, Name: "with-modules", Phase: "preparing"}
	model, _ := d.Update(worktreeQueuedMsg{job: &job})
	d = model.(continuousDashboard)
	model, _ = d.Update(old)
	d = model.(continuousDashboard)
	target := listTarget{Group: 1, ID: 5}
	if d.form != nil || d.side.job(&target) == nil || d.openWhenReady == nil || *d.side.selectedTarget() != target {
		t.Fatal("an older refresh removed or deselected the queued job")
	}
}

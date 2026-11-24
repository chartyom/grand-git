package gg

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestEscapeClearsSearchAndCtrlWMovesFocus(t *testing.T) {
	for _, editing := range []bool{false, true} {
		for _, popup := range []bool{false, true} {
			m := sidebar{p: &Project{Dir: t.TempDir()}, active: true, popup: popup,
				query: "missing", searching: editing, onlyActive: true, search: textinput.New(),
				state: &State{Tasks: []*Task{{ID: 1, Branch: "alpha"}, {ID: 2, Branch: "beta"}}},
				snap:  Snapshot{{Task: "1", ID: "%1"}}}
			m.search.SetValue("missing")
			next, cmd := m.Update(key(tea.KeyEscape))
			got := next.(sidebar)
			if cmd != nil || got.query != "" || got.searching || got.search.Value() != "" || !got.active || !got.onlyActive {
				t.Fatalf("editing=%v popup=%v: Escape did not clear only search", editing, popup)
			}
			if rows := got.rows(); len(rows) != 1 || rows[0].Branch != "alpha" {
				t.Fatal("active filter not preserved")
			}
			_, cmd = got.Update(key(tea.KeyEscape))
			if popup && cmd == nil {
				t.Fatal("second Escape should close popup")
			}
			if !popup && cmd != nil {
				t.Fatal("second Escape should leave list focus unchanged")
			}
			if popup {
				_, cmd = got.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
				if cmd == nil {
					t.Fatal("Ctrl+W should close popup")
				}
			}
		}
	}
}

func TestCtrlWClosesCompactPopupDuringInteraction(t *testing.T) {
	for _, mode := range []string{"search", "confirm", "actions", "settings"} {
		m := sidebar{p: &Project{Dir: t.TempDir()}, popup: true, active: true, search: textinput.New()}
		switch mode {
		case "search":
			m.searching = true
		case "confirm":
			m.confirming = true
		case "actions", "settings":
			m.modal = mode
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatalf("Ctrl+W did not close compact popup during %s", mode)
		}
	}
}

func TestSearchDoesNotOverrideModalCancellation(t *testing.T) {
	m := sidebar{p: &Project{Dir: t.TempDir()}, active: true, query: "alpha", modal: "split"}
	next, _ := m.Update(key(tea.KeyEscape))
	got := next.(sidebar)
	if got.modal != "" || got.query != "alpha" {
		t.Fatal("search took precedence over modal")
	}
}

func TestSuccessNoticeExpiresWithoutRefreshExtension(t *testing.T) {
	now := time.Now()
	state := &State{Notice: "Completed: stop", NoticeKind: "success", NoticeUntil: now.Add(5 * time.Second)}
	if text, success := state.visibleNotice(now.Add(4 * time.Second)); text == "" || !success {
		t.Fatal("success hidden early")
	}
	if text, _ := state.visibleNotice(now.Add(5 * time.Second)); text != "" {
		t.Fatal("success did not expire")
	}
	state.Notice, state.NoticeKind = "cannot remove files", "error"
	if text, success := state.visibleNotice(now.Add(24 * time.Hour)); text == "" || success {
		t.Fatal("error expired or became success")
	}
	for _, message := range []string{"Completed: delete", "Выполнено: stop"} {
		old := &State{Notice: message}
		if text, _ := old.visibleNotice(now); text != "" {
			t.Fatal("legacy success was redisplayed")
		}
	}
	// A later success has its own deadline; no earlier timer clears it.
	state.Notice, state.NoticeKind, state.NoticeUntil = "Completed: delete", "success", now.Add(9*time.Second)
	if text, _ := state.visibleNotice(now.Add(5 * time.Second)); text == "" {
		t.Fatal("new notice cleared by old deadline")
	}
}

func TestTemporaryPresentationAndJSONPreserveBranchIdentity(t *testing.T) {
	dir := t.TempDir()
	if _, err := command(dir, "git", "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	p := &Project{Root: dir, Dir: dir}
	task := &Task{ID: 0, Path: dir, Branch: "main", Protected: true}
	m := sidebar{p: p, cfg: defaults(), width: 60, height: 30, state: &State{Tasks: []*Task{task}},
		snap: Snapshot{{ID: "%1", Task: "0", Generation: "launch"}}}
	if !strings.Contains(m.View().Content, "dashboard") {
		t.Fatal(m.View().Content)
	}
	rows, err := p.sessionViews(m.state, m.snap)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Branch != "main" || !rows[0].Temporary {
		t.Fatal(rows)
	}
	m.snap[0].Dead = true
	rows, err = p.sessionViews(m.state, m.snap)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Temporary {
		t.Fatal("dead terminal is temporary")
	}
}

func TestSessionListSkipsRecordsDeletedAfterItsSnapshot(t *testing.T) {
	p := &Project{Dir: t.TempDir()}
	removed := &Task{ID: 1, Branch: "clone1", Path: filepath.Join(p.Dir, "missing")}
	stale := &State{NextID: 2, Tasks: []*Task{removed}}
	if err := atomicJSON(filepath.Join(p.Dir, "state.json"), &State{NextID: 2}); err != nil {
		t.Fatal(err)
	}
	rows, err := p.sessionViews(stale, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("retired record in listing: %v, %v", rows, err)
	}
}

func TestDependencyVersions(t *testing.T) {
	for _, test := range []struct {
		value, minimum string
		want           bool
	}{
		{"tmux 3.4", "3.4", true}, {"tmux 3.5a", "3.4", true}, {"tmux next-3.6", "3.4", true},
		{"tmux 3.3a", "3.4", false}, {"git version 2.43.0", "2.31", true},
		{"git version 2.30.9", "2.31", false}, {"garbage", "3.4", false},
	} {
		if got := versionAtLeast(test.value, test.minimum); got != test.want {
			t.Errorf("%q: %v", test.value, got)
		}
	}
}
func TestRelativePathActionBindsGitProject(t *testing.T) {
	m := designFixture(t)
	g := &m.groups[0]
	g.Workspace, g.Section = 7, g.View.ID
	g.Project = &Project{Root: "/fixture/repo", WorkspaceID: g.Workspace}
	g.View.Path = "/fixture/repo/subdir"
	g.State = &State{}
	other := &m.groups[1]
	other.Section, other.View.Type = other.View.ID, "directory"

	// Open the Git section's pointer menu with a different project active.
	target := listTarget{Group: g.Section, Folder: true}
	h := rowHit(t, m, target)
	next, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseRight, X: h.x1, Y: h.y})
	m = next.(sidebar)
	stale := m
	stale.groups = m.groups[1:]
	closed, cmd := stale.Update(commandKey("enter"))
	if cmd != nil || closed.(sidebar).modal != "" || closed.(sidebar).confirming {
		t.Fatal("a removed section remained actionable")
	}
	m.selectTarget(listTarget{Group: other.Section, Folder: true})
	click := func(s sidebar, key string) (tea.Model, tea.Cmd) {
		for _, h := range s.dialogOverlay(s.screenWidth, s.screenHeight).hits {
			if h.key == key {
				return s.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: h.x1, Y: h.y})
			}
		}
		t.Fatalf("missing dialog control %q", key)
		return nil, nil
	}
	next, cmd = click(m, "R")
	m = next.(sidebar)
	if !m.confirming || cmd != nil {
		t.Fatal("an empty Git section must offer confirmation before changing paths")
	}
	for _, locale := range []string{"en", "ru"} {
		m.cfg.Language = locale
		body, _ := m.dialog(90, 24)
		if !strings.Contains(body, g.Project.Root) || !strings.Contains(body, "2.48") {
			t.Fatalf("missing project or compatibility requirement in %s: %s", locale, body)
		}
	}
	cancelled, cmd := click(m, "N/Esc")
	if cancelled.(sidebar).confirming || cmd != nil {
		t.Fatal("cancel dispatched the project action")
	}
	next, cmd = click(m, "Y")
	if cmd == nil || next.(sidebar).confirming {
		t.Fatal("confirm failed to dispatch the project action")
	}
	a := cmd().(directActionMsg).action
	if a.Kind != "relative-paths" || a.Workspace != g.Workspace || a.Section != g.Section {
		t.Fatalf("confirmation changed target: %+v", a)
	}
	for _, target := range []listTarget{{Group: other.Section, Folder: true}, {Group: other.Section, ID: 1}} {
		if !target.Folder {
			other.View.Type = "git"
		}
		m = next.(sidebar)
		m.selectTarget(target)
		m.openActions(&target, nil)
		for _, shortcut := range m.shortcuts() {
			if shortcut.key == "R" {
				t.Fatalf("unexpected project action on %+v", target)
			}
		}
	}
}

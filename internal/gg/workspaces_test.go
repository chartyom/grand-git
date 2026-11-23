package gg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConcurrentCatalogRegistrationAndSymlinkIdentity(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	roots := []string{canonicalTempDir(t), canonicalTempDir(t), canonicalTempDir(t)}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); p := &Project{Root: roots[i%3], Directory: true}; errs <- p.register() }(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := readCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Workspaces) != 3 || c.NextID != 4 {
		t.Fatalf("registrations lost or duplicated: %+v", c)
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err = os.Symlink(roots[0], link); err != nil {
		t.Fatal(err)
	}
	p, err := DiscoverWorkspace(link)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.register(); err != nil {
		t.Fatal(err)
	}
	c, err = readCatalog()
	if err != nil || len(c.Workspaces) != 3 {
		t.Fatalf("symlink duplicated group: %+v %v", c, err)
	}
}

func TestCorruptCatalogIsNotOverwritten(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.MkdirAll(userStateDir(), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(userStateDir(), "workspaces.json")
	for _, data := range []string{"invalid", `{"next_id":1,"workspaces":[{"id":1,"root":"relative","directory":true}]}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		p := &Project{Root: t.TempDir(), Directory: true}
		if err := p.register(); err == nil {
			t.Fatal("corrupt catalog accepted")
		}
		got, _ := os.ReadFile(path)
		if string(got) != data {
			t.Fatal("corrupt catalog overwritten")
		}
	}
}

func TestDamagedGitMarkerNeverFallsBackToDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /nonexistent/gg-fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := DiscoverWorkspace(root); err == nil {
		t.Fatalf("damaged repository became directory: %+v", p)
	}
}

func groupedFixture(t *testing.T) sidebar {
	t.Helper()
	p := &Project{Dir: t.TempDir(), WorkspaceID: 1}
	m := sidebar{p: p, active: true, width: 40, height: 24, cursorGroup: 1, cursorID: 1}
	for i := 1; i <= 8; i++ {
		state := &State{Tasks: []*Task{{ID: 1, Group: i, Branch: "same"}}}
		m.groups = append(m.groups, sessionGroup{Project: p, State: state, View: WorkspaceView{ID: i, Name: fmt.Sprintf("project-%d", i), Path: fmt.Sprintf("/fixture/%d", i)}})
	}
	m.state = m.groups[0].State
	m.setState(m.state)
	return m
}

func TestGroupedScrollingKeepsSelectedHeaderAndMouseTarget(t *testing.T) {
	m := groupedFixture(t)
	for i := 0; i < 14; i++ {
		m.move(1)
	}
	view, hits := m.render()
	if !strings.Contains(view, "project-8") || strings.Contains(view, "▸ ") {
		t.Fatal(view)
	}
	if m.cursorGroup != 8 || m.cursorID != 1 {
		t.Fatal("last session was not selected")
	}
	found := false
	for _, h := range hits {
		if h.row && h.group == 8 && h.id == 1 && h.y < m.height {
			found = true
		}
	}
	if !found {
		t.Fatal("selected mouse target missing")
	}
	if strings.Contains(view, "8:1") {
		t.Fatal("dashboard exposes public ID")
	}
}

func TestGroupHeadingsAndOpenMarker(t *testing.T) {
	m := groupedFixture(t)
	m.groups = m.groups[:2]
	m.groups[0].State.Tasks = append(m.groups[0].State.Tasks, &Task{ID: 2, Group: 1, Branch: "second"})
	m.task = 1
	m.setState(m.state)
	m.move(1)
	view, _ := m.render()
	plain := ansi.Strip(view)
	if strings.Contains(plain, "1/2") || strings.Contains(plain, "0/1") {
		t.Fatal("group count visible", plain)
	}
	if !strings.Contains(plain, "▸ ○ same") || !strings.Contains(plain, "› ○ second") {
		t.Fatal("open marker followed selection", plain)
	}
	if !strings.Contains(view, "48;2;37;52;60") {
		t.Fatal("selected text lost its style", view)
	}
}

func TestGroupedConfirmationNeverFollowsEqualIDInAnotherGroup(t *testing.T) {
	m := groupedFixture(t)
	m.move(2)
	next, _ := m.Update(key('d'))
	m = next.(sidebar)
	if m.confirmGroup != 2 || m.confirmID != 1 {
		t.Fatal("confirmation not bound")
	}
	m.groups[1].State.Tasks = nil
	m.cursorGroup = 1
	m.setState(m.state)
	next, cmd := m.Update(key('y'))
	if cmd != nil || next.(sidebar).confirming {
		t.Fatal("confirmation followed an equal ID in another project")
	}
}

func TestGroupedSearchAndActivityKeepGroupContext(t *testing.T) {
	m := groupedFixture(t)
	m.query = "project-3"
	if rows := m.rows(); len(rows) != 1 || rows[0].Group != 3 {
		t.Fatal("group search lost context")
	}
	m.onlyActive = true
	if len(m.rows()) != 0 {
		t.Fatal("stopped row passed active filter")
	}
	m.groups[2].Snap = Snapshot{{Task: "1", Command: "sh"}}
	if rows := m.rows(); len(rows) != 1 || rows[0].Group != 3 {
		t.Fatal("activity from wrong group")
	}
}

func TestDuplicateGroupNamesShowTheirPaths(t *testing.T) {
	m := groupedFixture(t)
	m.width = 80
	m.height = 40
	m.groups = m.groups[:2]
	m.groups[0].View.Name = "same-project"
	m.groups[1].View.Name = "same-project"
	view, _ := m.render()
	if !strings.Contains(view, "/fixture/1") || !strings.Contains(view, "/fixture/2") {
		t.Fatal(view)
	}
}

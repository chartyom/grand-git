package gg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFailedCreationAndCorruptSectionMetadataPreserveCatalog(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	p := &Project{Root: root, Directory: true}
	want := errors.New("fixture provisioning failure")
	_, err := p.withCreationSection(root, func(*Project) (*Task, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	c, err := readCatalog()
	if err != nil || len(c.Sections) != 0 {
		t.Fatalf("failed creation left a section: %+v %v", c, err)
	}
	path := filepath.Join(userStateDir(), "workspaces.json")
	bad := `{"next_id":1,"workspaces":[],"next_section_id":2,"sections":[{"id":1,"workspace":1,"path":"/missing"}]}`
	if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := addSection(root); err == nil {
		t.Fatal("corrupt metadata accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != bad {
		t.Fatal("corrupt catalog overwritten")
	}
}

func TestFolderBrowserPathsHiddenDirectoriesAndCancellation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	for _, name := range []string{".hidden", "with spaces"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	f := newSectionForm(root, defaults())
	if strings.Join(f.entries, "|") != "..|.hidden|with spaces" {
		t.Fatalf("entries: %v", f.entries)
	}
	f.browse("with spaces")
	if f.directory != filepath.Join(root, "with spaces") {
		t.Fatal("relative path not based on launch cwd")
	}
	f.browse(".hidden")
	if f.directory != filepath.Join(root, ".hidden") {
		t.Fatal("relative base changed after browsing")
	}
	f.path.SetValue("missing")
	f.browse(f.path.Value())
	if f.message == "" || f.path.Value() != "missing" {
		t.Fatal("invalid input was lost")
	}
	m := continuousDashboard{folderForm: &f}
	n, _ := m.updateSectionForm(commandKey("esc"))
	if n.(continuousDashboard).folderForm != nil {
		t.Fatal("cancel did not close form")
	}
	if _, err := os.Stat(userStateDir()); !os.IsNotExist(err) {
		t.Fatal("browsing created state")
	}
}

func TestSectionMigrationRemovalAndDuplicatePaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(userStateDir(), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(userStateDir(), "workspaces.json")
	if err := atomicJSON(path, map[string]any{"next_id": 8, "workspaces": []workspaceRecord{{ID: 7, Root: root, Directory: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := catalogTransaction(func(c *workspaceCatalog) error { return nil }); err != nil {
		t.Fatal(err)
	}
	c, err := readCatalog()
	if err != nil || len(c.Sections) != 1 || c.Workspaces[0].ID != 7 {
		t.Fatalf("migration: %+v %v", c, err)
	}
	id := c.Sections[0].ID
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	s, err := addSection(link)
	if err != nil || s.ID != id {
		t.Fatalf("duplicate: %+v %v", s, err)
	}
	if err := removeSection(id); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		groups, err := collectSections()
		if err != nil || len(groups) != 0 {
			t.Fatalf("removed section returned: %+v %v", groups, err)
		}
	}
	c, _ = readCatalog()
	if len(c.Workspaces) != 1 {
		t.Fatal("removal deleted workspace")
	}
	s, err = addSection(root)
	if err != nil || s.ID <= id {
		t.Fatalf("section ID reused: %+v %v", s, err)
	}
}

func TestConcurrentSectionsAndStoppedRecordProtection(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := addSection(root); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, _ := readCatalog()
	if len(c.Sections) != 1 || len(c.Workspaces) != 1 {
		t.Fatalf("duplicates: %+v", c)
	}
	s, p, err := sectionByID(c.Sections[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.withCreationSection(root, func(target *Project) (*Task, error) {
		task := &Task{ID: 1, SectionID: target.SectionID, Kind: "terminal", Name: "stopped", Path: root}
		return task, target.locked(func(state *State) error { state.Tasks = append(state.Tasks, task); state.NextID = 2; return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := removeSection(s.ID); err == nil {
		t.Fatal("removed section with a stopped record")
	}
	state, err := p.load()
	if err != nil || len(state.Tasks) != 1 || state.Tasks[0].SectionID != s.ID {
		t.Fatalf("record lost: %+v %v", state, err)
	}
}

func TestSectionActionsUseWorkspaceAndFixedSectionTargets(t *testing.T) {
	m := designFixture(t)
	m.p.WorkspaceID = 9
	m.groups[0].Workspace, m.groups[0].Section = 9, 1
	m.groups[1].Workspace, m.groups[1].Section = 9, 2
	m.groups[1].State.Tasks[0].ID = 8
	m.selectTarget(listTarget{Group: 2, Folder: true})
	n, cmd := m.Update(key('t'))
	if cmd == nil {
		t.Fatal("missing terminal action")
	}
	a := cmd().(directActionMsg).action
	if a.Workspace != 9 || a.Section != 2 {
		t.Fatalf("section confused with workspace: %+v", a)
	}
	m = n.(sidebar)
	m.pendingAction = nil
	n, _ = m.Update(key('d'))
	m = n.(sidebar)
	// A removed section ID must never be interpreted as another workspace ID.
	m.groups = m.groups[:1]
	n, cmd = m.Update(key('y'))
	if cmd == nil {
		t.Fatal("missing bulk action")
	}
	a = cmd().(directActionMsg).action
	if a.Workspace != 9 || a.Section != 2 || len(a.IDs) != 1 || a.IDs[0] != 8 {
		t.Fatalf("bulk escaped section: %+v", a)
	}
}

func TestMissingSectionCannotResolveCurrentWorkspaceRecord(t *testing.T) {
	m := designFixture(t)
	m.p.WorkspaceID = 1
	m.groups = m.groups[1:]
	if _, err := m.findTask(1, 1); err == nil {
		t.Fatal("missing section fell back to a different workspace record")
	}
}

func TestEmptyDashboardFocusAndDirectoryFormBounds(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Directory: true}
	m := continuousDashboard{project: p, task: -1, side: newSidebar(p, -1), listFocus: true, width: 80, height: 24}
	m.side.direct, m.side.active = true, true
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'w', Mod: tea.ModCtrl}} {
		n, _ := m.Update(key)
		m = n.(continuousDashboard)
		if !m.listFocus {
			t.Fatal("empty dashboard lost list focus")
		}
	}
	for _, language := range []string{"en", "ru"} {
		f := newSectionForm(p.Root, Settings{Language: language})
		f.entries = []string{"..", strings.Repeat("directory", 30), ".hidden"}
		f.field, f.selected = 1, 2
		m.folderForm = &f
		for _, size := range [][2]int{{160, 42}, {99, 24}, {100, 16}, {22, 8}} {
			m.width, m.height = size[0], size[1]
			frame := m.sectionFrame()
			if frame.x+frame.width > m.width || frame.y+frame.height >= m.height {
				t.Fatalf("form overflow: %v %+v", size, frame)
			}
			for _, h := range frame.hits {
				if h.x1 < frame.x || h.x2 > frame.x+frame.width || h.y >= frame.y+frame.height {
					t.Fatal("hidden form hit")
				}
			}
		}
	}
}

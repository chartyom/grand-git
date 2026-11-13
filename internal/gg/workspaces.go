package gg

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type WorkspaceView struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type workspaceRecord struct {
	ID        int    `json:"id"`
	Root      string `json:"root"`
	Common    string `json:"common,omitempty"`
	Directory bool   `json:"directory,omitempty"`
}
type workspaceCatalog struct {
	NextID          int               `json:"next_id"`
	Workspaces      []workspaceRecord `json:"workspaces"`
	SectionsVersion int               `json:"sections_version"`
	NextSectionID   int               `json:"next_section_id"`
	Sections        []sectionRecord   `json:"sections"`
}
type sessionGroup struct {
	Project   *Project
	State     *State
	Snap      Snapshot
	Git       map[string]string
	Err       error
	View      WorkspaceView
	Workspace int
	Section   int
}

func userStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(base) {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "grand-git")
}

func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}

// A failed Git discovery must not silently turn a damaged repository into a
// directory workspace. Plain terminals themselves do not require Git installed.
func DiscoverWorkspace(dir string) (*Project, error) {
	root, err := canonicalDir(dir)
	if err != nil {
		return nil, err
	}
	marker := false
	for path := root; ; path = filepath.Dir(path) {
		if _, e := os.Lstat(filepath.Join(path, ".git")); e == nil || !errors.Is(e, os.ErrNotExist) {
			marker = true
			break
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	if _, err := exec.LookPath("git"); err == nil {
		c := exec.Command("git", "rev-parse", "--git-common-dir")
		c.Dir = root
		c.Env = append(os.Environ(), "LC_ALL=C")
		out, probeErr := c.CombinedOutput()
		if probeErr == nil || marker {
			return Discover(root)
		}
		if !strings.Contains(string(out), "not a git repository") {
			return nil, fmt.Errorf("git: %s", strings.TrimSpace(string(out)))
		}
	} else if marker {
		return nil, checkDependency("git", "2.31")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(root))
	return &Project{Root: root, Directory: true, Socket: fmt.Sprintf("gg-dir-%x", h[:8]), Exe: exe}, nil
}

func readCatalog() (*workspaceCatalog, error) {
	c := &workspaceCatalog{NextID: 1, Workspaces: []workspaceRecord{}}
	b, err := os.ReadFile(filepath.Join(userStateDir(), "workspaces.json"))
	if errors.Is(err, os.ErrNotExist) {
		c.SectionsVersion, c.NextSectionID, c.Sections = 1, 1, []sectionRecord{}
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("invalid workspace catalog: %w", err)
	}
	if c.NextID < 1 {
		return nil, errors.New("invalid workspace catalog counter")
	}
	ids, keys := map[int]bool{}, map[string]bool{}
	for _, r := range c.Workspaces {
		key := r.key()
		if r.ID < 1 || r.ID >= c.NextID || ids[r.ID] || keys[key] || !filepath.IsAbs(r.Root) || filepath.Clean(r.Root) != r.Root ||
			(!r.Directory && (!filepath.IsAbs(r.Common) || filepath.Clean(r.Common) != r.Common)) || (r.Directory && r.Common != "") {
			return nil, errors.New("invalid workspace catalog record; file preserved")
		}
		ids[r.ID], keys[key] = true, true
	}
	if err := c.validateSections(ids); err != nil {
		return nil, err
	}
	return c, nil
}
func (r workspaceRecord) key() string {
	if r.Directory {
		return "directory:" + r.Root
	}
	return "git:" + r.Common
}
func (p *Project) register() error {
	return catalogTransaction(func(c *workspaceCatalog) error {
		p.registerIn(c)
		return nil
	})
}

func (p *Project) registerIn(c *workspaceCatalog) {
	r := workspaceRecord{Root: p.Root, Common: p.Common, Directory: p.Directory}
	for _, old := range c.Workspaces {
		if old.key() == r.key() {
			p.setWorkspaceID(old.ID)
			return
		}
	}
	r.ID = c.NextID
	c.NextID++
	c.Workspaces = append(c.Workspaces, r)
	p.setWorkspaceID(r.ID)
}
func (p *Project) setWorkspaceID(id int) {
	p.WorkspaceID = id
	if p.Directory {
		p.Dir = filepath.Join(userStateDir(), "directories", strconv.Itoa(id))
	}
}
func (p *Project) workspaceView() WorkspaceView {
	kind := "git"
	if p.Directory {
		kind = "directory"
	}
	return WorkspaceView{ID: p.WorkspaceID, Type: kind, Name: filepath.Base(p.Root), Path: p.Root}
}
func (r workspaceRecord) view() WorkspaceView {
	p := &Project{Root: r.Root, Directory: r.Directory, WorkspaceID: r.ID}
	return p.workspaceView()
}
func (r workspaceRecord) project() (*Project, error) {
	var p *Project
	var err error
	if r.Directory {
		// Retain the workspace's original kind even if the directory later becomes Git.
		root, e := canonicalDir(r.Root)
		if e != nil {
			return nil, e
		}
		if root != r.Root {
			return nil, errors.New("workspace directory was replaced")
		}
		exe, e := os.Executable()
		if e != nil {
			return nil, e
		}
		h := sha256.Sum256([]byte(root))
		p = &Project{Root: root, Directory: true, Exe: exe, Socket: fmt.Sprintf("gg-dir-%x", h[:8])}
	} else {
		p, err = Discover(r.Root)
		if err != nil {
			return nil, err
		}
		if p.Common != r.Common || p.Root != r.Root {
			return nil, errors.New("workspace repository was replaced")
		}
	}
	p.setWorkspaceID(r.ID)
	return p, nil
}
func workspaceByID(id int) (*Project, error) {
	c, err := readCatalog()
	if err != nil {
		return nil, err
	}
	for _, r := range c.Workspaces {
		if r.ID == id {
			return r.project()
		}
	}
	return nil, fmt.Errorf("workspace %d not found", id)
}
func collectGroups(git bool) ([]sessionGroup, error) {
	c, err := readCatalog()
	if err != nil {
		return nil, err
	}
	sort.Slice(c.Workspaces, func(i, j int) bool {
		a, b := c.Workspaces[i], c.Workspaces[j]
		x, y := strings.ToLower(filepath.Base(a.Root)), strings.ToLower(filepath.Base(b.Root))
		if x == y {
			return a.Root < b.Root
		}
		return x < y
	})
	groups := make([]sessionGroup, len(c.Workspaces))
	var wg sync.WaitGroup
	limit := make(chan struct{}, 4)
	for i, r := range c.Workspaces {
		wg.Add(1)
		go func(i int, r workspaceRecord) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			g := sessionGroup{View: r.view()}
			g.Project, g.Err = r.project()
			if g.Err == nil {
				g.State, g.Err = g.Project.currentState("")
			}
			if g.Err == nil {
				g.Snap = g.Project.snapshot()
				for _, t := range g.State.Tasks {
					t.Group = r.ID
				}
				if git {
					g.Git = g.Project.gitStates(g.State)
				}
			}
			groups[i] = g
		}(i, r)
	}
	wg.Wait()
	return groups, nil
}
func (p *Project) publicID(id int) string { return fmt.Sprintf("%d:%d", p.WorkspaceID, id) }
func resolveSession(arg string) (*Project, *Task, error) {
	var p *Project
	var err error
	var id int
	byID := strings.Contains(arg, ":")
	if byID {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			return nil, nil, errors.New("invalid session ID; expected group:session")
		}
		group, e := strconv.Atoi(parts[0])
		id, err = strconv.Atoi(parts[1])
		if e != nil || err != nil || group < 1 || id < 0 {
			return nil, nil, errors.New("invalid session ID; expected group:session")
		}
		p, err = workspaceByID(group)
	} else {
		p, err = DiscoverWorkspace(".")
	}
	if err != nil {
		return nil, nil, err
	}
	if p.Directory && !byID {
		return nil, nil, errors.New("use the session ID from gg list: group:session")
	}
	if err = p.register(); err != nil {
		return nil, nil, err
	}
	s, err := p.currentState("")
	if err != nil {
		return nil, nil, err
	}
	var t *Task
	if byID {
		t, err = taskByID(s, id)
	} else {
		t, err = taskByBranch(s, arg)
	}
	return p, t, err
}

func (t *Task) plain() bool { return t.Kind == "terminal" || t.Kind == "temporary" }
func (t *Task) kind() string {
	if t.Kind != "" {
		return t.Kind
	}
	if t.Protected {
		return "source"
	}
	return "worktree"
}
func (t *Task) label() string {
	if t.plain() {
		return t.Name
	}
	return t.Branch
}
func (p *Project) validRecord(t *Task) bool {
	if t.SectionID < 0 || !filepath.IsAbs(t.Path) || filepath.Clean(t.Path) != t.Path {
		return false
	}
	if t.plain() {
		return t.ID > 0 && t.Name != "" && t.Branch == "" && t.Base == "" && !t.Protected &&
			((t.Kind == "terminal" && (t.Path == p.Root || (!p.Directory && t.SectionID > 0)) && t.RunToken == "") ||
				(t.Kind == "temporary" && len(t.RunToken) == 32))
	}
	return !p.Directory && (t.Kind == "" || t.Kind == "source" || t.Kind == "worktree") && t.Branch != "" &&
		(t.Protected || p.validManagedPath(t)) && (t.ID != 0 || (t.Protected && t.Path == p.Root))
}
func (p *Project) createTerminal(cwd, name string, temporary bool, token string, template ...string) (*Task, error) {
	return p.withCreationSection(cwd, func(target *Project) (*Task, error) {
		return target.createTerminalRecord(cwd, name, temporary, token, template...)
	})
}

func (p *Project) createTerminalRecord(cwd, name string, temporary bool, token string, template ...string) (*Task, error) {
	if err := checkDependency("tmux", "3.4"); err != nil {
		return nil, err
	}
	path, err := canonicalDir(cwd)
	if err != nil {
		return nil, err
	}
	cfg, err := p.settings()
	if err != nil {
		return nil, err
	}
	tpl := cfg.DefaultTemplate
	if len(template) > 0 && template[0] != "" {
		tpl = template[0]
	}
	if temporary {
		tpl = "shell"
	}
	def, err := p.resolveTemplate(tpl)
	if err != nil {
		return nil, err
	}
	if err = validateTemplateShell(def); err != nil {
		return nil, err
	}
	if name == "" {
		name = filepath.Base(path)
	}
	var created *Task
	err = p.transaction(func(s *State) error {
		t := &Task{ID: s.NextID, SectionID: p.SectionID, Name: name, Path: path, Kind: "terminal", Template: tpl, CreatedAt: time.Now().UTC()}
		if temporary {
			t.Kind = "temporary"
			t.RunToken = token
		}
		if !p.validRecord(t) {
			return errors.New("invalid terminal session")
		}
		if err := p.ensureServer(); err != nil {
			return err
		}
		rollback := func() {
			_ = p.killTask(t.ID)
			if t.LaunchToken != "" {
				_ = os.Remove(p.launchPath(t.LaunchToken))
			}
		}
		if err := p.ensureTask(t); err != nil {
			rollback()
			return err
		}
		s.Tasks = append(s.Tasks, t)
		s.NextID++
		if err := atomicJSON(filepath.Join(p.Dir, "state.json"), s); err != nil {
			rollback()
			return err
		}
		created = t
		return nil
	}, false)
	return created, err
}

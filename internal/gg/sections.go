package gg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type sectionRecord struct {
	ID        int    `json:"id"`
	Workspace int    `json:"workspace"`
	Path      string `json:"path"`
}

func (c *workspaceCatalog) validateSections(workspaces map[int]bool) error {
	if c.SectionsVersion == 0 {
		if c.NextSectionID != 0 || len(c.Sections) != 0 {
			return errors.New("invalid section catalog version or counter; file preserved")
		}
		c.needsSectionMigration = true
		c.SectionsVersion, c.NextSectionID, c.Sections = 1, 1, []sectionRecord{}
		for _, w := range c.Workspaces {
			c.ensureSection(w.ID, w.Root)
		}
	}
	if c.SectionsVersion != 1 || c.NextSectionID < 1 {
		return errors.New("invalid section catalog version or counter; file preserved")
	}
	ids, paths := map[int]bool{}, map[string]bool{}
	for _, s := range c.Sections {
		key := fmt.Sprintf("%d:%s", s.Workspace, s.Path)
		if s.ID < 1 || s.ID >= c.NextSectionID || ids[s.ID] || paths[key] || !workspaces[s.Workspace] || !filepath.IsAbs(s.Path) || filepath.Clean(s.Path) != s.Path {
			return errors.New("invalid section catalog record; file preserved")
		}
		ids[s.ID], paths[key] = true, true
	}
	return nil
}

func (c *workspaceCatalog) ensureSection(workspace int, path string) sectionRecord {
	for _, s := range c.Sections {
		if s.Workspace == workspace && s.Path == path {
			return s
		}
	}
	s := sectionRecord{ID: c.NextSectionID, Workspace: workspace, Path: path}
	c.NextSectionID++
	c.Sections = append(c.Sections, s)
	return s
}

func (c *workspaceCatalog) section(id int) (sectionRecord, error) {
	for _, s := range c.Sections {
		if s.ID == id {
			return s, nil
		}
	}
	return sectionRecord{}, errors.New("section no longer exists")
}

func (c *workspaceCatalog) project(id int) (*Project, error) {
	for _, w := range c.Workspaces {
		if w.ID == id {
			return w.project()
		}
	}
	return nil, errors.New("workspace no longer exists")
}

func saveCatalog(c *workspaceCatalog) error {
	path := filepath.Join(userStateDir(), "workspaces.json")
	encoded, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	if err == nil && bytes.Equal(old, append(encoded, '\n')) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomicJSON(path, c)
}

// Operations touching both catalogs and registries acquire the catalog first.
func catalogTransaction(fn func(*workspaceCatalog) error) error {
	if err := os.MkdirAll(userStateDir(), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(userStateDir(), "workspaces.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	c, err := readCatalog()
	if err != nil {
		return err
	}
	if err := fn(c); err != nil {
		return err
	}
	return saveCatalog(c)
}

func addSection(path string) (sectionRecord, error) {
	path, err := canonicalDir(path)
	if err != nil {
		return sectionRecord{}, err
	}
	// Reading the directory also checks access before committing its section.
	if _, err := os.ReadDir(path); err != nil {
		return sectionRecord{}, err
	}
	p, err := DiscoverWorkspace(path)
	if err != nil {
		return sectionRecord{}, err
	}
	var section sectionRecord
	err = catalogTransaction(func(c *workspaceCatalog) error {
		p.registerIn(c)
		section = c.ensureSection(p.WorkspaceID, path)
		return nil
	})
	return section, err
}

func sectionByID(id int) (sectionRecord, *Project, error) {
	c, err := readCatalog()
	if err != nil {
		return sectionRecord{}, nil, err
	}
	s, err := c.section(id)
	if err != nil {
		return s, nil, err
	}
	p, err := c.project(s.Workspace)
	if err == nil {
		p.SectionID = s.ID
	}
	return s, p, err
}

func taskInSection(t *Task, s sectionRecord, root string) bool {
	return t.SectionID == s.ID || (t.SectionID == 0 && s.Path == root)
}

func removeSection(id int) error {
	return catalogTransaction(func(c *workspaceCatalog) error {
		s, err := c.section(id)
		if err != nil {
			return err
		}
		p, err := c.project(s.Workspace)
		if err != nil {
			return err
		}
		path, err := canonicalDir(s.Path)
		if err != nil || path != s.Path {
			return errors.New("section directory is unavailable or replaced")
		}
		return p.transaction(func(state *State) error {
			for _, task := range state.Tasks {
				if taskInSection(task, s, p.Root) {
					return errors.New("section is not empty")
				}
			}
			for i, old := range c.Sections {
				if old.ID == id {
					c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
					break
				}
			}
			return saveCatalog(c)
		}, false)
	})
}

func (p *Project) withCreationSection(cwd string, create func(*Project) (*Task, error)) (*Task, error) {
	path, err := canonicalDir(cwd)
	if err != nil {
		return nil, err
	}
	var task *Task
	err = catalogTransaction(func(c *workspaceCatalog) error {
		p.registerIn(c)
		var section sectionRecord
		previousNext := c.NextSectionID
		if p.SectionID > 0 {
			var err error
			section, err = c.section(p.SectionID)
			if err != nil {
				return err
			}
			if section.Workspace != p.WorkspaceID {
				return errors.New("section belongs to another workspace")
			}
		} else {
			section = c.ensureSection(p.WorkspaceID, path)
		}
		if err := saveCatalog(c); err != nil {
			return err
		}
		target := *p
		target.SectionID = section.ID
		var err error
		task, err = create(&target)
		if err != nil && section.ID == previousNext {
			for i, saved := range c.Sections {
				if saved.ID == section.ID {
					c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
					break
				}
			}
			if cleanupErr := saveCatalog(c); cleanupErr != nil {
				return errors.Join(err, cleanupErr)
			}
		}
		return err
	})
	return task, err
}

func collectSections(git bool) ([]sessionGroup, error) {
	c, err := readCatalog()
	if err != nil {
		return nil, err
	}
	groups := collectWorkspaceGroups(c.Workspaces, git)
	byWorkspace := make(map[int]sessionGroup, len(groups))
	rootSections := make(map[int]bool)
	for _, g := range groups {
		byWorkspace[g.View.ID] = g
	}
	for _, section := range c.Sections {
		if section.Path == byWorkspace[section.Workspace].View.Path {
			rootSections[section.Workspace] = true
		}
	}
	var missingRoots []*Project
	for _, g := range groups {
		if !rootSections[g.View.ID] && hasUnsectionedTasks(g.State) {
			missingRoots = append(missingRoots, g.Project)
		}
	}
	if c.needsSectionMigration || len(missingRoots) != 0 {
		// Old clients can write records with no section. Recheck under the
		// catalog lock so a concurrent removal of an empty root is respected.
		err = catalogTransaction(func(latest *workspaceCatalog) error {
			for _, p := range missingRoots {
				state, err := p.load()
				if err == nil && hasUnsectionedTasks(state) {
					latest.ensureSection(p.WorkspaceID, p.Root)
				}
			}
			c = latest
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := []sessionGroup{}
	for _, section := range c.Sections {
		g, ok := byWorkspace[section.Workspace]
		if !ok {
			continue
		}
		root := g.View.Path
		g.Workspace, g.Section = section.Workspace, section.ID
		g.View.ID, g.View.Name, g.View.Path = section.ID, filepath.Base(section.Path), section.Path
		if g.State != nil {
			state := *g.State
			state.Tasks = []*Task{}
			for _, t := range g.State.Tasks {
				if taskInSection(t, section, root) {
					copy := *t
					copy.Group = section.ID
					state.Tasks = append(state.Tasks, &copy)
				}
			}
			g.State = &state
		}
		if g.Err == nil {
			path, err := canonicalDir(section.Path)
			if err != nil || path != section.Path {
				g.Err = errors.New("section directory is unavailable or replaced")
			}
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].View.Name), strings.ToLower(out[j].View.Name)
		if a != b {
			return a < b
		}
		if out[i].View.Path != out[j].View.Path {
			return out[i].View.Path < out[j].View.Path
		}
		return out[i].View.ID < out[j].View.ID
	})
	return out, nil
}

func hasUnsectionedTasks(state *State) bool {
	if state != nil {
		for _, task := range state.Tasks {
			if task.SectionID == 0 {
				return true
			}
		}
	}
	return false
}

func (g sessionGroup) workspaceID() int {
	if g.Workspace > 0 {
		return g.Workspace
	}
	return g.View.ID
}

func (m sidebar) openGroup() int {
	for _, g := range m.groups {
		if g.workspaceID() == m.p.WorkspaceID && g.State != nil {
			for _, t := range g.State.Tasks {
				if t.ID == m.task {
					return g.View.ID
				}
			}
		}
	}
	return m.p.WorkspaceID
}

func (m sidebar) collectGroups(git bool) ([]sessionGroup, error) {
	if m.direct {
		return collectSections(git)
	}
	return collectGroups(git)
}

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
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type Project struct {
	Root, Common, Dir, Socket, Exe string
	WorkspaceID                    int
	SectionID                      int
	Directory                      bool
	legacyWorktreeList             bool
}

type Task struct {
	SectionID   int               `json:"section_id,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Name        string            `json:"name,omitempty"`
	RunToken    string            `json:"run_token,omitempty"`
	Group       int               `json:"-"`
	ID          int               `json:"id"`
	Branch      string            `json:"branch"`
	Base        string            `json:"base"`
	Path        string            `json:"path"`
	CreatedAt   time.Time         `json:"created_at"`
	Generated   map[string]string `json:"generated_files,omitempty"`
	Protected   bool              `json:"protected,omitempty"`
	CopySource  string            `json:"copy_source,omitempty"`
	CopiedFiles int               `json:"copied_files,omitempty"`
	Template    string            `json:"template,omitempty"`
	LaunchToken string            `json:"launch_token,omitempty"`
}

type State struct {
	NextID       int       `json:"next_id"`
	HooksEnabled bool      `json:"hooks_enabled"`
	Tasks        []*Task   `json:"tasks"`
	Notice       string    `json:"notice,omitempty"`
	NoticeKind   string    `json:"notice_kind,omitempty"`
	NoticeUntil  time.Time `json:"notice_until,omitempty"`
}

func command(dir, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s: %w", name, strings.TrimSpace(string(b)), err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func Discover(dir string) (*Project, error) {
	version, err := checkedDependencyVersion("git", "2.31")
	if err != nil {
		return nil, err
	}
	common, err := command(dir, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("run this command inside a Git repository: %w", err)
	}
	if bare, _ := command(dir, "git", "rev-parse", "--is-bare-repository"); bare == "true" {
		return nil, errors.New("bare repositories are not supported")
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return nil, err
	}
	legacy := !versionAtLeast(version, "2.36")
	root, err := mainWorktree(dir, common, legacy)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(common))
	return &Project{Root: root, Common: common, Dir: filepath.Join(common, "grand-git"), Socket: fmt.Sprintf("gg-%x", h[:8]), Exe: exe, legacyWorktreeList: legacy}, nil
}

func mainWorktree(dir, common string, legacy bool) (string, error) {
	if legacy {
		// Git before 2.36 lists the canonical common directory with a trailing
		// /.git removed. Its line-delimited output can contain unescaped newlines.
		if filepath.Base(common) == ".git" {
			return filepath.Dir(common), nil
		}
		return common, nil
	}
	out, err := command(dir, "git", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", err
	}
	first, _, _ := strings.Cut(out, "\x00")
	return filepath.EvalSymlinks(strings.TrimPrefix(first, "worktree "))
}

func (p *Project) load() (*State, error) {
	s := &State{NextID: 1, Tasks: []*Task{}}
	b, err := os.ReadFile(filepath.Join(p.Dir, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("corrupt session registry: %w", err)
	}
	if s.NextID < 1 {
		return nil, errors.New("invalid registry counter")
	}
	seen := map[int]bool{}
	for _, task := range s.Tasks {
		if task == nil || task.ID < 0 || task.ID >= s.NextID || seen[task.ID] || !p.validRecord(task) {
			return nil, errors.New("invalid session record; registry left unchanged")
		}
		seen[task.ID] = true
	}
	sort.Slice(s.Tasks, func(i, j int) bool { return s.Tasks[i].ID < s.Tasks[j].ID })
	return s, nil
}

func atomicJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".gg-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (p *Project) locked(fn func(*State) error) error {
	return p.transaction(fn, true)
}

func (p *Project) transaction(fn func(*State) error, persist bool) error {
	if err := os.MkdirAll(p.Dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	s, err := p.load()
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	if !persist {
		return nil
	}
	return atomicJSON(filepath.Join(p.Dir, "state.json"), s)
}

func taskByID(s *State, id int) (*Task, error) {
	for _, t := range s.Tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("session no longer exists")
}

func (p *Project) Create(cwd, branch, base string, skipCopy bool, template ...string) (*Task, error) {
	return p.withCreationSection(cwd, func(target *Project) (*Task, error) {
		return target.createTask(cwd, branch, base, skipCopy, template...)
	})
}

func (p *Project) createTask(cwd, branch, base string, skipCopy bool, template ...string) (*Task, error) {
	if p.Directory {
		if base != "" || skipCopy {
			return nil, errors.New("--base and --no-copy require a Git repository")
		}
		return p.createTerminalRecord(cwd, branch, false, "", template...)
	}
	if err := checkDependency("tmux", "3.4"); err != nil {
		return nil, err
	}
	cfg, err := p.settings()
	if err != nil {
		return nil, err
	}
	name := cfg.DefaultTemplate
	if len(template) > 0 && template[0] != "" {
		name = template[0]
	}
	tpl, err := p.resolveTemplate(name)
	if err != nil {
		return nil, err
	}
	if err = validateTemplateShell(tpl); err != nil {
		return nil, err
	}
	branch = strings.TrimSpace(branch)
	if normalized, err := command(cwd, "git", "check-ref-format", "--branch", branch); err != nil || normalized != branch || branch == "HEAD" {
		return nil, fmt.Errorf(p.text("invalid branch name %q", "недопустимое имя ветки %q"), branch)
	}
	var created *Task
	err = p.transaction(func(s *State) error {
		for _, t := range s.Tasks {
			if t.Branch == branch {
				return fmt.Errorf(p.text("session %q already exists; use start or dashboard", "сессия %q уже существует; используйте start или dashboard"), branch)
			}
		}
		_, branchErr := command(cwd, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		exists := branchErr == nil
		if exists && base != "" {
			return errors.New("--base cannot be used with an existing branch")
		}
		source := ""
		if !skipCopy {
			var err error
			source, err = p.copySource(cwd, base)
			if err != nil {
				return err
			}
		}
		if exists {
			base = "refs/heads/" + branch
		} else if base == "" {
			base = "HEAD"
		}
		baseCommit, err := command(cwd, "git", "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if err != nil {
			return fmt.Errorf("cannot resolve base commit: %w", err)
		}
		path := filepath.Join(p.Root+"__sessions", branch)
		if err := safeSessionParents(p.Root+"__sessions", path); err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf(p.text("session directory already exists: %s", "каталог сессии уже существует: %s"), path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		args := []string{"worktree", "add"}
		if !exists {
			args = append(args, "-b", branch)
		}
		args = append(args, "--", path)
		if exists {
			args = append(args, branch)
		} else {
			args = append(args, baseCommit)
		}
		t := &Task{ID: s.NextID, SectionID: p.SectionID, Branch: branch, Base: baseCommit, Path: path, CreatedAt: time.Now().UTC(), Template: name}
		rollback := func() {
			p.killTask(t.ID)
			if t.LaunchToken != "" {
				_ = os.Remove(p.launchPath(t.LaunchToken))
			}
			command(p.Root, "git", "worktree", "remove", "--force", "--", path)
			if !exists {
				command(p.Root, "git", "branch", "-D", "--", branch)
			}
		}
		if _, err := command(p.Root, "git", args...); err != nil {
			// Git may finish registration and checkout before a project hook
			// fails. Only clean up a worktree of the requested branch at the
			// previously absent destination, never an unrelated occupied path.
			actual, _ := command(path, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
			if actual == branch && p.validateWorktree(t) == nil {
				rollback()
			}
			return err
		}
		if !skipCopy {
			count, err := copyIgnored(source, path)
			if err != nil {
				rollback()
				return fmt.Errorf("cannot copy environment: %w", err)
			}
			t.CopySource, t.CopiedFiles = source, count
		}
		if s.HooksEnabled {
			if err := p.installHooks(path); err != nil {
				rollback()
				return err
			}
			t.Generated = hookHashes(path)
		}
		if err := p.ensureServer(); err != nil {
			rollback()
			return err
		}
		if err := p.ensureTask(t); err != nil {
			rollback()
			return err
		}
		s.Tasks = append(s.Tasks, t)
		s.NextID++
		// Persist before returning, so rollback also covers registry write failures.
		if err := atomicJSON(filepath.Join(p.Dir, "state.json"), s); err != nil {
			rollback()
			return err
		}
		created = t
		return nil
	}, false)
	return created, err
}

func (p *Project) Delete(id int, force bool) error {
	return p.operate("delete", []int{id}, force, false)
}

// Only untouched files generated by us are exempt from dirty-worktree protection.
func (p *Project) clean(t *Task) (bool, error) {
	out, err := command(t.Path, "git", "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return false, nil
		}
		file := entry[3:]
		if expected := t.Generated[file]; expected == "" || fileHash(filepath.Join(t.Path, file)) != expected {
			return false, nil
		}
	}
	return true, nil
}

func (p *Project) notice(message string) {
	p.setNotice(message, false)
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func (p *Project) invocation(args ...string) string {
	if p.WorkspaceID > 0 && len(args) > 0 && args[0] != "_hook" && args[0] != "_service" && args[0] != "_welcome" {
		args = append(append([]string(nil), args...), "--workspace", fmt.Sprint(p.WorkspaceID))
	}
	all := []string{quote(p.Exe)}
	for _, arg := range args {
		all = append(all, quote(arg))
	}
	return strings.Join(all, " ")
}

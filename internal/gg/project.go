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
	Branch      string            `json:"branch"` // Stable worktree name under its historical registry key.
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

func commandIn(dir, name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Dir = dir
	return c
}

func command(dir, name string, args ...string) (string, error) {
	b, err := commandIn(dir, name, args...).CombinedOutput()
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
	return p.withRegistryLock(unix.LOCK_EX, func(s *State) error {
		if err := fn(s); err != nil {
			return err
		}
		if persist {
			return atomicJSON(filepath.Join(p.Dir, "state.json"), s)
		}
		return nil
	})
}

// LOCK_NB skips optional maintenance when a writer is provisioning a session.
func (p *Project) withRegistryLock(flags int, fn func(*State) error) error {
	if err := os.MkdirAll(p.Dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(p.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), flags); err != nil {
		if flags&unix.LOCK_NB != 0 && errors.Is(err, unix.EWOULDBLOCK) {
			return nil
		}
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	s, err := p.load()
	if err != nil {
		return err
	}
	return fn(s)
}

func taskByID(s *State, id int) (*Task, error) {
	for _, t := range s.Tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("session no longer exists")
}

func (p *Project) Create(cwd, name, branch, base string, skipCopy bool, template ...string) (*Task, error) {
	if p.Directory {
		if branch != "" {
			return nil, errors.New(p.text("--branch requires a Git repository", "--branch доступен только в Git"))
		}
		if base != "" || skipCopy {
			return nil, errors.New("--base and --no-copy require a Git repository")
		}
		return p.createTerminal(cwd, name, false, "", template...)
	}
	tpl := ""
	if len(template) != 0 {
		tpl = template[0]
	}
	job, lease, err := p.reserveWorktree(cwd, name, branch, base, tpl, skipCopy, false, 0)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	return p.provisionWorktree(job, lease)
}

func worktreeDirName(name string) string {
	// Worktree names use Git's branch-name syntax, which reserves ~.
	if len(name) > 255 {
		return fmt.Sprintf("~%x", sha256.Sum256([]byte(name)))
	}
	return strings.ReplaceAll(name, "/", "~")
}

func validGitName(cwd, name string) bool {
	normalized, err := command(cwd, "git", "check-ref-format", "--branch", name)
	return err == nil && normalized == name && name != "HEAD"
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

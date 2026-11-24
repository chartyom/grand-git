package gg

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Jobs are separate from the committed registry. Older clients may rewrite
// state.json, and checkout hooks and lifecycle commands read only ready tasks.
type worktreeJob struct {
	ID           int               `json:"id"`
	SectionID    int               `json:"section_id"`
	Workspace    int               `json:"workspace"`
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Phase        string            `json:"phase"`
	CreatedAt    time.Time         `json:"created_at"`
	CWD          string            `json:"cwd"`
	Branch       string            `json:"branch,omitempty"`
	Base         string            `json:"base"`
	Source       string            `json:"source,omitempty"`
	Template     string            `json:"template"`
	AdminID      string            `json:"admin_id"`
	CreateBranch bool              `json:"create_branch,omitempty"`
	OwnsBranch   bool              `json:"owns_branch,omitempty"`
	DirectoryDev uint64            `json:"directory_device,omitempty"`
	DirectoryIno uint64            `json:"directory_inode,omitempty"`
	Generated    map[string]string `json:"generated_files,omitempty"`
	Dashboard    bool              `json:"dashboard,omitempty"`
	Error        string            `json:"error,omitempty"`
}

func (j worktreeJob) task() *Task {
	return &Task{ID: j.ID, SectionID: j.SectionID, Branch: j.Name, Base: j.Base, Path: j.Path, CreatedAt: j.CreatedAt, Template: j.Template, Generated: j.Generated}
}

func (p *Project) jobFile(id int, suffix string) string {
	return filepath.Join(p.Dir, "creations", strconv.Itoa(id)+suffix)
}

func (p *Project) saveJob(j *worktreeJob) error {
	return atomicJSON(p.jobFile(j.ID, ".json"), j)
}

func (p *Project) readJob(id int) (*worktreeJob, error) {
	b, err := os.ReadFile(p.jobFile(id, ".json"))
	if err != nil {
		return nil, err
	}
	var j worktreeJob
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	if id <= 0 || id != j.ID || j.Workspace <= 0 || (p.WorkspaceID > 0 && j.Workspace != p.WorkspaceID) ||
		j.Name == "" || j.Path != filepath.Join(p.Root+"__sessions", worktreeDirName(j.Name)) || !p.validManagedPath(j.task()) ||
		!filepath.IsAbs(j.CWD) || !strings.HasPrefix(j.AdminID, fmt.Sprintf("gg-%d-", id)) || filepath.Base(j.AdminID) != j.AdminID {
		return nil, errors.New("invalid worktree creation record")
	}
	switch j.Phase {
	case "preparing", "copying", "starting", "failed":
		return &j, nil
	}
	return nil, errors.New("invalid worktree creation phase")
}

func (p *Project) readJobs() ([]worktreeJob, error) {
	if p == nil || p.Directory || p.Dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(filepath.Join(p.Dir, "creations"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var jobs []worktreeJob
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok {
			continue
		}
		id, err := strconv.Atoi(name)
		if err != nil || strconv.Itoa(id) != name {
			return nil, errors.New("invalid worktree creation record")
		}
		j, err := p.readJob(id)
		if errors.Is(err, os.ErrNotExist) {
			continue // A concurrent completion removed the snapshot.
		}
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, nil
}

func (p *Project) worktreeJobs() ([]worktreeJob, error) {
	jobs, err := p.readJobs()
	if err != nil {
		return nil, err
	}
	visible := jobs[:0]
	for _, j := range jobs {
		if j.Phase != "failed" {
			if err := p.recoverAbandonedJob(j.ID); err != nil {
				return nil, err
			}
		}
		if j.Dashboard {
			visible = append(visible, j)
		}
	}
	return visible, nil
}

// The creator holds the exclusive lease continuously, including the handoff to
// a detached worker. Its automatic release on exit permits exactly one recovery.
func (p *Project) recoverAbandonedJob(id int) error {
	f, err := os.OpenFile(p.jobFile(id, ".lock"), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); errors.Is(err, unix.EWOULDBLOCK) {
		return nil
	} else if err != nil {
		return err
	}
	j, err := p.readJob(id)
	if errors.Is(err, os.ErrNotExist) || (err == nil && j.Phase == "failed") {
		return nil
	}
	if err != nil {
		return err
	}
	return p.startWorktreeWorker(j, f, true)
}

func (p *Project) removeJob(id int) error {
	for _, suffix := range []string{".json", ".lock"} {
		if err := os.Remove(p.jobFile(id, suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (p Project) queueWorktree(cwd, name, branch, template string, replaces ...int) (*worktreeJob, error) {
	replaced := 0
	if len(replaces) > 0 {
		replaced = replaces[0]
	}
	j, lease, err := p.reserveWorktree(cwd, name, branch, "", template, false, true, replaced)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	if err = p.startWorktreeWorker(j, lease, false); err != nil {
		// The reservation was accepted. Preserve this failure with its draft.
		if saveErr := p.failWorktreeJob(j, err); saveErr != nil {
			return nil, saveErr
		}
	}
	return j, nil
}

func (p *Project) startWorktreeWorker(j *worktreeJob, lease *os.File, recovery bool) error {
	args := []string{"_prepare-worktree", "--workspace", strconv.Itoa(j.Workspace), "--job", strconv.Itoa(j.ID)}
	if recovery {
		args = append(args, "--recover")
	}
	cmd := exec.Command(p.Exe, args...)
	cmd.Dir = p.Root
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{lease}
	var ready, signal *os.File
	if !recovery {
		var err error
		ready, signal, err = os.Pipe()
		if err != nil {
			return err
		}
		defer ready.Close()
		defer signal.Close()
		cmd.ExtraFiles = append(cmd.ExtraFiles, signal)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	if ready != nil {
		_ = signal.Close()
		var ack [1]byte
		if _, err := io.ReadFull(ready, ack[:]); err != nil {
			return fmt.Errorf("cannot start worktree worker: %w", err)
		}
	}
	return nil
}

// The descriptors are inherited only by gg service processes. Git and its
// hooks may keep background children running long after checkout finishes.
func (p *Project) withWorkerLease(id int, run func(*worktreeJob, *os.File) error) error {
	lease := os.NewFile(3, "creation lease")
	if lease == nil {
		return errors.New("missing worktree creation lease")
	}
	defer lease.Close()
	claimed, err := lease.Stat()
	if err != nil {
		return err
	}
	current, err := os.Stat(p.jobFile(id, ".lock"))
	if err != nil || !os.SameFile(current, claimed) {
		return errors.New("worktree creation lease no longer matches")
	}
	if err := unix.Flock(3, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	j, err := p.readJob(id)
	if err != nil || j.Phase == "failed" {
		return err
	}
	return run(j, lease)
}

func (p *Project) runWorktreeWorker(id int, recovery bool) error {
	return p.withWorkerLease(id, func(j *worktreeJob, lease *os.File) error {
		if recovery {
			return p.rollbackInterruptedJob(j)
		}
		signal := os.NewFile(4, "worker ready")
		if signal == nil {
			return errors.New("missing worktree worker signal")
		}
		_, _ = signal.Write([]byte{1}) // The dashboard can exit before acknowledgement.
		_ = signal.Close()
		_, err := p.provisionWorktree(j, lease)
		return err
	})
}

func (p *Project) reserveWorktree(cwd, name, branch, base, template string, skipCopy, dashboard bool, replaces int) (*worktreeJob, *os.File, error) {
	if p.Directory {
		return nil, nil, errors.New("--branch requires a Git repository")
	}
	if err := checkDependency("tmux", "3.4"); err != nil {
		return nil, nil, err
	}
	cwd, err := canonicalDir(cwd)
	if err != nil {
		return nil, nil, err
	}
	if template == "" {
		cfg, err := p.settings()
		if err != nil {
			return nil, nil, err
		}
		template = cfg.DefaultTemplate
	}
	tpl, err := p.resolveTemplate(template)
	if err != nil {
		return nil, nil, err
	}
	if err := validateTemplateShell(tpl); err != nil {
		return nil, nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = branch
	}
	if !validGitName(cwd, name) {
		return nil, nil, fmt.Errorf(p.text("invalid worktree name %q", "недопустимое имя worktree %q"), name)
	}
	if branch != "" && branch != name && !validGitName(cwd, branch) {
		return nil, nil, fmt.Errorf(p.text("invalid branch name %q", "недопустимое имя ветки %q"), branch)
	}

	j := &worktreeJob{Name: name, CWD: cwd, Branch: branch, Template: template, Phase: "preparing", Dashboard: dashboard,
		CreatedAt: time.Now().UTC(), Path: filepath.Join(p.Root+"__sessions", worktreeDirName(name))}
	var lease *os.File
	err = catalogTransaction(func(c *workspaceCatalog) error {
		p.registerIn(c)
		j.Workspace, j.SectionID = p.WorkspaceID, p.SectionID
		if p.SectionID > 0 {
			section, err := c.section(p.SectionID)
			if err != nil {
				return err
			}
			if section.Workspace != j.Workspace {
				return errors.New("section belongs to another workspace")
			}
		} else {
			// Publish an implicit CLI section only along with a successful task.
			for _, section := range c.Sections {
				if section.Workspace == j.Workspace && section.Path == cwd {
					j.SectionID = section.ID
					break
				}
			}
		}
		if err := saveCatalog(c); err != nil {
			return err
		}
		return p.transaction(func(s *State) error {
			if err := p.checkWorktreeSlot(s, j, replaces); err != nil {
				return err
			}
			if err := p.resolveWorktreeBase(j, base, skipCopy); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(p.Dir, "creations"), 0700); err != nil {
				return err
			}
			j.ID = s.NextID
			j.AdminID = fmt.Sprintf("gg-%d-%s", j.ID, rand.Text())
			s.NextID++ // Reserved IDs must also remain unique for legacy writers.
			if err := atomicJSON(filepath.Join(p.Dir, "state.json"), s); err != nil {
				return err
			}
			var err error
			lease, err = os.OpenFile(p.jobFile(j.ID, ".lock"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				return err
			}
			if err := unix.Flock(int(lease.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
				return err
			}
			if err := p.saveJob(j); err != nil {
				return err
			}
			if replaces > 0 {
				return p.removeJob(replaces)
			}
			return nil
		}, false)
	})
	if err != nil {
		if lease != nil {
			_ = p.removeJob(j.ID)
			_ = lease.Close()
		}
		return nil, nil, err
	}
	return j, lease, nil
}

func (p *Project) checkWorktreeSlot(s *State, j *worktreeJob, replaces int) error {
	existsErr := fmt.Errorf(p.text("session %q already exists; use start or dashboard", "сессия %q уже существует; используйте start или dashboard"), j.Name)
	for _, t := range s.Tasks {
		if !t.plain() && t.label() == j.Name {
			return existsErr
		}
		if !t.plain() && t.Path == j.Path {
			return fmt.Errorf(p.text("session directory is already registered: %s", "каталог сессии уже зарегистрирован: %s"), j.Path)
		}
	}
	jobs, err := p.readJobs()
	if err != nil {
		return err
	}
	retryOK := replaces <= 0
	for _, other := range jobs {
		if other.Phase == "failed" {
			if replaces == other.ID && j.SectionID == other.SectionID && other.Dashboard {
				retryOK = true
			}
			continue
		}
		if err := p.recoverAbandonedJob(other.ID); err != nil {
			return err
		}
		if other.Name == j.Name || other.Path == j.Path {
			return existsErr
		}
		if j.Branch != "" && other.Branch == j.Branch {
			return fmt.Errorf(p.text("branch %q is in use by a worktree being created", "ветка %q уже занята создаваемым worktree"), j.Branch)
		}
	}
	if !retryOK {
		return errors.New(p.text("The failed creation no longer exists", "Запись о неудавшемся создании больше не существует"))
	}
	if err := ensureSessionsDir(p.Root + "__sessions"); err != nil {
		return err
	}
	if _, err := os.Lstat(j.Path); err == nil {
		return fmt.Errorf(p.text("session directory already exists: %s", "каталог сессии уже существует: %s"), j.Path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (p *Project) resolveWorktreeBase(j *worktreeJob, base string, skipCopy bool) error {
	if !skipCopy {
		var err error
		j.Source, err = p.copySource(j.CWD, base)
		if err != nil {
			return err
		}
	}
	if j.Branch != "" {
		_, err := command(j.CWD, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+j.Branch)
		var exitErr *exec.ExitError
		switch {
		case err == nil && base != "":
			return errors.New("--base cannot be used with an existing branch")
		case err == nil:
			base = "refs/heads/" + j.Branch
			inUse, err := p.worktreeForBranch(base)
			if err != nil {
				return err
			}
			if inUse != "" {
				return fmt.Errorf(p.text("branch %q is already checked out at %s", "ветка %q уже открыта в %s"), j.Branch, inUse)
			}
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
			j.CreateBranch = true
		default:
			return err
		}
	}
	if base == "" {
		base = "HEAD"
	}
	commit, err := command(j.CWD, "git", "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
	if err != nil {
		return fmt.Errorf("cannot resolve base commit: %w", err)
	}
	j.Base = commit
	return nil
}

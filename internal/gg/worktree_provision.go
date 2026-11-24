package gg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func (p *Project) provisionWorktree(j *worktreeJob, lease *os.File) (*Task, error) {
	t := j.task()
	if err := p.prepareCheckout(j, t, lease); err != nil {
		err = p.rollbackWorktree(j, t, err)
		return nil, errors.Join(err, p.failWorktreeJob(j, err))
	}
	// The task and terminal have been committed. A subsequent reader can also
	// retire a leftover job after an interruption between commit and cleanup.
	_ = p.removeJob(j.ID)
	return t, nil
}

func (p *Project) claimCheckout(j *worktreeJob) error {
	if j.Phase != "preparing" {
		return errors.New("worktree checkout already attempted")
	}
	if j.CreateBranch {
		// Claim the ref separately: worktree add -b can leave a new branch
		// behind even when Git refuses the destination before checkout.
		if err := runWithHookOutput(commandIn(j.CWD, "git", "branch", "--", j.Branch, j.Base)); err != nil {
			return err
		}
		j.OwnsBranch = true
		if err := p.saveJob(j); err != nil {
			// Only this helper has the successful Git receipt if persisting it
			// failed. Roll back while that ownership is still known.
			return p.rollbackWorktree(j, j.task(), err)
		}
	}
	return p.addNamedWorktree(j)
}

func (p *Project) prepareCheckout(j *worktreeJob, t *Task, lease *os.File) error {
	// A killed copy worker cannot release the reservation out from under an
	// active Git mutation. The helper records what Git claimed before it exits.
	checkout := commandIn(p.Root, p.Exe, "_checkout-worktree", "--workspace", strconv.Itoa(j.Workspace), "--job", strconv.Itoa(j.ID))
	checkout.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	checkout.ExtraFiles = []*os.File{lease}
	output, checkoutErr := checkout.CombinedOutput()
	if checkoutErr != nil && len(output) > 0 {
		checkoutErr = errors.New(strings.TrimPrefix(strings.TrimSpace(string(output)), "gg: "))
	}
	latest, err := p.readJob(j.ID)
	if err != nil {
		return errors.Join(checkoutErr, err)
	}
	*j = *latest
	if checkoutErr != nil {
		return checkoutErr
	}
	admin := filepath.Join(p.Common, "worktrees", j.AdminID)
	if !p.ownsCheckout(t, admin) {
		return errors.New("worktree registration changed during creation")
	}
	if j.Source != "" {
		j.Phase = "copying"
		if err := p.saveJob(j); err != nil {
			return err
		}
		count, err := copyIgnored(j.Source, j.Path)
		if err != nil {
			return fmt.Errorf("cannot copy environment: %w", err)
		}
		t.CopySource, t.CopiedFiles = j.Source, count
	}
	j.Phase = "starting"
	if err := p.saveJob(j); err != nil {
		return err
	}
	// A CLI invocation from a new directory creates its section at commit.
	// Existing sections remain pinned by their reservation during preparation.
	target := *p
	target.SectionID = j.SectionID
	committed := false
	_, err = target.withCreationSection(j.CWD, func(selected *Project) (*Task, error) {
		t.SectionID = selected.SectionID
		return t, selected.transaction(func(s *State) error {
			if t.ID >= s.NextID {
				return errors.New("worktree creation reservation was lost")
			}
			for _, other := range s.Tasks {
				if other.ID == t.ID || !other.plain() && (other.label() == j.Name || other.Path == t.Path) {
					return errors.New("worktree creation conflicts with a registered session")
				}
			}
			if !selected.ownsCheckout(t, admin) {
				return errors.New("worktree registration changed during creation")
			}
			if s.HooksEnabled {
				if err := selected.installHooks(t.Path); err != nil {
					return err
				}
				t.Generated = hookHashes(t.Path)
				j.Generated = t.Generated
				if err := selected.saveJob(j); err != nil {
					return err
				}
			}
			if err := selected.ensureServer(); err != nil {
				return err
			}
			// Startup commands claim their token under the registry lock, so a
			// configured shell cannot run them before its task has committed.
			if err := selected.ensureTask(t); err != nil {
				return err
			}
			s.Tasks = append(s.Tasks, t)
			if err := atomicJSON(filepath.Join(selected.Dir, "state.json"), s); err != nil {
				return err
			}
			committed = true
			return nil
		}, false)
	})
	if committed {
		// The section was saved before the callback. A later catalog read
		// failure cannot undo a registered task or its running terminal.
		return nil
	}
	return err
}

func (p *Project) rollbackWorktree(j *worktreeJob, t *Task, cause error) error {
	admin := filepath.Join(p.Common, "worktrees", j.AdminID)
	if p.ownsCheckout(t, admin) {
		// A failed checkout hook can move its worktree or save a commit.
		actual, branchErr := currentBranch(t.Path)
		head, headErr := command(t.Path, "git", "rev-parse", "--verify", "HEAD")
		remove := branchErr == nil && headErr == nil && actual == j.Branch && head == j.Base
		if remove {
			clean, err := p.clean(t)
			remove = err == nil && clean
		}
		if remove {
			if j.Phase == "starting" {
				_ = p.killTask(t.ID)
			}
			if t.LaunchToken != "" {
				_ = os.Remove(p.launchPath(t.LaunchToken))
			}
			args := []string{"worktree", "remove"}
			if len(t.Generated) > 0 {
				args = append(args, "--force")
			}
			args = append(args, "--", t.Path)
			if err := runWithHookOutput(commandIn(p.Root, "git", args...)); err != nil {
				return errors.Join(cause, err)
			}
		} else {
			cause = errors.Join(cause, fmt.Errorf(p.text("worktree with changes preserved at %s", "worktree с изменениями сохранён в %s"), t.Path))
		}
	} else if info, err := os.Lstat(t.Path); err == nil && info.IsDir() {
		// Git may never have registered the empty directory. Its inode is
		// recorded before checkout, and rmdir refuses any nonempty replacement.
		stat := info.Sys().(*syscall.Stat_t)
		if j.DirectoryIno != 0 && stat.Ino == j.DirectoryIno && uint64(stat.Dev) == j.DirectoryDev {
			if err := unix.Rmdir(t.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				cause = errors.Join(cause, fmt.Errorf(p.text("worktree directory preserved: %w", "каталог worktree сохранён: %w"), err))
			}
		}
	}
	if j.OwnsBranch {
		// Neither a pre-existing ref nor one advanced independently is ours.
		oid, err := command(p.Root, "git", "rev-parse", "--verify", "refs/heads/"+j.Branch)
		if err == nil && oid == j.Base {
			err = runWithHookOutput(commandIn(p.Root, "git", "branch", "-D", "--", j.Branch))
			return errors.Join(cause, err)
		}
	}
	return cause
}

func (p *Project) failWorktreeJob(j *worktreeJob, cause error) error {
	if !j.Dashboard {
		return p.removeJob(j.ID)
	}
	j.Phase, j.Error = "failed", cause.Error()
	if err := p.saveJob(j); err != nil {
		return err
	}
	if err := os.Remove(p.jobFile(j.ID, ".lock")); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (p *Project) rollbackInterruptedJob(j *worktreeJob) error {
	state, err := p.load()
	if err != nil {
		return err // Never roll back a checkout whose commit cannot be verified.
	}
	if t, err := taskByID(state, j.ID); err == nil {
		if t.Path != j.Path {
			return errors.New("worktree creation no longer matches the registered session")
		}
		return p.removeJob(j.ID)
	}
	cause := errors.New(p.text("Worktree creation was interrupted", "Создание worktree прервано"))
	cause = p.rollbackWorktree(j, j.task(), cause)
	return p.failWorktreeJob(j, cause)
}

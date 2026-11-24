package gg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (p *Project) validManagedPath(t *Task) bool {
	base := p.Root + "__sessions"
	if !filepath.IsLocal(t.Branch) || t.Branch == "." || filepath.Clean(t.Branch) != t.Branch || strings.Contains(t.Branch, "~") {
		return false
	}
	return t.Path == filepath.Join(base, worktreeDirName(t.Branch)) ||
		t.Path == filepath.Join(base, t.Branch) || t.Path == filepath.Join(base, strconv.Itoa(t.ID))
}

func ensureSessionsDir(base string) error {
	info, err := os.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return os.Mkdir(base, 0755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("session parent is not a real directory: %s", base)
	}
	return nil
}

func taskByWorktreeName(s *State, name string) (*Task, error) {
	for _, t := range s.Tasks {
		if !t.plain() && t.label() == name {
			return t, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", name)
}

func (p *Project) validateWorktree(t *Task) error {
	canonical, err := filepath.EvalSymlinks(t.Path)
	if err != nil || canonical != t.Path {
		return errors.New(p.text("worktree path is unavailable or replaced", "путь worktree недоступен или подменён"))
	}
	if t.plain() {
		info, err := os.Stat(t.Path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("session directory is unavailable")
		}
		return nil
	}
	other, err := Discover(t.Path)
	top, topErr := command(t.Path, "git", "rev-parse", "--show-toplevel")
	if err != nil || topErr != nil || top != t.Path || other.Common != p.Common {
		return errors.New(p.text("worktree no longer belongs to this project", "worktree больше не принадлежит проекту"))
	}
	return nil
}

func (p *Project) Start(id int, template ...string) error {
	return p.locked(func(s *State) error {
		t, err := taskByID(s, id)
		if err != nil {
			return err
		}
		if len(template) > 0 && template[0] != "" {
			if len(p.snapshot().task(id).live()) > 0 {
				return errors.New(p.text("session is active; stop it before changing its template", "сессия активна: остановите её перед сменой шаблона"))
			}
			tpl, err := p.resolveTemplate(template[0])
			if err != nil {
				return err
			}
			if err = validateTemplateShell(tpl); err != nil {
				return err
			}
			t.Template = tpl.Name
		}
		if err = p.validateWorktree(t); err != nil {
			return err
		}
		if err = p.ensureServer(); err != nil {
			return err
		}
		return p.ensureTask(t)
	})
}

func (p *Project) checkDelete(t *Task, force, all bool) error {
	if t.plain() {
		return nil
	}
	if t.Protected {
		if all {
			return nil
		}
		return errors.New(p.text("original worktree is protected; it cannot be deleted", "исходный worktree защищён: удалять его нельзя"))
	}
	jobs, err := p.readJobs()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		relative, relErr := filepath.Rel(t.Path, j.CWD)
		if j.Phase != "failed" && (j.Source == t.Path || relErr == nil && filepath.IsLocal(relative)) {
			if err := p.recoverAbandonedJob(j.ID); err != nil {
				return err
			}
			return fmt.Errorf(p.text("worktree is a copy source for %q; wait for its creation to finish", "worktree используется как источник для %q; дождитесь завершения создания"), j.Name)
		}
	}
	if err := p.validateWorktree(t); err != nil {
		return err
	}
	if force {
		return nil
	}
	clean, err := p.clean(t)
	if err != nil {
		return err
	}
	if !clean {
		return errors.New(p.text("uncommitted or untracked files; commit them or explicitly use --force", "есть незакоммиченные или неотслеживаемые файлы; сохраните их либо явно используйте --force"))
	}
	branch, err := currentBranch(t.Path)
	if err != nil || branch != "" {
		return err
	}
	refs, err := command(t.Path, "git", "for-each-ref", "--contains=HEAD", "--count=1", "--format=%(refname)", "refs/heads/", "refs/remotes/", "refs/tags/")
	if err != nil {
		return err
	}
	if refs == "" {
		return errors.New(p.text("no branch or tag contains this detached HEAD; save it with git switch -c <branch> or explicitly use --force", "коммит без ветки отсутствует в истории веток и тегов; сохраните его через git switch -c <ветка> или явно используйте --force"))
	}
	return nil
}

func (p *Project) preflightDelete(ids []int, force, all bool) error {
	return p.transaction(func(s *State) error {
		for _, id := range ids {
			t, err := taskByID(s, id)
			if err != nil {
				return err
			}
			if err := p.checkDelete(t, force, all); err != nil {
				return fmt.Errorf("%s: %w", t.label(), err)
			}
		}
		return nil
	}, false)
}

// Resolve the complete target set before changing processes or files. IDs bind
// delayed UI confirmations/workers to records, even if the list changes.
func (p *Project) operate(action string, ids []int, force, all bool, surviving ...bool) error {
	return p.transaction(func(s *State) error {
		targets := []*Task{}
		for _, id := range ids {
			t, err := taskByID(s, id)
			if err != nil {
				if all && len(surviving) > 0 && surviving[0] {
					continue
				}
				return err
			}
			targets = append(targets, t)
		}
		if len(targets) == 0 && len(surviving) > 0 && surviving[0] {
			return errors.New(p.text("Sessions no longer exist", "Сессии больше не существуют"))
		}
		if action == "delete" {
			for _, t := range targets {
				if err := p.checkDelete(t, force, all); err != nil {
					return fmt.Errorf("%s: %w", t.label(), err)
				}
			}
		}
		var failures []error
		for _, t := range targets {
			if err := p.killTask(t.ID); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", t.label(), err))
				continue
			}
			_ = os.Remove(p.agentPath(t.ID))
			if t.Protected || t.Kind == "temporary" {
				removeTask(s, t.ID)
				continue
			}
			if action != "delete" {
				continue
			}
			if t.plain() {
				removeTask(s, t.ID)
				continue
			}
			if err := p.checkDelete(t, force, all); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", t.label(), err))
				continue
			}
			args := []string{"worktree", "remove"}
			if force || len(t.Generated) > 0 {
				args = append(args, "--force")
			}
			args = append(args, "--", t.Path)
			if _, err := command(p.Root, "git", args...); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", t.label(), err))
				continue
			}
			for i, record := range s.Tasks {
				if record.ID == t.ID {
					s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
					break
				}
			}
		}
		// Save successful removals even when another target failed.
		if err := atomicJSON(filepath.Join(p.Dir, "state.json"), s); err != nil {
			failures = append(failures, err)
		}
		return errors.Join(failures...)
	}, false)
}

func removeTask(s *State, id int) {
	for i, t := range s.Tasks {
		if t.ID == id {
			s.Tasks = append(s.Tasks[:i], s.Tasks[i+1:]...)
			return
		}
	}
}

func (p *Project) inOwnServer() bool {
	if os.Getenv("TMUX") == "" {
		return false
	}
	path, err := p.tmux("display-message", "-p", "#{socket_path}")
	return err == nil && strings.HasPrefix(os.Getenv("TMUX"), path+",")
}

func (p *Project) operationInBackground(action string, ids []int, force, all bool, surviving ...bool) error {
	if err := p.ensureServer(); err != nil {
		return err
	}
	args := []string{"_operate", "--root", p.Root, "--action", action}
	for _, id := range ids {
		args = append(args, "--id", strconv.Itoa(id))
	}
	if force {
		args = append(args, "--force")
	}
	if all {
		args = append(args, "--all")
	}
	if len(surviving) > 0 && surviving[0] {
		args = append(args, "--surviving")
	}
	_, err := p.tmux("run-shell", "-b", p.invocation(args...))
	return err
}

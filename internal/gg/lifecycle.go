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
	if t.Path == filepath.Join(base, strconv.Itoa(t.ID)) {
		return true
	}
	path := filepath.Join(base, t.Branch)
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && t.Path == path
}

func safeSessionParents(base, path string) error {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("invalid session path")
	}
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	current := base
	for i := -1; i < len(parts); i++ {
		if i >= 0 {
			if parts[i] == "." {
				continue
			}
			current = filepath.Join(current, parts[i])
		}
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("session parent is not a real directory: %s", current)
		}
	}
	return nil
}

func taskByBranch(s *State, branch string) (*Task, error) {
	for _, t := range s.Tasks {
		if !t.plain() && t.Branch == branch {
			return t, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", branch)
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

func (p *Project) preflightDelete(ids []int, force, all bool) error {
	return p.transaction(func(s *State) error {
		for _, id := range ids {
			t, err := taskByID(s, id)
			if err != nil {
				return err
			}
			if t.plain() {
				continue
			}
			if t.Protected {
				if all {
					continue
				}
				return errors.New(p.text("original worktree is protected", "исходный worktree защищён"))
			}
			if err = p.validateWorktree(t); err != nil {
				return err
			}
			if !force {
				clean, err := p.clean(t)
				if err != nil {
					return err
				}
				if !clean {
					return errors.New(p.text("uncommitted or untracked files; explicitly use --force", "есть незакоммиченные или неотслеживаемые файлы; явно используйте --force"))
				}
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
				if t.plain() {
					continue
				}
				if t.Protected {
					if all {
						continue
					}
					return errors.New(p.text("original worktree is protected; it cannot be deleted", "исходный worktree защищён: удалять его нельзя"))
				}
				if err := p.validateWorktree(t); err != nil {
					return fmt.Errorf("%s: %w", t.Branch, err)
				}
				if !force {
					clean, err := p.clean(t)
					if err != nil {
						return err
					}
					if !clean {
						return fmt.Errorf(p.text("%s: uncommitted or untracked files; commit them or explicitly use --force", "%s: есть незакоммиченные или неотслеживаемые файлы; сохраните их либо явно используйте --force"), t.Branch)
					}
				}
			}
		}
		var failures []error
		for _, t := range targets {
			if err := p.killTask(t.ID); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", t.Branch, err))
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
			if !force {
				clean, err := p.clean(t)
				if err != nil || !clean {
					failures = append(failures, fmt.Errorf(p.text("%s: changes found after stopping; files preserved", "%s: после остановки обнаружены изменения; файлы сохранены"), t.Branch))
					continue
				}
			}
			args := []string{"worktree", "remove"}
			if force || len(t.Generated) > 0 {
				args = append(args, "--force")
			}
			args = append(args, "--", t.Path)
			if _, err := command(p.Root, "git", args...); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", t.Branch, err))
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

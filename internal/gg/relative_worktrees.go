package gg

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (p *Project) enableRelativeWorktrees() error {
	if p.Directory {
		return errors.New(p.text("Relative worktree paths require a Git repository", "Относительные пути worktree требуют Git-репозитория"))
	}
	if err := checkDependency("git", "2.48"); err != nil {
		return err
	}
	return p.transaction(func(s *State) error {
		paths, err := p.worktreePathsForRepair(s)
		if err != nil {
			return err
		}
		if err := p.configureRelativeWorktrees(); err != nil {
			return err
		}
		if _, err := command(p.Root, "git", "worktree", "repair", "--relative-paths"); err != nil {
			return fmt.Errorf("%s: %w", p.text("Relative paths enabled; worktree repair incomplete", "Относительные пути включены, восстановление ссылок worktree не завершено"), err)
		}
		for _, path := range paths {
			cmd := commandIn(path, "git", "--no-optional-locks", "status", "--porcelain", "-z")
			var diagnostic strings.Builder
			cmd.Stderr = &diagnostic
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%s %q: %s: %w", p.text("Relative paths enabled; git status failed in", "Относительные пути включены, ошибка git status в"), path, strings.TrimSpace(diagnostic.String()), err)
			}
		}
		return nil
	}, false)
}

func (p *Project) configureRelativeWorktrees() error {
	const key = "worktree.useRelativePaths"
	result, err := commandIn(p.Root, "git", "config", "--show-scope", "--type=bool", "--get", key).CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		// git config --get exits 1 with empty output for a missing key.
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || len(result) != 0 {
			return fmt.Errorf("git config: %s: %w", strings.TrimSpace(string(result)), err)
		}
	}
	value := strings.TrimSpace(string(result))
	conflict := p.text("Another Git setting overrides worktree.useRelativePaths for this repository", "Другая настройка Git переопределяет worktree.useRelativePaths для этого репозитория")
	if value == "worktree\tfalse" || value == "command\tfalse" {
		return errors.New(conflict)
	}
	if _, err := command(p.Root, "git", "config", "--local", key, "true"); err != nil {
		return err
	}
	// A later include in the local configuration can also override the key.
	enabled, err := command(p.Root, "git", "config", "--type=bool", "--get", key)
	if err == nil && enabled != "true" {
		return errors.New(conflict)
	}
	return err
}

func (p *Project) worktreePathsForRepair(s *State) ([]string, error) {
	list, err := command(p.Root, "git", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var paths []string
	listed := make(map[string]bool)
	for field := range strings.SplitSeq(list, "\x00") {
		path, ok := strings.CutPrefix(field, "worktree ")
		if !ok {
			continue
		}
		if listed[path] {
			return nil, fmt.Errorf("%s %q", p.text("Multiple worktrees point to", "Несколько worktree указывают на"), path)
		}
		if err := p.checkWorktreeForRepair(path); err != nil {
			return nil, fmt.Errorf("%q: %w", path, err)
		}
		listed[path] = true
		paths = append(paths, path)
	}
	if !listed[p.Root] {
		return nil, errors.New(p.text("Primary worktree missing from Git worktree list", "Основное рабочее дерево отсутствует в git worktree list"))
	}
	for _, t := range s.Tasks {
		if !t.plain() && !listed[t.Path] {
			return nil, fmt.Errorf("%s %q", p.text("Git worktree list is missing the registered session at", "В git worktree list отсутствует зарегистрированная сессия по пути"), t.Path)
		}
	}
	return paths, nil
}

func (p *Project) checkWorktreeForRepair(path string) error {
	// Paths can contain newlines. Match both known paths together to avoid
	// splitting a path or accepting a nested or foreign checkout.
	identity, err := command(path, "git", "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir", "--absolute-git-dir")
	if err != nil {
		return err
	}
	admin, matches := strings.CutPrefix(identity, path+"\n"+p.Common+"\n")
	if !matches {
		return errors.New(p.text("worktree no longer belongs to this project", "worktree больше не принадлежит проекту"))
	}
	if path == p.Root {
		return nil
	}
	if filepath.Dir(admin) != filepath.Join(p.Common, "worktrees") {
		return fmt.Errorf("%s %q", p.text("Unexpected Git administrative directory", "Неожиданный служебный каталог Git"), admin)
	}
	for _, file := range []string{filepath.Join(path, ".git"), filepath.Join(admin, "gitdir")} {
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s %q", p.text("Expected a regular worktree link file at", "Ожидался обычный файл связи worktree по пути"), file)
		}
	}
	return nil
}

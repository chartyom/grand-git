package gg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// A hook can leave descendants holding its output descriptors. A regular file
// preserves diagnostics while allowing creation to return as soon as Git exits.
func runWithHookOutput(cmd *exec.Cmd) error {
	output, err := os.CreateTemp("", "gg-worktree-*")
	if err != nil {
		return err
	}
	defer output.Close()
	if err = os.Remove(output.Name()); err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = output, output
	if err = cmd.Run(); err != nil {
		info, statErr := output.Stat()
		if statErr != nil {
			return err
		}
		b, _ := io.ReadAll(io.NewSectionReader(output, 0, info.Size()))
		return fmt.Errorf("%s: %s: %w", filepath.Base(cmd.Path), strings.TrimSpace(string(b)), err)
	}
	return nil
}

// Git stores a worktree's canonical directory and derives its private admin ID
// from the supplied basename. The private ID lets rollback identify our checkout
// even when another Git process adds a detached tree at the same path and commit.
func (p *Project) addNamedWorktree(j *worktreeJob) error {
	t, branch, adminID := j.task(), j.Branch, j.AdminID
	adminDir := filepath.Join(p.Common, "worktrees", adminID)
	if _, err := os.Lstat(adminDir); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("worktree administration path unavailable: %s", adminDir)
	}
	private, err := os.MkdirTemp("", "gg-add-*")
	if err != nil {
		return err
	}
	defer os.Remove(private)
	alias, err := filepath.Abs(filepath.Join(private, adminID))
	if err != nil {
		return err
	}
	if err := os.Symlink(t.Path, alias); err != nil {
		return err
	}
	defer os.Remove(alias)
	if err := os.Mkdir(t.Path, 0755); err != nil {
		return err
	}
	dir, err := os.Open(t.Path)
	if err != nil {
		_ = unix.Rmdir(t.Path)
		return err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	stat := info.Sys().(*syscall.Stat_t)
	j.DirectoryDev, j.DirectoryIno = uint64(stat.Dev), stat.Ino
	removeEmpty := func() {
		current, err := os.Lstat(t.Path)
		if err == nil && os.SameFile(info, current) {
			_ = unix.Rmdir(t.Path)
		}
	}
	if err := p.saveJob(j); err != nil {
		removeEmpty()
		return err
	}
	args := []string{"worktree", "add"}
	start := branch
	if start == "" {
		args = append(args, "--detach")
		start = t.Base
	}
	args = append(args, "--", alias, start)
	if err = runWithHookOutput(commandIn(p.Root, "git", args...)); err != nil {
		removeEmpty()
		return errors.New(strings.ReplaceAll(err.Error(), alias, t.Path))
	}
	return nil
}

func (p *Project) ownsCheckout(t *Task, adminDir string) bool {
	if adminDir == "" {
		return false
	}
	actual, err := command(t.Path, "git", "rev-parse", "--path-format=absolute", "--absolute-git-dir", "--show-toplevel", "--git-common-dir")
	return err == nil && actual == strings.Join([]string{adminDir, t.Path, p.Common}, "\n")
}

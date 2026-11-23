package gg

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var errCopyConflict = errors.New("target belongs to the checked-out branch")

func (p *Project) copySource(cwd, base string) (string, error) {
	source, err := command(cwd, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if base != "" {
		ref, _ := command(cwd, "git", "rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", base)
		if strings.HasPrefix(ref, "refs/heads/") {
			path, err := p.worktreeForBranch(ref)
			if err != nil {
				return "", err
			}
			if path != "" {
				source = path
			}
		}
	}
	return filepath.EvalSymlinks(source)
}

func (p *Project) worktreeForBranch(ref string) (string, error) {
	if p.legacyWorktreeList {
		out, err := command(p.Root, "git", "for-each-ref", "--count=1", "--format=%(refname)%00%(worktreepath)%00", "--", ref)
		if err != nil {
			return "", err
		}
		name, rest, _ := strings.Cut(out, "\x00")
		if path, _, ok := strings.Cut(rest, "\x00"); ok && name == ref {
			return path, nil
		}
		return "", nil
	}
	out, err := command(p.Root, "git", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", err
	}
	path := ""
	for field := range strings.SplitSeq(out, "\x00") {
		if value, ok := strings.CutPrefix(field, "worktree "); ok {
			path = value
		} else if field == "branch "+ref {
			return path, nil
		}
	}
	return "", nil
}

type ignoredCopier struct {
	source, target string
	dirs           map[string]bool
	created        []string
	buffer         []byte
	count          int
}

func safeCopyPath(path string) bool {
	if first := strings.Split(filepath.Clean(path), string(filepath.Separator))[0]; first == ".gg" {
		return false
	}
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) == ".." || strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator)) {
		return false
	}
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == ".git" {
			return false
		}
	}
	return true
}

// Every directory is checked with Lstat: a symlink in the target may point
// outside the new worktree and must never be followed or overwritten.
func (c *ignoredCopier) ensureDir(rel string) error {
	if rel == "." || rel == "" {
		return nil
	}
	if !safeCopyPath(rel) {
		return errCopyConflict
	}
	if c.dirs[rel] {
		return nil
	}
	if err := c.ensureDir(filepath.Dir(rel)); err != nil {
		return err
	}
	src, err := os.Lstat(filepath.Join(c.source, rel))
	if err != nil {
		return err
	}
	if !src.IsDir() {
		return errCopyConflict
	}
	if _, err := os.Lstat(filepath.Join(c.source, rel, ".git")); err == nil {
		return errCopyConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dest := filepath.Join(c.target, rel)
	info, err := os.Lstat(dest)
	if err == nil {
		if !info.IsDir() {
			return errCopyConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		if err := os.Mkdir(dest, src.Mode().Perm()|0700); err != nil {
			return err
		}
		c.created = append(c.created, rel)
	}
	c.dirs[rel] = true
	return nil
}

func (c *ignoredCopier) entry(rel string) error {
	if !safeCopyPath(rel) {
		return nil
	}
	if err := c.ensureDir(filepath.Dir(rel)); err != nil {
		if errors.Is(err, errCopyConflict) {
			return nil
		}
		return err
	}
	src := filepath.Join(c.source, rel)
	dest := filepath.Join(c.target, rel)
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	} // Nested Git repositories aren't traversed.
	if _, err := os.Lstat(dest); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.Symlink(link, dest); err != nil {
			return err
		}
		c.count++
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	} // sockets, pipes, devices
	fd, err := unix.Open(src, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	in := os.NewFile(uintptr(fd), src)
	defer in.Close()
	info, err = in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := cloneFile(out, in); err != nil {
		if err := out.Truncate(0); err != nil {
			return err
		}
		if _, err := io.CopyBuffer(out, in, c.buffer); err != nil {
			return err
		}
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(dest, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	c.count++
	return nil
}

func copyIgnored(source, target string) (int, error) {
	out, err := command(source, "git", "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return 0, err
	}
	c := &ignoredCopier{source: source, target: target, dirs: map[string]bool{}, buffer: make([]byte, 128*1024)}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		if err := c.entry(filepath.FromSlash(rel)); err != nil {
			return 0, fmt.Errorf("copying %q: %w", rel, err)
		}
	}
	// Git's file listing omits empty directories. Collapsed ignored directory
	// entries supply roots for preserving their directory structure separately.
	dirs, err := command(source, "git", "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return 0, err
	}
	for _, rel := range strings.Split(dirs, "\x00") {
		if !strings.HasSuffix(rel, "/") || !safeCopyPath(rel) {
			continue
		}
		root := filepath.Join(source, filepath.FromSlash(rel))
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			if !safeCopyPath(relative) {
				return filepath.SkipDir
			}
			if err := c.ensureDir(relative); errors.Is(err, errCopyConflict) {
				return filepath.SkipDir
			} else {
				return err
			}
		})
		if err != nil {
			return 0, err
		}
	}
	for i := len(c.created) - 1; i >= 0; i-- {
		rel := c.created[i]
		info, err := os.Stat(filepath.Join(source, rel))
		if err != nil {
			return 0, err
		}
		if err := os.Chmod(filepath.Join(target, rel), info.Mode().Perm()); err != nil {
			return 0, err
		}
		if err := os.Chtimes(filepath.Join(target, rel), info.ModTime(), info.ModTime()); err != nil {
			return 0, err
		}
	}
	return c.count, nil
}

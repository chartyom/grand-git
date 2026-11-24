package gg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativeWorktreesPreserveConfigAndLinksOnOverride(t *testing.T) {
	if err := checkDependency("git", "2.48"); err != nil {
		t.Skip(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_COUNT", "0")
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	git := func(args ...string) string {
		t.Helper()
		result, err := command(root, "git", args...)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	git("init")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "base")
	git("worktree", "add", "--detach", link)
	git("config", "--local", "extensions.worktreeConfig", "true")
	p, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	admin := git("-C", link, "rev-parse", "--absolute-git-dir")
	files := []string{filepath.Join(p.Common, "config"), filepath.Join(admin, "gitdir"), filepath.Join(link, ".git")}
	for _, scope := range []string{"worktree", "command"} {
		t.Run(scope, func(t *testing.T) {
			if scope == "worktree" {
				git("config", "--worktree", "worktree.useRelativePaths", "false")
				defer git("config", "--worktree", "--unset", "worktree.useRelativePaths")
			} else {
				t.Setenv("GIT_CONFIG_COUNT", "1")
				t.Setenv("GIT_CONFIG_KEY_0", "worktree.useRelativePaths")
				t.Setenv("GIT_CONFIG_VALUE_0", "false")
			}
			before := make(map[string][]byte)
			for _, file := range files {
				before[file], err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := p.enableRelativeWorktrees(); err == nil || !strings.Contains(err.Error(), "worktree.useRelativePaths") {
				t.Fatalf("expected a setting conflict, got %v", err)
			}
			for _, file := range files {
				content, err := os.ReadFile(file)
				if err != nil || !bytes.Equal(before[file], content) {
					t.Fatalf("file changed on conflict: %s (%v)", file, err)
				}
			}
		})
	}
}

func TestRelativeWorktreesRejectOlderGitBeforeWriting(t *testing.T) {
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\ntest \"$1\" = --version || exit 2\necho 'git version 2.47.0'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	p := &Project{Root: t.TempDir(), Dir: filepath.Join(t.TempDir(), "registry")}
	err := p.enableRelativeWorktrees()
	if err == nil || !strings.Contains(err.Error(), "2.48") {
		t.Fatalf("expected Git version diagnostic, got %v", err)
	}
	if _, err := os.Stat(p.Dir); !os.IsNotExist(err) {
		t.Fatalf("unexpected registry after rejecting Git: %v", err)
	}
}

func TestEnableRelativeWorktreesAcrossMounts(t *testing.T) {
	if err := checkDependency("git", "2.48"); err != nil {
		t.Skip(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	mount := t.TempDir()
	vm := filepath.Join(mount, "vm")
	host := filepath.Join(mount, "host")
	root := filepath.Join(vm, "project")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := command(dir, "git", args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	git(root, "init")
	git(root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "base")
	tree := filepath.Join(vm, "project__sessions", "branch with\nnewline")
	git(root, "worktree", "add", "--detach", tree)
	writeFixture(t, filepath.Join(tree, "pending"), "user change")
	p, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	// A replacement checkout at a registered path must be left untouched.
	moved := tree + "-moved"
	if err := os.Rename(tree, moved); err != nil {
		t.Fatal(err)
	}
	git(root, "init", tree)
	if err := p.enableRelativeWorktrees(); err == nil {
		t.Fatal("accepted a foreign repository at a registered path")
	}
	if _, err := command(root, "git", "config", "--local", "--get", "worktree.useRelativePaths"); err == nil {
		t.Fatal("a failed preflight enabled the option")
	}
	if err := os.Rename(tree, tree+"-foreign"); err != nil {
		t.Fatal(err)
	}
	if err := p.enableRelativeWorktrees(); err == nil {
		t.Fatal("accepted an unavailable registered worktree")
	}
	if err := os.Rename(moved, tree); err != nil {
		t.Fatal(err)
	}
	// Repair can succeed before status finds an unreadable index. A retry
	// must verify those already-converted links after the index is restored.
	index := filepath.Join(git(tree, "rev-parse", "--absolute-git-dir"), "index")
	if err := os.Rename(index, index+".saved"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, index, "invalid index")
	if err := p.enableRelativeWorktrees(); err == nil || !strings.Contains(err.Error(), "git status") {
		t.Fatalf("expected the status verification to report the damaged index, got %v", err)
	}
	if err := os.Rename(index+".saved", index); err != nil {
		t.Fatal(err)
	}
	retries := make(chan error, 2)
	for range 2 {
		go func() { retries <- p.enableRelativeWorktrees() }()
	}
	for range 2 {
		if err := <-retries; err != nil {
			t.Fatal(err)
		}
	}
	if got := git(root, "config", "--local", "--get", "worktree.useRelativePaths"); got != "true" {
		t.Fatalf("unexpected worktree setting %q", got)
	}
	second := filepath.Join(vm, "new-tree")
	git(root, "worktree", "add", "--detach", second)
	for _, path := range []string{tree, second} {
		admin := git(path, "rev-parse", "--absolute-git-dir")
		for _, file := range []string{filepath.Join(path, ".git"), filepath.Join(admin, "gitdir")} {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.IsAbs(strings.TrimPrefix(string(content), "gitdir: ")) {
				t.Fatalf("expected a relative link in %q, got %s", file, content)
			}
		}
	}
	if err := os.Rename(vm, host); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{tree, second} {
		rel, err := filepath.Rel(vm, path)
		if err != nil {
			t.Fatal(err)
		}
		status := git(filepath.Join(host, rel), "status", "--porcelain")
		if path == tree && status != "?? pending" {
			t.Fatalf("missing worktree changes after remount: %q", status)
		}
	}
}

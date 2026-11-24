package gg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type gitCache struct {
	Updated  time.Time
	States   map[string]string
	Branches map[string]string
}

// A quiet symbolic-ref exits with status 1 for detached HEAD.
// Reading the full ref avoids disambiguated names when a tag has the same name.
func currentBranch(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--git-dir=.git", "symbolic-ref", "--quiet", "HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if exit.ExitCode() == 1 {
				return "", nil
			}
			return "", fmt.Errorf("git symbolic-ref: %s: %w", strings.TrimSpace(string(exit.Stderr)), err)
		}
		return "", err
	}
	ref := strings.TrimSuffix(string(output), "\n")
	return strings.TrimPrefix(ref, "refs/heads/"), nil
}

func (t *Task) currentBranch() (string, error) {
	if t.plain() {
		return "", nil
	}
	return currentBranch(t.Path)
}

func (p *Project) gitStates(s *State) gitCache {
	path := filepath.Join(p.Dir, "git-status.json")
	read := func() gitCache {
		var c gitCache
		if b, e := os.ReadFile(path); e == nil {
			_ = json.Unmarshal(b, &c)
		}
		return c
	}
	cache := read()
	if time.Since(cache.Updated) < 2*time.Second {
		return cache
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return cache
	}
	defer f.Close()
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return cache
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	cache = read()
	if time.Since(cache.Updated) < 2*time.Second {
		return cache
	}
	cache = gitCache{Updated: time.Now(), States: map[string]string{}, Branches: map[string]string{}}
	for _, t := range s.Tasks {
		if t.plain() {
			cache.States[strconv.Itoa(t.ID)] = "n/a"
			continue
		}
		if branch, err := currentBranch(t.Path); err == nil {
			cache.Branches[strconv.Itoa(t.ID)] = branch
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=normal")
		cmd.Dir = t.Path
		b, e := cmd.Output()
		cancel()
		value := "unknown"
		if e == nil {
			value = "clean"
			if len(b) > 0 {
				value = "modified"
			}
		}
		cache.States[strconv.Itoa(t.ID)] = value
	}
	_ = atomicJSON(path, cache)
	return cache
}

func (p *Project) updateCounter(s *State, snap Snapshot, cfg Settings) {
	active := 0
	for _, t := range s.Tasks {
		if len(snap.task(t.ID).live()) > 0 {
			active++
		}
	}
	value := cfg.text("Active", "Активно") + " " + strconv.Itoa(active) + "/" + strconv.Itoa(len(s.Tasks)) + " · Ctrl+W " + cfg.text("Next", "Далее")
	old, _ := p.tmux("show-options", "-g", "-v", "status-right")
	if old != value {
		p.tmux("set-option", "-g", "status-right", value)
	}
}

func (p *Project) updateGroupCounter(groups []sessionGroup, cfg Settings) {
	active, total := 0, 0
	for _, g := range groups {
		if g.Err != nil || g.State == nil {
			continue
		}
		for _, t := range g.State.Tasks {
			total++
			if len(g.Snap.task(t.ID).live()) > 0 {
				active++
			}
		}
	}
	value := cfg.text("Active", "Активно") + " " + strconv.Itoa(active) + "/" + strconv.Itoa(total) + " · Ctrl+W " + cfg.text("Next", "Далее")
	old, _ := p.tmux("show-options", "-g", "-v", "status-right")
	if old != value {
		p.tmux("set-option", "-g", "status-right", value)
	}
}

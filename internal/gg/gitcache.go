package gg

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

type gitCache struct {
	Updated time.Time
	States  map[string]string
}

func (p *Project) gitStates(s *State) map[string]string {
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
		return cache.States
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return cache.States
	}
	defer f.Close()
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return cache.States
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	cache = read()
	if time.Since(cache.Updated) < 2*time.Second {
		return cache.States
	}
	cache = gitCache{Updated: time.Now(), States: map[string]string{}}
	for _, t := range s.Tasks {
		if t.plain() {
			cache.States[strconv.Itoa(t.ID)] = "n/a"
			continue
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
	return cache.States
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

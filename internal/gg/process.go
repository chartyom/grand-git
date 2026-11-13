package gg

import (
	"os"
	"syscall"
	"time"
)

type processInfo struct {
	pid, parent int
	start       string
}

func stopPaneProcesses(panes Snapshot) error {
	roots := map[int]bool{}
	for _, pane := range panes {
		if pane.PID > 1 {
			roots[pane.PID] = true
		}
	}
	processes, err := processTable()
	if err != nil {
		return err
	}
	owned := map[int]processInfo{}
	for changed := true; changed; {
		changed = false
		for pid, p := range processes {
			if pid == os.Getpid() {
				continue
			}
			if _, ok := owned[pid]; ok {
				continue
			}
			if _, ok := owned[p.parent]; roots[pid] || ok {
				owned[pid] = p
				changed = true
			}
		}
	}
	signal := func(p processInfo, sig syscall.Signal) {
		if current, ok := readProcess(p.pid); ok && current.start == p.start {
			_ = syscall.Kill(p.pid, sig)
		}
	}
	for pid, p := range owned {
		sig := syscall.SIGTERM
		if roots[pid] {
			sig = syscall.SIGHUP
		}
		signal(p, sig)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for pid, p := range owned {
			if current, ok := readProcess(pid); ok && current.start == p.start {
				alive = true
				break
			}
		}
		if !alive {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, p := range owned {
		signal(p, syscall.SIGKILL)
	}
	return nil
}

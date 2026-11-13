package gg

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func newGeneration() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func (s Snapshot) temporary() bool {
	for _, p := range s.live() {
		if p.Generation != "" {
			return true
		}
	}
	return false
}

type dashboardClient struct{ identity, name string }

func (p *Project) dashboardClients() ([]dashboardClient, error) {
	out, err := p.tmux("list-clients", "-F", "#{client_pid}:#{client_created}\t#{client_name}\t#{session_name}")
	if err != nil {
		return nil, err
	}
	var clients []dashboardClient
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) == 3 && (fields[2] == "dashboard" || strings.HasPrefix(fields[2], "run-")) {
			clients = append(clients, dashboardClient{fields[0], fields[1]})
		}
	}
	return clients, nil
}

func (p *Project) inputClient(pane string) (string, error) {
	args := []string{"display-message", "-p"}
	if pane != "" {
		args = append(args, "-t", pane)
	}
	// tmux resolves the active input client, including an existing own-server client.
	identity, err := p.tmux(append(args, "#{client_pid}:#{client_created}")...)
	if err != nil {
		return "", err
	}
	clients, err := p.dashboardClients()
	if err != nil {
		return "", err
	}
	for _, client := range clients {
		if client.identity == identity {
			return identity, nil
		}
	}
	return "", errors.New(p.text("dashboard client is no longer attached", "клиент dashboard больше не подключён"))
}

type quitTarget struct {
	ID                 int
	Window, Generation string
}

func (p *Project) quitInBackground(client string) error {
	if r, err := readRun(clientPID(client)); err == nil {
		if proc, ok := readProcess(r.PID); ok && proc.start == r.Start {
			_, err = p.tmux("run-shell", "-b", p.invocation("_quit-run", "--root", p.Root, "--client", client, "--run-token", r.Token))
			return err
		}
	}
	targets := []quitTarget{}
	err := p.transaction(func(s *State) error {
		snap := p.snapshot()
		for _, task := range s.Tasks {
			if !task.Protected {
				continue
			}
			for _, pane := range snap.task(task.ID).live() {
				if pane.Generation != "" {
					targets = append(targets, quitTarget{task.ID, pane.Window, pane.Generation})
					break
				}
			}
		}
		return nil
	}, false)
	if err != nil {
		return err
	}
	data, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	_, err = p.tmux("run-shell", "-b", p.invocation("_quit", "--root", p.Root, "--client", client, "--targets", string(data)))
	return err
}

// Run outside the caller's PTY. Bind both the client and terminal generation so
// a delayed worker cannot detach a replacement client or stop a restarted shell.
func (p *Project) quit(identity string, targets []quitTarget) error {
	return p.transaction(func(s *State) error {
		clients, err := p.dashboardClients()
		if err != nil {
			return err
		}
		found := false
		for _, client := range clients {
			if client.identity != identity {
				continue
			}
			if pid, err := strconv.Atoi(strings.SplitN(identity, ":", 2)[0]); err == nil {
				_ = os.Remove(p.attachLeasePath(pid))
			}
			if _, err := p.tmux("detach-client", "-t", client.name); err != nil {
				return err
			}
			found = true
			break
		}
		if !found {
			return nil
		}
		clients, err = p.dashboardClients()
		if err != nil {
			return err
		}
		if len(clients) > 0 || p.pendingAttach() {
			return nil
		}
		var failures []error
		for _, target := range targets {
			task, err := taskByID(s, target.ID)
			if err != nil || !task.Protected || target.Generation == "" {
				continue
			}
			current := p.snapshot().task(task.ID).live()
			matches := len(current) > 0
			for _, pane := range current {
				if pane.Window != target.Window || pane.Generation != target.Generation {
					matches = false
				}
			}
			if !matches {
				continue
			}
			if err := p.killTask(task.ID); err != nil {
				failures = append(failures, err)
				continue
			}
			_ = os.Remove(p.agentPath(task.ID))
		}
		return errors.Join(failures...)
	}, false)
}

type attachLease struct {
	PID   int
	Start string
}

func (p *Project) attachLeasePath(pid int) string {
	return filepath.Join(p.Dir, "attaches", strconv.Itoa(pid)+".json")
}
func (p *Project) prepareAttachLease() error {
	proc, ok := readProcess(os.Getpid())
	if !ok {
		return fmt.Errorf("cannot identify attaching process")
	}
	if err := os.MkdirAll(filepath.Join(p.Dir, "attaches"), 0700); err != nil {
		return err
	}
	return atomicJSON(p.attachLeasePath(proc.pid), attachLease{proc.pid, proc.start})
}
func (p *Project) pendingAttach() bool {
	entries, err := os.ReadDir(filepath.Join(p.Dir, "attaches"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(p.Dir, "attaches", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return true
		}
		var lease attachLease
		if json.Unmarshal(data, &lease) != nil {
			return true
		}
		if proc, ok := readProcess(lease.PID); ok && proc.start == lease.Start {
			return true
		}
		_ = os.Remove(path)
	}
	return false
}

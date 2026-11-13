package gg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type runOrigin struct {
	Workspace int    `json:"workspace"`
	Task      int    `json:"task"`
	Token     string `json:"token"`
	Path      string `json:"path,omitempty"`
}
type dashboardRun struct {
	PID     int         `json:"pid"`
	Start   string      `json:"start"`
	Token   string      `json:"token"`
	Origins []runOrigin `json:"origins"`
}

func runPath(pid int) string {
	return filepath.Join(userStateDir(), "clients", strconv.Itoa(pid)+".json")
}
func readRun(pid int) (dashboardRun, error) {
	var r dashboardRun
	b, err := os.ReadFile(runPath(pid))
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	if err == nil && (r.PID != pid || len(r.Token) != 32) {
		err = errors.New("invalid dashboard client context")
	}
	return r, err
}
func clientPID(identity string) int {
	v, _ := strconv.Atoi(strings.SplitN(identity, ":", 2)[0])
	return v
}

func (p *Project) clientName(identity string) (string, error) {
	clients, err := p.dashboardClients()
	if err != nil {
		return "", err
	}
	for _, client := range clients {
		if client.identity == identity {
			return client.name, nil
		}
	}
	return "", errors.New("dashboard client is no longer attached")
}
func bindRun(pid int, origin runOrigin) error {
	proc, ok := readProcess(pid)
	if !ok {
		return errors.New("cannot identify dashboard client")
	}
	r, err := readRun(pid)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if r.Start != proc.start {
		r = dashboardRun{PID: pid, Start: proc.start}
	}
	r.Token, err = newGeneration()
	if err != nil {
		return err
	}
	r.Origins = append(r.Origins, origin)
	if err = os.MkdirAll(filepath.Dir(runPath(pid)), 0700); err != nil {
		return err
	}
	return atomicJSON(runPath(pid), r)
}

// Locate an enclosing Grand Git client without nesting another tmux client.
func enclosingDashboard() (*Project, string, error) {
	if os.Getenv("TMUX") == "" {
		return nil, "", nil
	}
	c, err := readCatalog()
	if err != nil {
		return nil, "", err
	}
	for _, r := range c.Workspaces {
		p, e := r.project()
		if e != nil {
			continue
		}
		if p.inOwnServer() {
			identity, e := p.inputClient(os.Getenv("TMUX_PANE"))
			return p, identity, e
		}
	}
	return nil, "", nil
}

func (p *Project) connect(id int) error {
	if err := p.prepareAttachLease(); err != nil {
		return err
	}
	r, err := readRun(os.Getpid())
	if err != nil {
		return err
	}
	if err := p.openTaskForRun(id, r.Token); err != nil {
		_ = os.Remove(p.attachLeasePath(os.Getpid()))
		return err
	}
	_, _ = p.tmux("set-option", "-t", "run-"+r.Token, "@gg_client_pid", strconv.Itoa(os.Getpid()))
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "TMUX=") && !strings.HasPrefix(v, "TMUX_PANE=") {
			env = append(env, v)
		}
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	err = syscall.Exec(path, []string{path, "-L", p.Socket, "attach-session", "-t", "=run-" + r.Token}, env)
	_ = os.Remove(p.attachLeasePath(os.Getpid()))
	return err
}

func (p *Project) disconnectedRun(pid int, token string) error {
	for attempt := 0; attempt < 60; attempt++ {
		r, err := readRun(pid)
		if err != nil || r.Token != token {
			return nil
		}
		attached := false
		catalog, err := readCatalog()
		if err != nil {
			return err
		}
		for _, record := range catalog.Workspaces {
			owner, err := record.project()
			if err != nil {
				continue
			}
			clients, _ := owner.dashboardClients()
			for _, client := range clients {
				if clientPID(client.identity) == pid {
					attached = true
				}
			}
		}
		if attached {
			return nil
		}
		if proc, ok := readProcess(pid); !ok || proc.start != r.Start || attempt == 59 {
			return p.finishRun(pid, token)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
func (p *Project) handoff(target *Project, id int, identity string) error {
	if target.WorkspaceID == p.WorkspaceID {
		r, err := readRun(clientPID(identity))
		if err != nil {
			return err
		}
		if err := target.openTaskForRun(id, r.Token); err != nil {
			return err
		}
		name, err := p.clientName(identity)
		if err != nil {
			return err
		}
		_, err = p.tmux("switch-client", "-c", name, "-t", "=run-"+r.Token)
		return err
	}
	clients, err := p.dashboardClients()
	if err != nil {
		return err
	}
	for _, client := range clients {
		if client.identity != identity {
			continue
		}
		if err = target.Start(id); err != nil {
			return err
		}
		_, err = p.tmux("detach-client", "-t", client.name, "-E", "exec "+target.invocation("_connect", "--root", target.Root, "--task", strconv.Itoa(id)))
		return err
	}
	return errors.New("dashboard client is no longer attached")
}

func (p *Project) quitRun(identity, token string) error {
	pid := clientPID(identity)
	r, err := readRun(pid)
	if err != nil {
		return err
	}
	if r.Token != token {
		return nil
	}
	proc, ok := readProcess(pid)
	if !ok || proc.start != r.Start {
		return nil
	}
	clients, err := p.dashboardClients()
	if err != nil {
		return err
	}
	found := false
	for _, client := range clients {
		if client.identity == identity {
			if _, err = p.tmux("detach-client", "-t", client.name); err != nil {
				return err
			}
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return p.finishRun(pid, token)
}

func (p *Project) finishRun(pid int, token string) error {
	r, err := readRun(pid)
	if errors.Is(err, os.ErrNotExist) || r.Token != token {
		return nil
	}
	if err != nil {
		return err
	}
	var failures []error
	for _, origin := range r.Origins {
		owner, e := workspaceByID(origin.Workspace)
		if e != nil {
			failures = append(failures, e)
			continue
		}
		e = owner.transaction(func(s *State) error {
			t, e := taskByID(s, origin.Task)
			if e != nil {
				return nil
			}
			if t.Kind != "temporary" || t.RunToken != origin.Token {
				return nil
			}
			for _, pane := range owner.snapshot().task(t.ID).live() {
				if pane.Generation != origin.Token {
					return nil
				}
			}
			if e = owner.killTask(t.ID); e != nil {
				return e
			}
			removeTask(s, t.ID)
			_ = os.Remove(owner.agentPath(t.ID))
			return atomicJSON(filepath.Join(owner.Dir, "state.json"), s)
		}, false)
		if e != nil {
			failures = append(failures, fmt.Errorf("%s: %w", owner.Root, e))
		}
	}
	if current, e := readRun(pid); e == nil && current.Token == token {
		_ = os.Remove(runPath(pid))
	}
	for _, origin := range r.Origins {
		owner, e := workspaceByID(origin.Workspace)
		if e == nil {
			_, _ = owner.tmux("kill-session", "-t", "run-"+token)
		}
	}
	return errors.Join(failures...)
}

func (p *Project) removeEndedTemporary(id int, token string) error {
	if id < 1 || len(token) != 32 {
		return nil
	}
	return p.transaction(func(s *State) error {
		t, err := taskByID(s, id)
		if err != nil || t.Kind != "temporary" || t.RunToken != token {
			return nil
		}
		snap, err := p.checkedSnapshot()
		if err != nil || len(snap.task(id).live()) != 0 {
			return err
		}
		if err = p.killTask(id); err != nil {
			return err
		}
		removeTask(s, id)
		return atomicJSON(filepath.Join(p.Dir, "state.json"), s)
	}, false)
}

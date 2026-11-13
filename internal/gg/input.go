package gg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Sidebar processes are shared by clients. Native tmux action bindings record
// the originating client before forwarding the key, including keys in searches.
// A FIFO rather than a last-client option preserves separate queued keypresses.
func (p *Project) inputQueue(pid int, fn func(*[]string) error) error {
	dir := filepath.Join(p.Dir, "inputs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, strconv.Itoa(pid))
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	var queue []string
	data, err := os.ReadFile(path + ".json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &queue); err != nil {
			return err
		}
	}
	if err := fn(&queue); err != nil {
		return err
	}
	if len(queue) == 0 {
		_ = os.Remove(path + ".json")
		return nil
	}
	return atomicJSON(path+".json", queue)
}

func boundInputKey(key string) bool {
	switch strings.ToLower(key) {
	case "q", "n", "t", "s", "y", "b", "r", "enter":
		return true
	}
	return false
}
func (p *Project) forwardQuitKey(pane, client, key string, reader int) error {
	if !boundInputKey(key) {
		return errors.New("invalid forwarded key")
	}
	valid := false
	for _, x := range p.snapshot() {
		if x.ID == pane && x.Sidebar && x.PID == reader {
			valid = true
		}
	}
	if !valid {
		return nil
	}
	return p.inputQueue(reader, func(queue *[]string) error {
		// Persist before injecting the key. The reader blocks on this lock until the
		// key has been sent, so another client's metadata cannot overtake it.
		*queue = append(*queue, client)
		path := filepath.Join(p.Dir, "inputs", strconv.Itoa(reader)+".json")
		if err := atomicJSON(path, *queue); err != nil {
			return err
		}
		args := []string{"send-keys", "-l", "-t", pane, key}
		if key == "Enter" {
			args = []string{"send-keys", "-t", pane, "Enter"}
		}
		_, err := p.tmux(args...)
		if err != nil {
			*queue = (*queue)[:len(*queue)-1]
			_ = atomicJSON(path, *queue)
		}
		return err
	})
}

func (p *Project) takeInputClient() (string, error) {
	client := ""
	err := p.inputQueue(os.Getpid(), func(queue *[]string) error {
		if len(*queue) > 0 {
			client, *queue = (*queue)[0], (*queue)[1:]
		}
		return nil
	})
	return client, err
}

func (p *Project) takeMouseClient(pane string) (string, error) {
	if pane == "" {
		return "", nil
	}
	queue, err := p.tmux("show-options", "-p", "-v", "-t", pane, "@gg_mouse_clients")
	if err != nil || queue == "" {
		client, err := p.tmux("show-options", "-p", "-v", "-t", pane, "@gg_last_mouse_client")
		if err != nil {
			return "", nil
		}
		return client, nil
	}
	client := strings.SplitN(queue, ",", 2)[0]
	// The substitution runs in tmux against the current value, preserving clicks
	// appended after our read. There is one reader per sidebar pane.
	_, err = p.tmux("set-option", "-p", "-F", "-t", pane, "@gg_mouse_clients", "#{s/^[^,]*,//:@gg_mouse_clients}")
	return client, err
}

func (p *Project) mouseActionClient(queued string) (string, error) {
	clients, err := p.dashboardClients()
	if err != nil {
		return "", err
	}
	for _, client := range clients {
		if client.identity == queued {
			return queued, nil
		}
	}
	if len(clients) == 1 {
		return clients[0].identity, nil
	}
	if len(clients) == 0 {
		return "", nil
	}
	return "", errors.New(p.text("cannot identify dashboard mouse client", "не удалось определить клиента dashboard для мыши"))
}

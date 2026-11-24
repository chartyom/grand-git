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
	"syscall"
)

type terminalLaunch struct {
	Task                                    int `json:"task"`
	Branch, Path, Template, Shell, Language string
	Commands                                []string
}

func (p *Project) terminalEnv(t *Task, branch string) []string {
	return []string{"-e", "GG_BRANCH=" + branch, "-e", "GG_WORKTREE=" + t.Path, "-e", "GG_PROJECT_ROOT=" + p.Root}
}

func (p *Project) launchPath(token string) string {
	return filepath.Join(p.Dir, "launches", token+".json")
}

func (p *Project) prepareLaunch(t *Task, branch string) (string, error) {
	tpl, err := p.resolveTemplate(t.templateName())
	if err != nil {
		return "", err
	}
	if err = validateTemplateShell(tpl); err != nil {
		return "", err
	}
	// Plain shells retain their native direct startup and immediate input path.
	// Only configured commands need the post-commit worker.
	if len(tpl.Commands) == 0 {
		t.LaunchToken = ""
		return quote(shell()), nil
	}
	cfg, err := p.settings()
	if err != nil {
		return "", err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", err
	}
	t.LaunchToken = hex.EncodeToString(random[:])
	if err = os.MkdirAll(filepath.Join(p.Dir, "launches"), 0700); err != nil {
		return "", err
	}
	payload := terminalLaunch{Task: t.ID, Branch: branch, Path: t.Path, Template: tpl.Name, Shell: shell(), Language: cfg.Language, Commands: tpl.Commands}
	if err = atomicJSON(p.launchPath(t.LaunchToken), payload); err != nil {
		return "", err
	}
	return p.invocation("_terminal", "--root", p.Root, "--task", strconv.Itoa(t.ID), "--token", t.LaunchToken), nil
}

// No startup script runs until the creating/starting transaction has committed.
// The token prevents a delayed worker for an old launch from starting a new task.
func (p *Project) runTerminal(id int, token string) error {
	if len(token) != 32 {
		return errors.New("invalid launch token")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return err
	}
	path := p.launchPath(token)
	defer os.Remove(path)
	var launch terminalLaunch
	err := p.transaction(func(s *State) error {
		t, err := taskByID(s, id)
		if err != nil {
			return err
		}
		if t.LaunchToken != token {
			return errors.New("terminal launch was not committed")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &launch); err != nil {
			return err
		}
		if launch.Task != id || launch.Path != t.Path || launch.Template != t.templateName() {
			return errors.New("terminal launch no longer matches the session")
		}
		return p.validateWorktree(t)
	}, false)
	if err != nil {
		return err
	}
	_ = os.Remove(path) // syscall.Exec does not run defers.
	if err = os.Chdir(launch.Path); err != nil {
		return err
	}
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GG_BRANCH=") && !strings.HasPrefix(entry, "GG_WORKTREE=") && !strings.HasPrefix(entry, "GG_PROJECT_ROOT=") {
			env = append(env, entry)
		}
	}
	env = append(env, "GG_BRANCH="+launch.Branch, "GG_WORKTREE="+launch.Path, "GG_PROJECT_ROOT="+p.Root)
	args := []string{launch.Shell, "-i"}
	if len(launch.Commands) > 0 {
		args = append(args, "-c", startupScript(launch))
	}
	return syscall.Exec(launch.Shell, args, env)
}

func startupScript(launch terminalLaunch) string {
	// Caught (not ignored) INT remains interruptible in foreground children.
	// Commands share shell state, with an explicit check for each entry's result.
	var script strings.Builder
	script.WriteString("trap ':' INT\n_gg_startup() {\n")
	for _, c := range launch.Commands {
		script.WriteString("eval " + quote(c) + " || return $?\n")
	}
	script.WriteString("}\n_gg_startup\n_gg_exit_code=$?\ntrap - INT\n")
	message := (Settings{Language: launch.Language}).text("Grand Git: startup exited with status %s\\n", "Grand Git: запуск завершился с кодом %s\\n")
	fmt.Fprintf(&script, "printf %s \"$_gg_exit_code\"\n", quote(message))
	script.WriteString("exec " + quote(launch.Shell) + " -i\n")
	return script.String()
}

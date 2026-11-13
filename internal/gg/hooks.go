package gg

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var hookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PermissionRequest", "Stop", "Interrupt", "SessionEnd"}
var hookFiles = []string{".codex/hooks.json", ".codex/config.toml"}

type AgentState struct {
	SessionID string    `json:"session_id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func hookHashes(path string) map[string]string {
	m := map[string]string{}
	for _, file := range hookFiles {
		if hash := fileHash(filepath.Join(path, file)); hash != "" {
			m[file] = hash
		}
	}
	return m
}

func (p *Project) writeHookFile(path string, data []byte) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if string(old) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if len(old) > 0 {
		dir := filepath.Join(p.Dir, "hook-backups")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		name := fmt.Sprintf("%x-%d-%s", sha256.Sum256([]byte(path)), time.Now().UnixNano(), filepath.Base(path))
		if err := os.WriteFile(filepath.Join(dir, name), old, 0600); err != nil {
			return err
		}
	}
	// Rename rather than follow an existing symlink and modify another project.
	f, err := os.CreateTemp(filepath.Dir(path), ".gg-hook-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (p *Project) installHooks(path string) error {
	// Don't follow a shared .codex directory: integration must stay in this worktree.
	if info, err := os.Lstat(filepath.Join(path, ".codex")); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s/.codex is a symlink; local installation cancelled", path)
	}
	hookPath := filepath.Join(path, ".codex", "hooks.json")
	doc := map[string]any{}
	if b, err := os.ReadFile(hookPath); err == nil {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: %w", hookPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if doc == nil {
		return errors.New("hooks.json must contain a JSON object, not null")
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		if doc["hooks"] != nil {
			return errors.New("hooks must be a JSON object")
		}
		hooks = map[string]any{}
	}
	for _, event := range hookEvents {
		groups, ok := hooks[event].([]any)
		if !ok && hooks[event] != nil {
			return fmt.Errorf("invalid hooks structure: %s", event)
		}
		kept := []any{}
		for _, raw := range groups {
			group, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid hooks group: %s", event)
			}
			handlers, ok := group["hooks"].([]any)
			if !ok {
				return fmt.Errorf("invalid hooks handlers: %s", event)
			}
			remaining := []any{}
			for _, handler := range handlers {
				h, ok := handler.(map[string]any)
				if !ok || h["statusMessage"] != "grand-git" {
					remaining = append(remaining, handler)
				}
			}
			if len(remaining) > 0 {
				group["hooks"] = remaining
				kept = append(kept, group)
			}
		}
		kept = append(kept, map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": p.invocation("_hook", "--event", event), "statusMessage": "grand-git",
		}}})
		hooks[event] = kept
	}
	doc["hooks"] = hooks
	hookData, _ := json.MarshalIndent(doc, "", "  ")
	configPath := filepath.Join(path, ".codex", "config.toml")
	config := map[string]any{}
	configData, readErr := os.ReadFile(configPath)
	if readErr == nil {
		if err := toml.Unmarshal(configData, &config); err != nil {
			return fmt.Errorf("%s: %w", configPath, err)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	features, ok := config["features"].(map[string]any)
	if !ok {
		if config["features"] != nil {
			return errors.New("features must be a TOML table")
		}
		features = map[string]any{}
	}
	if features["hooks"] != true {
		features["hooks"] = true
		config["features"] = features
		var err error
		configData, err = toml.Marshal(config)
		if err != nil {
			return err
		}
	}
	if err := p.writeHookFile(configPath, configData); err != nil {
		return err
	}
	return p.writeHookFile(hookPath, append(hookData, '\n'))
}

func (p *Project) Init() error {
	return p.locked(func(s *State) error {
		calling, err := command(".", "git", "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		paths := []string{p.Root}
		seen := map[string]bool{p.Root: true}
		if calling != p.Root {
			paths = append(paths, calling)
			seen[calling] = true
		}
		for _, task := range s.Tasks {
			if task.plain() {
				continue
			}
			if !seen[task.Path] {
				paths = append(paths, task.Path)
				seen[task.Path] = true
			}
		}
		for _, path := range paths {
			before := hookHashes(path)
			if err := p.installHooks(path); err != nil {
				return err
			}
			for _, task := range s.Tasks {
				if task.plain() || task.Path != path {
					continue
				}
				if task.Generated == nil {
					task.Generated = map[string]string{}
				}
				for file, hash := range hookHashes(path) {
					if before[file] == "" || (task.Generated[file] != "" && task.Generated[file] == before[file]) {
						task.Generated[file] = hash
					}
				}
			}
		}
		s.HooksEnabled = true
		return nil
	})
}

func (p *Project) agentPath(id int) string {
	return filepath.Join(p.Dir, "agent-"+strconv.Itoa(id)+".json")
}

func (p *Project) hook(event string, input io.Reader) error {
	var payload struct {
		Cwd       string `json:"cwd"`
		SessionID string `json:"session_id"`
		AgentID   string `json:"agent_id"`
	}
	if err := json.NewDecoder(io.LimitReader(input, 2<<20)).Decode(&payload); err != nil {
		return err
	}
	if payload.Cwd == "" || payload.SessionID == "" || payload.AgentID != "" {
		return nil
	}
	cwd, err := filepath.EvalSymlinks(payload.Cwd)
	if err != nil {
		return nil
	}
	statuses := map[string]string{"SessionStart": "ready", "UserPromptSubmit": "working", "PreToolUse": "working", "PostToolUse": "working", "PermissionRequest": "waiting", "Stop": "done", "Interrupt": "interrupted", "SessionEnd": "shell"}
	status, ok := statuses[event]
	if !ok {
		return nil
	}
	return p.locked(func(s *State) error {
		var matched *Task
		for _, task := range s.Tasks {
			rel, err := filepath.Rel(task.Path, cwd)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				continue
			}
			if matched == nil || len(task.Path) > len(matched.Path) {
				matched = task
			}
		}
		if matched != nil {
			previous := p.agent(matched.ID)
			if event != "SessionStart" && previous.SessionID != "" && previous.SessionID != payload.SessionID {
				return nil
			}
			return atomicJSON(p.agentPath(matched.ID), &AgentState{SessionID: payload.SessionID, Status: status, UpdatedAt: time.Now().UTC()})
		}
		return nil
	})
}

func (p *Project) agent(id int) AgentState {
	var state AgentState
	if b, err := os.ReadFile(p.agentPath(id)); err == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}

func (p *Project) status(t *Task, snap Snapshot) string {
	panes := snap.task(t.ID).terminals()
	live, nonShell := false, false
	for _, pane := range panes {
		if pane.Dead {
			continue
		}
		live = true
		switch pane.Command {
		case "sh", "bash", "zsh", "fish", "dash", "nu":
		default:
			nonShell = true
		}
	}
	if !live {
		return "stopped"
	}
	a := p.agent(t.ID)
	if a.Status != "" && nonShell {
		return statusCode(a.Status)
	}
	if nonShell {
		return "unknown"
	}
	return "shell"
}

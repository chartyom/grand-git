package gg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func TestFormRequiresBranchAndCanCancel(t *testing.T) {
	m := newCreateForm()
	next, _ := m.Update(key(tea.KeyEnter))
	m = next.(createForm)
	if m.done || m.message == "" {
		t.Fatal("empty branch was accepted")
	}
	next, _ = m.Update(key(tea.KeyEscape))
	if !next.(createForm).cancelled {
		t.Fatal("Escape did not cancel")
	}
}

func TestFormOnlyPromptsRequiredBranch(t *testing.T) {
	m := newCreateForm()
	if len(m.inputs) != 1 || len(m.labels) != 1 || m.labels[0] != "Branch name" {
		t.Fatal("form must prompt only for the required branch")
	}
	if strings.Contains(m.View().Content, "Description") {
		t.Fatal("optional description is being prompted")
	}
	m.inputs[0].SetValue("feature/login")
	next, _ := m.Update(key(tea.KeyEnter))
	if !next.(createForm).done {
		t.Fatal("branch submission should finish the form immediately")
	}
}

func TestSidebarDeleteIsContextualAndConfirmationIsCancelable(t *testing.T) {
	p := &Project{Dir: t.TempDir()}
	m := sidebar{p: p, width: 32, height: 30, state: &State{}, active: true}
	if strings.Contains(m.View().Content, "Delete") {
		t.Fatal("delete visible without a task")
	}
	next, _ := m.Update(key('d'))
	if next.(sidebar).confirming {
		t.Fatal("delete enabled without a task")
	}
	m.state.Tasks = []*Task{{ID: 7, Branch: "login"}}
	view, hits := m.render()
	if !strings.Contains(view, "[D] Delete") {
		t.Fatal("delete is missing from the footer")
	}
	found := false
	for _, h := range hits {
		if h.key == "D" {
			found = true
			clickModel := m
			clickModel.popup = true
			clicked, _ := clickModel.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: h.x1, Y: h.y})
			if !clicked.(sidebar).confirming {
				t.Fatal("delete footer click did not open confirmation")
			}
		}
	}
	if !found {
		t.Fatal("delete has no mouse target")
	}
	next, _ = m.Update(key('d'))
	m = next.(sidebar)
	if !m.confirming || !strings.Contains(m.View().Content, "Delete login?") {
		t.Fatal("no confirmation")
	}
	next, _ = m.Update(key(tea.KeyEscape))
	if next.(sidebar).confirming {
		t.Fatal("Escape did not cancel confirmation")
	}
}

func TestQuotesAreSafeForShellArguments(t *testing.T) {
	value := "one ' two $(echo unsafe) `echo unsafe`"
	out, err := command(t.TempDir(), "/bin/sh", "-c", "printf '%s' "+quote(value))
	if err != nil || out != value {
		t.Fatalf("quoting failed: %q %v", out, err)
	}
}

func TestDeletionConfirmationCannotMoveToAnotherTask(t *testing.T) {
	m := sidebar{p: &Project{Dir: t.TempDir()}, state: &State{Tasks: []*Task{{ID: 1, Branch: "first"}}}}
	next, _ := m.Update(key('d'))
	m = next.(sidebar)
	m.state.Tasks = []*Task{{ID: 2, Branch: "second"}}
	next, cmd := m.Update(key('y'))
	if cmd != nil || next.(sidebar).confirming {
		t.Fatal("confirmation followed a different task")
	}
}

func TestOriginIsVisibleButCannotBeDeletedFromSidebar(t *testing.T) {
	m := sidebar{p: &Project{Dir: t.TempDir()}, width: 32, height: 30,
		state: &State{Tasks: []*Task{{ID: 0, Branch: "main", Protected: true}}}, active: true}
	view := m.View().Content
	if !strings.Contains(view, "main") || strings.Contains(view, "Delete") {
		t.Fatal("origin should be visible and protected")
	}
	next, _ := m.Update(key('d'))
	if next.(sidebar).confirming {
		t.Fatal("protected origin entered deletion confirmation")
	}
}

func TestStatusDoesNotConfuseTerminalAndAgent(t *testing.T) {
	p := &Project{Dir: t.TempDir()}
	task := &Task{ID: 1}
	if got := p.status(task, nil); got != "stopped" {
		t.Fatal(got)
	}
	snap := Snapshot{{ID: "%1", Task: "1", Command: "zsh"}}
	if got := p.status(task, snap); got != "shell" {
		t.Fatal(got)
	}
	snap[0].Command = "codex"
	if got := p.status(task, snap); got != "unknown" {
		t.Fatal(got)
	}
	if err := atomicJSON(p.agentPath(1), AgentState{Status: "ждёт разрешения"}); err != nil {
		t.Fatal(err)
	}
	if got := p.status(task, snap); got != "waiting" {
		t.Fatal(got)
	}
	snap[0].Dead = true
	if got := p.status(task, snap); got != "stopped" {
		t.Fatal(got)
	}
}

func TestCorruptRegistryIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	p := &Project{Dir: dir}
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.locked(func(s *State) error { s.NextID = 999; return nil }); err == nil {
		t.Fatal("corrupt registry accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "not json" {
		t.Fatal("registry overwritten")
	}
}

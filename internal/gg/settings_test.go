package gg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "grand-git-unit-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	_ = os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestConfigurationLayersResetAndAtomicValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir(), Socket: "grand-git-unit-no-server"}
	if s, err := p.settings(); err != nil || s != defaults() {
		t.Fatalf("%+v %v", s, err)
	}
	if err := p.setSetting("language", "ru", false, false); err != nil {
		t.Fatal(err)
	}
	if err := p.setSetting("default_template", "codex", true, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := p.settings(); s.Language != "ru" || s.DefaultTemplate != "codex" {
		t.Fatal(s)
	}
	if err := p.setSetting("language", "en", true, false); err != nil {
		t.Fatal(err)
	}
	if s, _ := p.settings(); s.Language != "en" {
		t.Fatal(s)
	}
	before, _ := os.ReadFile(p.configPath())
	if err := p.setSetting("navigation", "invalid", true, false); err == nil {
		t.Fatal("invalid setting accepted")
	}
	after, _ := os.ReadFile(p.configPath())
	if string(before) != string(after) {
		t.Fatal("invalid input changed settings")
	}
	if err := p.setSetting("language", "", true, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := p.settings(); s.Language != "ru" || s.DefaultTemplate != "codex" {
		t.Fatal(s)
	}
	if err := p.saveSettings(Settings{Language: "en", DefaultTemplate: "shell"}, true); err != nil {
		t.Fatal(err)
	}
	if s, _ := p.settings(); s.Language != "en" || s.DefaultTemplate != "shell" {
		t.Fatal(s)
	}
}

func TestMalformedConfigurationIsPreserved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir(), Socket: "grand-git-unit-no-server"}
	if err := os.MkdirAll(filepath.Dir(p.configPath()), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("language = [")
	if err := os.WriteFile(p.configPath(), content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.settings(); err == nil {
		t.Fatal("invalid config accepted")
	}
	if err := p.setSetting("language", "ru", true, false); err == nil {
		t.Fatal("corrupt config overwritten")
	}
	after, _ := os.ReadFile(p.configPath())
	if string(after) != string(content) {
		t.Fatal("config changed")
	}
}

func TestLegacyStatusAndFilteredSessionList(t *testing.T) {
	p := &Project{Dir: t.TempDir()}
	m := newSidebar(p, 1)
	m.state = &State{Tasks: []*Task{{ID: 0, Branch: "main", Protected: true}, {ID: 1, Branch: "feat/login"}, {ID: 2, Branch: "fix/logout"}}}
	m.snap = Snapshot{{ID: "%1", Task: "1", Command: "sh"}}
	m.width, m.height = 60, 40
	for i := 3; i < 12; i++ {
		m.state.Tasks = append(m.state.Tasks, &Task{ID: i, Branch: fmt.Sprintf("extra-%d", i)})
	}
	m.query = "LOGIN"
	m.setState(m.state)
	if len(m.rows()) != 1 || m.selectedTask().Branch != "feat/login" {
		t.Fatal("branch search failed")
	}
	m.query = ""
	m.onlyActive = true
	m.setState(m.state)
	if len(m.rows()) != 1 || m.rows()[0].ID != 1 {
		t.Fatal("activity filter used focus instead of live terminals")
	}
	view := m.View().Content
	if !strings.Contains(view, "●") || strings.Contains(view, "#1") || strings.Contains(view, "description") {
		t.Fatal(view)
	}
	if statusCode("ждёт разрешения") != "waiting" {
		t.Fatal("legacy agent status lost")
	}
}

func TestLayoutPreservesTerminalTreeWhenSidebarIsRemoved(t *testing.T) {
	parser := layoutParser{s: "120x30,0,0{30x30,0,0,9,89x30,31,0[89x14,31,0,1,89x15,31,15{44x15,31,15,2,44x15,76,15,3}]}"}
	tree, err := parser.node()
	if err != nil {
		t.Fatal(err)
	}
	tree = tree.remove(9)
	tree.fit(119, 35, 41, 0)
	layout := tree.String()
	reparsed := layoutParser{s: layout}
	again, err := reparsed.node()
	if err != nil {
		t.Fatal(err)
	}
	if again.kind != '[' || again.children[1].kind != '{' || again.children[0].pane != 1 || again.children[1].children[0].pane != 2 || again.children[1].children[1].pane != 3 {
		t.Fatal(layout)
	}
	if strings.Contains(layout, ",9") {
		t.Fatal("sidebar still in terminal layout")
	}
}

func TestManagedPathsSupportLegacyAndPreventTraversal(t *testing.T) {
	p := &Project{Root: filepath.Join(t.TempDir(), "repo")}
	if !p.validManagedPath(&Task{ID: 1, Branch: "feat/login", Path: p.Root + "__sessions/1"}) {
		t.Fatal("legacy path rejected")
	}
	if !p.validManagedPath(&Task{ID: 2, Branch: "feat/login", Path: p.Root + "__sessions/feat/login"}) {
		t.Fatal("named path rejected")
	}
	if p.validManagedPath(&Task{ID: 3, Branch: "../../outside", Path: filepath.Clean(p.Root + "__sessions/../../outside")}) {
		t.Fatal("path traversal accepted")
	}
}

package gg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestYAMLLegacyLayerMigrationAndReset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir()}
	globalLegacy := strings.TrimSuffix(userConfigPath(), ".yaml") + ".toml"
	localLegacy := filepath.Join(p.Dir, "config.toml")
	writeFixture(t, globalLegacy, "language = 'ru'\nnavigation = 'alt-hjkl'\n")
	writeFixture(t, localLegacy, "language = 'en'\n")
	if s, err := p.settings(); err != nil || s.Language != "en" || s.Navigation != "" {
		t.Fatalf("%+v %v", s, err)
	}
	if err := p.setSetting("default_template", "codex", false, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(userConfigPath()); err != nil || strings.Contains(string(b), "navigation") {
		t.Fatalf("obsolete navigation was retained: %s %v", b, err)
	}
	if err := p.setSetting("default_template", "claude", true, false); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, globalLegacy, "language = 'en'\n")
	writeFixture(t, localLegacy, "navigation = 'ctrl-arrows'\n")
	if err := p.setSetting("language", "", true, true); err != nil {
		t.Fatal(err)
	}
	if s, err := p.settings(); err != nil || s.Language != "ru" || s.Navigation != "" || s.DefaultTemplate != "claude" {
		t.Fatalf("%+v %v", s, err)
	}
	if err := p.setSetting("default_template", "", true, true); err != nil {
		t.Fatal(err)
	}
	if s, err := p.settings(); err != nil || s.DefaultTemplate != "codex" || s.Navigation != "" {
		t.Fatalf("%+v %v", s, err)
	}
	b, _ := os.ReadFile(p.configPath())
	if strings.TrimSpace(string(b)) != "{}" {
		t.Fatalf("reset must retain an empty YAML layer: %s", b)
	}
}

func TestDeprecatedNavigationYAMLIsDroppedOnWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir()}
	writeFixture(t, p.configPath(), "language: ru\nnavigation: alt-arrows\n")
	if s, err := p.settings(); err != nil || s.Language != "ru" || s.Navigation != "" {
		t.Fatalf("%+v %v", s, err)
	}
	if err := p.setSetting("default_template", "codex", true, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p.configPath()); err != nil || strings.Contains(string(b), "navigation") || !strings.Contains(string(b), "language: ru") {
		t.Fatalf("obsolete setting survived migration: %s %v", b, err)
	}
}

func TestStrictYAMLRejectsBadSettingsAndCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir()}
	for _, content := range []string{"language: en\nlanguage: ru", "navigation: []", "default_template: 12", "unknown: x", "language: en\n---\nlanguage: ru", "[]", "language: [", "default_template: ../outside", "language: &x en\nnavigation: *x"} {
		writeFixture(t, p.configPath(), content)
		if _, err := p.settings(); err == nil {
			t.Errorf("accepted settings: %s", content)
		}
		if err := p.setSetting("language", "en", true, false); err == nil {
			t.Errorf("overwrote invalid settings: %s", content)
		}
		b, _ := os.ReadFile(p.configPath())
		if string(b) != content {
			t.Fatal("invalid YAML was modified")
		}
	}
	path := filepath.Join(p.Root, ".gg/templates/check.yaml")
	for _, content := range []string{"{}", "commands: run", "commands: [12]", "commands: [null]", "commands: ['']", "commands: []\nextra: x", "commands: []\ncommands: []", "commands: []\n---\ncommands: []", "commands: [*missing]"} {
		writeFixture(t, path, content)
		if _, err := p.resolveTemplate("check"); err == nil {
			t.Errorf("accepted template: %s", content)
		}
	}
}

func TestTemplateLayersReserveShellAndUseWholeDefinitions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &Project{Root: t.TempDir(), Dir: t.TempDir()}
	dirs := p.templateDirs()
	writeFixture(t, filepath.Join(dirs[0], "codex.yaml"), "commands: ['echo global', 'echo second']")
	writeFixture(t, filepath.Join(dirs[1], "codex.yaml"), "commands: ['echo local']")
	tpl, err := p.resolveTemplate("codex")
	if err != nil || len(tpl.Commands) != 1 || tpl.Commands[0] != "echo local" {
		t.Fatalf("%+v %v", tpl, err)
	}
	writeFixture(t, filepath.Join(dirs[1], "shell.yaml"), "commands: ['echo forbidden']")
	tpl, err = p.resolveTemplate("shell")
	if err != nil || len(tpl.Commands) != 0 || tpl.Source != "built-in" {
		t.Fatalf("%+v %v", tpl, err)
	}
	if _, err = p.templates(); err == nil {
		t.Fatal("reserved override should be reported")
	}
	for _, name := range []string{"../x", "", "x/y", "x.yaml", "x;echo"} {
		if _, err = p.resolveTemplate(name); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
}

func TestCreateTemplatePickerIsOptionalAndCancelable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newCreateForm()
	m.inputs[0].SetValue("feat/picker")
	next, _ := m.Update(key('\t'))
	m = next.(createForm)
	if !m.picking {
		t.Fatal("Tab did not open picker")
	}
	next, _ = m.Update(key('k'))
	m = next.(createForm)
	next, _ = m.Update(key(27))
	m = next.(createForm)
	if m.picking || m.template != "shell" || m.cancelled {
		t.Fatal("picker cancellation changed selection")
	}
	next, _ = m.Update(key(13))
	if !next.(createForm).done {
		t.Fatal("ordinary Enter should create without another question")
	}
}

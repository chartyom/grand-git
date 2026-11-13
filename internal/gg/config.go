package gg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
	"golang.org/x/sys/unix"
)

type Settings struct {
	Language        string `toml:"language" yaml:"language,omitempty" json:"language"`
	Navigation      string `toml:"navigation" yaml:"-" json:"-"`
	DefaultTemplate string `toml:"default_template" yaml:"default_template,omitempty" json:"default_template"`
}

func defaults() Settings {
	return Settings{Language: "en", DefaultTemplate: "shell"}
}

func userConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" || !filepath.IsAbs(base) {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "grand-git", "config.yaml")
}

func (p *Project) configPath() string { return filepath.Join(p.Root, ".gg", "config.yaml") }

var settingKeys = []string{"language", "default_template"}

func (s Settings) values() map[string]string {
	return map[string]string{"language": s.Language, "default_template": s.DefaultTemplate}
}

// One mapping, string keys, no duplicate keys/aliases/extra YAML documents.
func yamlMapping(b []byte) (map[string]*yaml.Node, error) {
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	var doc yaml.Node
	if err := d.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	var extra yaml.Node
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("expected one YAML document")
	}
	out := map[string]*yaml.Node{}
	if len(doc.Content) == 0 {
		return out, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("expected a YAML mapping")
	}
	for i := 0; i < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
			return nil, errors.New("expected string keys")
		}
		if _, exists := out[k.Value]; exists {
			return nil, fmt.Errorf("duplicate key %q", k.Value)
		}
		out[k.Value] = v
	}
	return out, nil
}

func readSettings(path string, into *Settings) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	layer := Settings{}
	if filepath.Ext(path) == ".toml" {
		if err := toml.NewDecoder(strings.NewReader(string(b))).DisallowUnknownFields().Decode(&layer); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	} else {
		nodes, err := yamlMapping(b)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for key, n := range nodes {
			if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
				return fmt.Errorf("%s: %s must be a string", path, key)
			}
			if err := validateSetting(key, n.Value); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			switch key {
			case "language":
				layer.Language = n.Value
			case "navigation":
				layer.Navigation = n.Value
			case "default_template":
				layer.DefaultTemplate = n.Value
			}
		}
	}
	for key, v := range layer.values() {
		if v != "" {
			if err := validateSetting(key, v); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	if layer.Navigation != "" {
		if err := validateSetting("navigation", layer.Navigation); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if layer.Language != "" {
		into.Language = layer.Language
	}
	if layer.Navigation != "" {
		into.Navigation = layer.Navigation
	}
	if layer.DefaultTemplate != "" {
		into.DefaultTemplate = layer.DefaultTemplate
	}
	return nil
}

func (p *Project) settingsLayer(local bool) (Settings, string, error) {
	path := userConfigPath()
	legacy := strings.TrimSuffix(path, ".yaml") + ".toml"
	if local {
		if p == nil || p.Root == "" || p.Directory {
			return Settings{}, "", nil
		}
		path, legacy = p.configPath(), filepath.Join(p.Dir, "config.toml")
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		path = legacy
	}
	s := Settings{}
	err := readSettings(path, &s)
	return s, path, err
}

func validateSetting(key, value string) error {
	switch key {
	case "language":
		if value == "en" || value == "ru" {
			return nil
		}
	case "navigation":
		if value == "ctrl-arrows" || value == "alt-arrows" || value == "alt-hjkl" {
			return nil
		}
	case "default_template":
		return validateTemplateName(value)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return fmt.Errorf("invalid value %q for %s", value, key)
}

func (p *Project) settings() (Settings, error) {
	s := defaults()
	for _, local := range []bool{false, true} {
		layer, _, err := p.settingsLayer(local)
		if err != nil {
			return s, err
		}
		if layer.Language != "" {
			s.Language = layer.Language
		}
		if layer.DefaultTemplate != "" {
			s.DefaultTemplate = layer.DefaultTemplate
		}
	}
	return s, nil
}

// Update one layer under a separate lock; registry transactions are independent.
func (p *Project) setSetting(key, value string, project, reset bool) error {
	if key == "navigation" {
		return fmt.Errorf("unknown setting %q", key)
	}
	if reset {
		if key != "language" && key != "default_template" {
			return fmt.Errorf("unknown setting %q", key)
		}
	} else if err := validateSetting(key, value); err != nil {
		return err
	}
	updates := map[string]string{key: value}
	return p.writeSettings(updates, project, reset)
}

func (p *Project) saveSettings(s Settings, project bool) error {
	for key, value := range s.values() {
		if err := validateSetting(key, value); err != nil {
			return err
		}
	}
	return p.writeSettings(s.values(), project, false)
}

func (p *Project) writeSettings(updates map[string]string, project, reset bool) error {
	path := userConfigPath()
	if project {
		if p == nil || p.Root == "" || p.Directory {
			return errors.New("--local requires a Git repository")
		}
		path = p.configPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lockPath := path + ".lock"
	if project {
		if err := os.MkdirAll(p.Dir, 0700); err != nil {
			return err
		}
		lockPath = filepath.Join(p.Dir, "settings.lock")
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	layer, _, err := p.settingsLayer(project)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for key, value := range layer.values() {
		if value != "" {
			values[key] = value
		}
	}
	for key, value := range updates {
		if reset {
			delete(values, key)
		} else {
			values[key] = value
		}
	}
	b, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	if err := atomicBytes(path, b, 0600); err != nil {
		return err
	}
	if p != nil {
		return p.syncSettings()
	}
	return nil
}

func atomicBytes(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".grand-git-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (p *Project) text(en, ru string) string {
	s, _ := p.settings()
	if s.Language == "ru" {
		return ru
	}
	return en
}

func (s Settings) text(en, ru string) string {
	if s.Language == "ru" {
		return ru
	}
	return en
}

func statusCode(status string) string {
	legacy := map[string]string{"готов": "ready", "работает": "working", "ждёт разрешения": "waiting", "завершил ход": "done", "прерван": "interrupted", "оболочка": "shell", "остановлен": "stopped", "неизвестно": "unknown"}
	if code, ok := legacy[status]; ok {
		return code
	}
	return status
}

func (p *Project) statusLabel(status string) string {
	ru := map[string]string{"ready": "готов", "working": "работает", "waiting": "ждёт разрешения", "done": "завершил ход", "interrupted": "прерван", "shell": "оболочка", "stopped": "остановлен", "unknown": "неизвестно"}
	return p.text(status, ru[status])
}

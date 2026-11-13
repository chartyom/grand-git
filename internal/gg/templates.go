package gg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Template struct {
	Name     string   `json:"name" yaml:"-"`
	Source   string   `json:"source" yaml:"-"`
	Commands []string `json:"-" yaml:"commands"`
}

var templateNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateTemplateName(name string) error {
	if !templateNamePattern.MatchString(name) {
		return fmt.Errorf("invalid template name %q: use letters, numbers, _ or -", name)
	}
	return nil
}

func builtInTemplates() map[string][]string {
	return map[string][]string{"shell": {}, "codex": {"codex"}, "claude": {"claude"}, "npm-dev": {"npm run dev"}}
}

func (p *Project) templateDirs() []string {
	dirs := []string{filepath.Join(filepath.Dir(userConfigPath()), "templates")}
	if p != nil && !p.Directory {
		dirs = append(dirs, filepath.Join(p.Root, ".gg", "templates"))
	}
	return dirs
}

func readTemplate(path, name string) (Template, error) {
	t := Template{Name: name, Source: path}
	if name == "shell" {
		return t, errors.New("shell is reserved and cannot be overridden")
	}
	if err := validateTemplateName(name); err != nil {
		return t, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	fields, err := yamlMapping(b)
	if err != nil {
		return t, fmt.Errorf("%s: %w", path, err)
	}
	for key := range fields {
		if key != "commands" {
			return t, fmt.Errorf("%s: unknown template field %q", path, key)
		}
	}
	commands := fields["commands"]
	if commands == nil || commands.Kind != yaml.SequenceNode {
		return t, fmt.Errorf("%s: commands must be a sequence of strings", path)
	}
	t.Commands = []string{}
	for _, node := range commands.Content {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || strings.TrimSpace(node.Value) == "" || strings.ContainsRune(node.Value, '\x00') {
			return t, fmt.Errorf("%s: each command must be a nonempty string without NUL", path)
		}
		t.Commands = append(t.Commands, node.Value)
	}
	return t, nil
}

func (p *Project) resolveTemplate(name string) (Template, error) {
	if err := validateTemplateName(name); err != nil {
		return Template{}, err
	}
	if name == "shell" {
		return Template{Name: name, Source: "built-in", Commands: []string{}}, nil
	}
	var path string
	for _, dir := range p.templateDirs() {
		candidate := filepath.Join(dir, name+".yaml")
		if _, err := os.Lstat(candidate); err == nil {
			path = candidate
		} else if !errors.Is(err, os.ErrNotExist) {
			return Template{}, err
		}
	}
	if path != "" {
		return readTemplate(path, name)
	}
	if commands, ok := builtInTemplates()[name]; ok {
		return Template{Name: name, Source: "built-in", Commands: commands}, nil
	}
	return Template{}, fmt.Errorf(p.text("template %q not found; use gg template list or choose --template shell", "шаблон %q не найден; используйте gg template list или выберите --template shell"), name)
}

func (p *Project) templates() ([]Template, error) {
	names := map[string]bool{}
	for name := range builtInTemplates() {
		names[name] = true
	}
	for _, dir := range p.templateDirs() {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".yaml")
			if name == "shell" {
				return nil, fmt.Errorf("%s: shell is reserved and cannot be overridden", filepath.Join(dir, entry.Name()))
			}
			if err := validateTemplateName(name); err != nil {
				return nil, err
			}
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	out := make([]Template, 0, len(ordered))
	for _, name := range ordered {
		t, err := p.resolveTemplate(name)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (t *Task) templateName() string {
	if t.Template == "" {
		return "shell"
	}
	return t.Template
}

func validateTemplateShell(t Template) error {
	if len(t.Commands) == 0 {
		return nil
	}
	switch filepath.Base(shell()) {
	case "sh", "dash", "bash", "zsh":
		return nil
	}
	return fmt.Errorf("startup commands require sh, bash or zsh; current shell is %s; choose --template shell", shell())
}

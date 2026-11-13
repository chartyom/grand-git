package gg

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Set at release build time. Local builds remain recognizable as development builds.
var Version = "dev"
var Commit = "unknown"
var BuildDate = "unknown"

var dependencyVersion = regexp.MustCompile(`(?:^|[^0-9])([0-9]+)\.([0-9]+)`)

func versionAtLeast(value, minimum string) bool {
	parse := func(s string) (int, int, bool) {
		match := dependencyVersion.FindStringSubmatch(s)
		if match == nil {
			return 0, 0, false
		}
		major, e1 := strconv.Atoi(match[1])
		minor, e2 := strconv.Atoi(match[2])
		return major, minor, e1 == nil && e2 == nil
	}
	a, b, ok := parse(value)
	x, y, minOK := parse(minimum)
	return ok && minOK && (a > x || a == x && b >= y)
}

func dependencyInstallHint() string {
	if runtime.GOOS == "darwin" {
		return "brew install git tmux"
	}
	return "Ubuntu/WSL: sudo apt update && sudo apt install git tmux; Fedora: sudo dnf install git tmux; Arch: sudo pacman -S git tmux"
}

func checkDependency(name, minimum string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found; requires %s+. Install: %s", name, minimum, dependencyInstallHint())
	}
	flag := "--version"
	if name == "tmux" {
		flag = "-V"
	}
	out, err := exec.Command(path, flag).CombinedOutput()
	if err != nil || !versionAtLeast(string(out), minimum) {
		return fmt.Errorf("%s %s+ required (found %q). Install or upgrade: %s", name, minimum, strings.TrimSpace(string(out)), dependencyInstallHint())
	}
	return nil
}

package gg

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const uiRevision = "grand-git-v10"

type Pane struct {
	Generation                                          string
	ID, Window, Task, Command, Session                  string
	Sidebar, Dead, Active, Service                      bool
	X, Y, Width, Height, WindowWidth, WindowHeight, PID int
}
type Snapshot []Pane

func (p *Project) tmux(args ...string) (string, error) {
	// tmux expands formats in start-directory even when the shell argument is
	// quoted. Keep Git branch paths containing #{...} or #(...) literal.
	args = append([]string(nil), args...)
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c" || (len(args) > 0 && args[0] == "display-popup" && args[i] == "-d") {
			args[i+1] = strings.ReplaceAll(args[i+1], "#", "##")
			i++
		}
	}
	return command(p.Root, "tmux", append([]string{"-L", p.Socket}, args...)...)
}

func (p *Project) snapshot() Snapshot {
	panes, _ := p.checkedSnapshot()
	return panes
}

// Migration must not interpret a failed query as an empty server.
func (p *Project) checkedSnapshot() (Snapshot, error) {
	out, err := p.tmux("list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{@gg_task}\t#{@gg_sidebar}\t#{pane_dead}\t#{pane_active}\t#{pane_current_command}\t#{session_name}\t#{pane_left}\t#{pane_top}\t#{pane_width}\t#{pane_height}\t#{window_width}\t#{window_height}\t#{@gg_service}\t#{pane_pid}\t#{@gg_temporary}")
	if err != nil {
		message := err.Error()
		if strings.Contains(message, "no server running on ") ||
			(strings.Contains(message, "error connecting to ") &&
				(strings.Contains(message, "(No such file or directory)") || strings.Contains(message, "(Connection refused)"))) {
			return Snapshot{}, nil
		}
		return nil, err
	}
	var panes Snapshot
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		v := strings.Split(line, "\t")
		if len(v) != 17 {
			return nil, errors.New("invalid tmux pane snapshot")
		}
		if seen[v[0]] {
			continue
		}
		seen[v[0]] = true
		n := func(i int) int { x, _ := strconv.Atoi(v[i]); return x }
		panes = append(panes, Pane{ID: v[0], Window: v[1], Task: v[2], Sidebar: v[3] == "1", Dead: v[4] == "1", Active: v[5] == "1", Command: v[6], Session: v[7], X: n(8), Y: n(9), Width: n(10), Height: n(11), WindowWidth: n(12), WindowHeight: n(13), Service: v[14] == "1", PID: n(15), Generation: v[16]})
	}
	return panes, nil
}
func (s Snapshot) task(id int) Snapshot {
	var out Snapshot
	for _, x := range s {
		if x.Task == strconv.Itoa(id) {
			out = append(out, x)
		}
	}
	return out
}
func (s Snapshot) terminals() Snapshot {
	var out Snapshot
	for _, x := range s {
		if !x.Sidebar && !x.Service && x.Task != "-1" {
			out = append(out, x)
		}
	}
	return out
}
func (s Snapshot) live() Snapshot {
	var out Snapshot
	for _, x := range s.terminals() {
		if !x.Dead {
			out = append(out, x)
		}
	}
	return out
}
func (s Snapshot) sidebar() string {
	for _, x := range s {
		if x.Sidebar && x.Session != "_gg-park" {
			return x.ID
		}
	}
	return ""
}
func shell() string {
	if candidate := os.Getenv("SHELL"); candidate != "" {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return "/bin/sh"
}

func (p *Project) ensureServer() error {
	if err := checkDependency("tmux", "3.4"); err != nil {
		return err
	}
	if _, err := p.tmux("has-session", "-t", "=dashboard"); err == nil {
		if err = p.ensureControl(); err != nil {
			return err
		}
		if err = p.upgradePanels(); err != nil {
			return err
		}
		return p.syncSettings()
	}
	if _, err := p.tmux("-f", "/dev/null", "new-session", "-d", "-s", "dashboard", "-n", "Grand Git", "-x", "120", "-y", "32", "-c", p.Root, p.invocation("_sidebar", "--root", p.Root, "--task", "-1")); err != nil {
		return err
	}
	pane, err := p.tmux("display-message", "-p", "-t", "dashboard:0", "#{pane_id}")
	if err != nil {
		return err
	}
	for _, args := range [][]string{
		{"set-option", "-g", "prefix", "None"}, {"set-option", "-g", "prefix2", "None"},
		{"set-option", "-g", "status", "on"}, {"set-option", "-g", "status-interval", "1"},
		{"set-option", "-g", "window-status-format", ""}, {"set-option", "-g", "window-status-current-format", ""},
		{"set-option", "-g", "set-titles", "on"}, {"set-option", "-g", "set-titles-string", "Grand Git · #W"},
		{"set-option", "-g", "destroy-unattached", "off"}, {"set-option", "-g", "exit-empty", "on"},
		{"set-option", "-g", "history-limit", "10000"}, {"set-window-option", "-g", "remain-on-exit", "on"},
		{"set-window-option", "-g", "automatic-rename", "off"}, {"set-window-option", "-g", "allow-rename", "off"},
		{"set-window-option", "-g", "pane-border-status", "top"}, {"set-window-option", "-g", "pane-border-format", " #{pane_title} "},
		{"set-window-option", "-g", "window-size", "latest"},
		{"set-option", "-p", "-t", pane, "@gg_sidebar", "1"}, {"set-option", "-p", "-t", pane, "@gg_task", "-1"},
		{"set-option", "-p", "-t", pane, "@gg_ui_revision", uiRevision},
	} {
		if _, err = p.tmux(args...); err != nil {
			p.tmux("kill-session", "-t", "=dashboard")
			return err
		}
	}
	return p.syncSettings()
}

func (p *Project) controlPane() string {
	for _, x := range p.snapshot() {
		if x.Task == "-1" && x.Sidebar && x.Session == "dashboard" {
			return x.ID
		}
	}
	return ""
}
func (p *Project) ensureControl() error {
	if p.controlPane() != "" {
		return nil
	}
	pane, err := p.tmux("new-window", "-d", "-t", "dashboard:", "-n", "Grand Git", "-c", p.Root, "-P", "-F", "#{pane_id}", p.invocation("_sidebar", "--root", p.Root, "--task", "-1"))
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{{"@gg_sidebar", "1"}, {"@gg_task", "-1"}, {"@gg_ui_revision", uiRevision}} {
		if _, err = p.tmux("set-option", "-p", "-t", pane, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

func (p *Project) upgradePanels() error {
	state, err := p.load()
	if err != nil {
		return err
	}
	checkedWindows := map[string]bool{}
	for _, pane := range p.snapshot() {
		if pane.Sidebar {
			revision, _ := p.tmux("show-options", "-p", "-v", "-t", pane.ID, "@gg_ui_revision")
			if revision != uiRevision {
				if pane.ID == os.Getenv("TMUX_PANE") {
					continue
				}
				// Freeze identity on the pane so parking it in another window is safe.
				p.tmux("set-option", "-p", "-t", pane.ID, "@gg_task", pane.Task)
				if _, err = p.tmux("respawn-pane", "-k", "-t", pane.ID, "-c", p.Root, p.invocation("_sidebar", "--root", p.Root, "--task", pane.Task)); err != nil {
					return err
				}
				p.tmux("set-option", "-p", "-t", pane.ID, "@gg_ui_revision", uiRevision)
			}
		} else if !pane.Service {
			if t, e := taskByID(state, mustInt(pane.Task)); e == nil {
				label := oneLine(t.label())
				if !checkedWindows[pane.Window] {
					name, e := p.tmux("display-message", "-p", "-t", pane.Window, "#{window_name}")
					if e != nil {
						return e
					}
					if name != label {
						if _, err = p.tmux("rename-window", "-t", pane.Window, label); err != nil {
							return err
						}
					}
					checkedWindows[pane.Window] = true
				}
				applied, _ := p.tmux("show-options", "-p", "-v", "-t", pane.ID, "@gg_applied_label")
				if applied != label {
					if applied == "" {
						title, e := p.tmux("display-message", "-p", "-t", pane.ID, "#{pane_title}")
						if e != nil {
							return e
						}
						if title == label {
							_, err = p.tmux("set-option", "-p", "-t", pane.ID, "@gg_applied_label", label)
							if err != nil {
								return err
							}
							applied = label
						}
					}
					if applied != label {
						if err = p.labelPane(pane.ID, label); err != nil {
							return err
						}
					}
				}
			}
			if pane.Task == "0" {
				start, _ := p.tmux("display-message", "-p", "-t", pane.ID, "#{pane_start_command}")
				if strings.Contains(start, "'_welcome'") {
					if _, err = p.tmux("respawn-pane", "-k", "-t", pane.ID, "-c", p.Root, quote(shell())); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
func (p *Project) labelPane(pane, label string) error {
	if _, err := p.tmux("select-pane", "-t", pane, "-T", label); err != nil {
		return err
	}
	_, err := p.tmux("set-option", "-p", "-t", pane, "@gg_applied_label", label)
	return err
}
func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }

// Rebind only our isolated project's navigation keys. Application input and
// user terminal processes are never replayed or restarted.
func (p *Project) syncSettings() error {
	if p.controlPane() == "" {
		return nil
	}
	cfg, err := p.settings()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(cfg)
	data = append(data, []byte(":"+uiRevision)...)
	previous, _ := p.tmux("show-options", "-g", "-v", "@gg_settings")
	if previous == string(data) {
		return nil
	}
	lock, err := os.OpenFile(p.Dir+"/settings.apply-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	previous, _ = p.tmux("show-options", "-g", "-v", "@gg_settings")
	if previous == string(data) {
		return nil
	}
	for _, key := range []string{"C-Left", "C-Down", "C-Up", "C-Right", "M-Left", "M-Down", "M-Up", "M-Right", "M-h", "M-j", "M-k", "M-l"} {
		if _, err = p.tmux("unbind-key", "-q", "-n", key); err != nil {
			return err
		}
	}
	if _, err = p.tmux("unbind-key", "-q", "-n", "C-p"); err != nil {
		return err
	}
	for _, key := range []string{"q", "Q", "n", "N", "t", "T", "s", "S", "y", "Y", "b", "B", "r", "R", "Enter"} {
		script := p.invocation("_input", "--root", p.Root, "--key", key) + " --pane '#{pane_id}' --reader '#{pane_pid}' --client '#{client_pid}:#{client_created}'"
		if _, err = p.tmux("bind-key", "-n", key, "if-shell", "-F", "#{==:#{@gg_sidebar},1}", "run-shell "+quote(script), "send-keys "+key); err != nil {
			return err
		}
	}
	mouse := "set-option -p -F @gg_mouse_clients " + quote("#{@gg_mouse_clients}#{client_pid}:#{client_created},") + " ; send-keys -M"
	if _, err = p.tmux("bind-key", "-n", "MouseDown1Pane", "select-pane -t = ; if-shell -F "+quote("#{==:#{@gg_sidebar},1}")+" "+quote(mouse)+" "+quote("send-keys -M")); err != nil {
		return err
	}
	if _, err = p.tmux("bind-key", "-n", "MouseDown3Pane", "if-shell -F "+quote("#{==:#{@gg_sidebar},1}")+" "+quote("select-pane -t = ; "+mouse)+" "+quote("send-keys -M")); err != nil {
		return err
	}
	mouseRelease := "set-option -p -F @gg_last_mouse_client " + quote("#{client_pid}:#{client_created}") + " ; send-keys -M"
	if _, err = p.tmux("bind-key", "-n", "MouseUp1Pane", "select-pane -t = ; if-shell -F "+quote("#{==:#{@gg_sidebar},1}")+" "+quote(mouseRelease)+" "+quote("send-keys -M")); err != nil {
		return err
	}
	for _, binding := range []struct{ key, selection string }{
		{"DoubleClick1Pane", "select-word"},
		{"TripleClick1Pane", "select-line"},
	} {
		ordinary := "if-shell -F " + quote("#{||:#{pane_in_mode},#{mouse_any_flag}}") + " " + quote("send-keys -M") + " " + quote("copy-mode -H ; send-keys -X "+binding.selection+" ; run-shell -d 0.3 ; send-keys -X copy-pipe-and-cancel")
		command := "select-pane -t = ; if-shell -F " + quote("#{==:#{@gg_sidebar},1}") + " " + quote(mouse) + " " + quote(ordinary)
		if _, err = p.tmux("bind-key", "-n", binding.key, command); err != nil {
			return err
		}
	}
	if _, err = p.tmux("unbind-key", "-q", "-n", "C-g"); err != nil {
		return err
	}
	for _, key := range []string{"Left", "Down", "Up", "Right", "Escape", "Any"} {
		if _, err = p.tmux("unbind-key", "-q", "-T", "gg-navigate", key); err != nil {
			return err
		}
	}
	script := p.invocation("_navigate", "--root", p.Root) + " --pane '#{pane_id}' --client '#{client_pid}:#{client_created}'"
	if _, err = p.tmux("bind-key", "-n", "C-w", "run-shell", script); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"set-option", "-g", "mouse", "on"},
		{"set-option", "-s", "escape-time", "10"},
		{"set-option", "-g", "status-left", " Grand Git · " + strings.ReplaceAll(p.Root[strings.LastIndex(p.Root, "/")+1:], "#", "##") + " │ "},
		{"set-option", "-g", "status-left-length", "50"}, {"set-option", "-g", "status-right-length", "60"},
		{"set-hook", "-g", "client-attached", "run-shell " + quote(p.invocation("_attached", "--root", p.Root)+" --client #{q:hook_client}")},
		{"set-hook", "-g", "client-detached", "run-shell -b " + quote(p.invocation("_client-gone", "--root", p.Root)+" --pid '#{@gg_client_pid}' --session '#{session_name}'")},
		{"set-hook", "-g", "pane-died", "run-shell -b " + quote(p.invocation("_temporary-ended", "--root", p.Root)+" --task '#{@gg_task}' --token '#{@gg_temporary}'")},
		{"set-hook", "-g", "client-resized", "run-shell " + quote(p.invocation("_resize", "--root", p.Root))},
		{"set-option", "-g", "@gg_settings", string(data)},
	} {
		if _, err = p.tmux(args...); err != nil {
			return err
		}
	}
	return nil
}

func sidebarWidth(width int) int     { return min(56, max(28, int(math.Round(float64(width)*.25)))) }
func compact(width, height int) bool { return width < 100 || height < 16 }

func (p *Project) ensureParking() error {
	if _, err := p.tmux("has-session", "-t", "=_gg-park"); err == nil {
		return nil
	}
	pane, err := p.tmux("new-session", "-d", "-s", "_gg-park", "-P", "-F", "#{pane_id}", "-c", p.Root, p.invocation("_service"))
	if err != nil {
		return err
	}
	_, err = p.tmux("set-option", "-p", "-t", pane, "@gg_service", "1")
	return err
}

func (p *Project) resize() error {
	// Serialize rapid resize hooks; don't let opposing hide/show calls race.
	if err := os.MkdirAll(p.Dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(p.Dir+"/layout.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	snap := p.snapshot()
	for _, side := range snap {
		if !side.Sidebar || side.Task == "-1" {
			continue
		}
		terms := snap.task(mustInt(side.Task)).terminals()
		if len(terms) == 0 {
			continue
		}
		target := terms[0]
		if compact(target.WindowWidth, target.WindowHeight) {
			if side.Session == "dashboard" {
				if err = p.ensureParking(); err != nil {
					return err
				}
				if side.Active {
					p.tmux("select-pane", "-t", target.ID)
				}
				if _, err = p.tmux("break-pane", "-d", "-s", side.ID, "-t", "_gg-park:"); err != nil {
					return err
				}
			}
		} else if side.Session == "_gg-park" {
			if _, err = p.tmux("join-pane", "-d", "-h", "-b", "-l", strconv.Itoa(sidebarWidth(target.WindowWidth)), "-s", side.ID, "-t", target.ID); err != nil {
				return err
			}
			if err = p.layoutSidebar(side.ID, target.Window); err != nil {
				return err
			}
		} else if width := sidebarWidth(target.WindowWidth); side.Width != width {
			if _, err = p.tmux("resize-pane", "-t", side.ID, "-x", strconv.Itoa(width)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Tmux split -f spans the full window. For restored sidebars use a saved
// terminal layout, nested to the right of one full-height sidebar.
func (p *Project) layoutSidebar(side, window string) error {
	// The side pane is already at the left. Joining against the entire window
	// via -f isn't portable to 3.4; swapping it out and split -f would restart UI.
	// A generated tmux layout keeps every user pane and its relative geometry.
	return p.fullHeightSidebar(side, window)
}

func (p *Project) addSidebar(target string, id int) error {
	sizeText, err := p.tmux("display-message", "-p", "-t", target, "#{window_width},#{window_height}")
	if err != nil {
		return err
	}
	size := strings.Split(sizeText, ",")
	if len(size) != 2 {
		return errors.New("invalid window size")
	}
	width := sidebarWidth(mustInt(size[0]))
	args := []string{"split-window", "-d", "-f", "-h", "-b", "-l", strconv.Itoa(width), "-t", target}
	if compact(mustInt(size[0]), mustInt(size[1])) {
		if err := p.ensureParking(); err != nil {
			return err
		}
		args = []string{"new-window", "-d", "-t", "_gg-park:"}
	}
	args = append(args, "-c", p.Root, "-P", "-F", "#{pane_id}", p.invocation("_sidebar", "--root", p.Root, "--task", strconv.Itoa(id)))
	pane, err := p.tmux(args...)
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{{"@gg_sidebar", "1"}, {"@gg_task", strconv.Itoa(id)}, {"@gg_ui_revision", uiRevision}} {
		if _, err = p.tmux("set-option", "-p", "-t", pane, kv[0], kv[1]); err != nil {
			return err
		}
	}
	_, err = p.tmux("select-pane", "-t", pane, "-T", p.text("Sessions", "Сессии"))
	return err
}

func (p *Project) ensureTask(t *Task, temporary ...bool) (err error) {
	if err := p.validateWorktree(t); err != nil {
		return err
	}
	panes := p.snapshot().task(t.ID)
	if len(panes.live()) > 0 {
		return nil
	}
	generation := ""
	if t.Kind == "temporary" {
		generation = t.RunToken
	}
	if t.Protected && len(temporary) > 0 && temporary[0] {
		generation, err = newGeneration()
		if err != nil {
			return err
		}
	}
	invocation, err := p.prepareLaunch(t)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(p.launchPath(t.LaunchToken))
		}
	}()
	_ = os.Remove(p.agentPath(t.ID))
	terms := panes.terminals()
	if len(terms) > 0 {
		if _, err = p.tmux("set-window-option", "-t", terms[0].Window, "@gg_temporary", generation); err != nil {
			return err
		}
		args := append([]string{"respawn-pane", "-k", "-t", terms[0].ID, "-c", t.Path}, p.terminalEnv(t)...)
		if _, err = p.tmux(append(args, invocation)...); err != nil {
			return err
		}
		return p.labelPane(terms[0].ID, oneLine(t.label()))
	}
	args := append([]string{"new-window", "-d", "-t", "dashboard:", "-n", oneLine(t.label()), "-c", t.Path, "-P", "-F", "#{window_id}"}, p.terminalEnv(t)...)
	window, err := p.tmux(append(args, invocation)...)
	if err != nil {
		return err
	}
	if _, err = p.tmux("set-window-option", "-t", window, "@gg_task", strconv.Itoa(t.ID)); err != nil {
		p.tmux("kill-window", "-t", window)
		return err
	}
	if _, err = p.tmux("set-window-option", "-t", window, "@gg_temporary", generation); err != nil {
		p.tmux("kill-window", "-t", window)
		return err
	}
	if err = p.addSidebar(window, t.ID); err != nil {
		p.tmux("kill-window", "-t", window)
		return err
	}
	for _, pane := range p.snapshot().task(t.ID).terminals() {
		if err = p.labelPane(pane.ID, oneLine(t.label())); err != nil {
			return err
		}
		p.tmux("select-pane", "-t", pane.ID)
	}
	return nil
}

func (p *Project) openTask(id int, control bool) error {
	if err := p.Start(id); err != nil {
		return err
	}
	panes := p.snapshot().task(id).terminals()
	if len(panes) == 0 {
		return errors.New(p.text("cannot open terminal", "не удалось открыть терминал"))
	}
	if _, err := p.tmux("select-window", "-t", panes[0].Window); err != nil {
		return err
	}
	target := p.returnPane(id)
	if control {
		if side := p.snapshot().task(id).sidebar(); side != "" {
			target = side
		}
	}
	if _, err := p.tmux("select-pane", "-t", target); err != nil {
		return err
	}
	return p.resize()
}

func (p *Project) openTaskForRun(id int, token string) error {
	if err := p.Start(id); err != nil {
		return err
	}
	session := "run-" + token
	if _, err := p.tmux("has-session", "-t", session); err != nil {
		if _, err = p.tmux("new-session", "-d", "-s", session, "-t", "dashboard"); err != nil {
			return err
		}
	}
	panes := p.snapshot().task(id).terminals()
	if len(panes) == 0 {
		return errors.New(p.text("cannot open terminal", "не удалось открыть терминал"))
	}
	windows, err := p.tmux("list-windows", "-t", session, "-F", "#{window_id}\t#{window_index}")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(windows, "\n") {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) == 2 && fields[0] == panes[0].Window {
			if _, err = p.tmux("select-pane", "-t", p.returnPane(id)); err != nil {
				return err
			}
			if _, err = p.tmux("select-window", "-t", session+":"+fields[1]); err != nil {
				return err
			}
			return p.resize()
		}
	}
	return errors.New("dashboard window is unavailable")
}

func (p *Project) killTask(id int) error {
	snap, err := p.checkedSnapshot()
	if err != nil {
		return err
	}
	panes := snap.task(id)
	if len(panes) == 0 {
		return nil
	}
	home := p.controlPane()
	if home != "" {
		// Stopping a background task must not steal focus from another terminal.
		current, _ := p.tmux("display-message", "-p", "-t", "=dashboard:", "#{window_id}")
		for _, pane := range panes {
			if pane.Window == current {
				p.tmux("select-window", "-t", home)
				break
			}
		}
		controlWindow, _ := p.tmux("display-message", "-p", "-t", home, "#{window_id}")
		sessions, _ := p.tmux("list-sessions", "-F", "#{session_name}\t#{window_id}")
		for _, line := range strings.Split(sessions, "\n") {
			fields := strings.SplitN(line, "\t", 2)
			if len(fields) != 2 || !strings.HasPrefix(fields[0], "run-") {
				continue
			}
			for _, pane := range panes {
				if pane.Window != fields[1] {
					continue
				}
				windows, _ := p.tmux("list-windows", "-t", fields[0], "-F", "#{window_id}\t#{window_index}")
				for _, window := range strings.Split(windows, "\n") {
					parts := strings.SplitN(window, "\t", 2)
					if len(parts) == 2 && parts[0] == controlWindow {
						_, _ = p.tmux("select-window", "-t", fields[0]+":"+parts[1])
						break
					}
				}
				break
			}
		}
	}
	// Capture descendants before closing their PTYs, including foreground and
	// background jobs. The detached operation worker is a child of tmux itself.
	if err := stopPaneProcesses(panes.terminals()); err != nil {
		return err
	}
	windows := map[string]bool{}
	for _, pane := range panes {
		if pane.Session == "_gg-park" {
			if _, err := p.tmux("kill-pane", "-t", pane.ID); err != nil {
				return err
			}
			continue
		}
		if !windows[pane.Window] {
			if _, err := p.tmux("kill-window", "-t", pane.Window); err != nil {
				return err
			}
			windows[pane.Window] = true
		}
	}
	return nil
}
func (p *Project) deletingOwnTerminal(id int) bool {
	if !p.inOwnServer() {
		return false
	}
	for _, pane := range p.snapshot().task(id) {
		if pane.ID == os.Getenv("TMUX_PANE") {
			return true
		}
	}
	return false
}
func (p *Project) deleteInBackground(id int, force bool) error {
	return p.operationInBackground("delete", []int{id}, force, false)
}

func (p *Project) returnPane(id int) string {
	panes := p.snapshot().task(id)
	terms := panes.live()
	if len(terms) == 0 {
		return ""
	}
	last, _ := p.tmux("show-window-option", "-v", "-t", terms[0].Window, "@gg_return")
	for _, pane := range terms {
		if pane.ID == last {
			return last
		}
	}
	return terms[0].ID
}

func (p *Project) navigate(paneID string, client ...string) error {
	snap := p.snapshot()
	var current *Pane
	for i := range snap {
		if snap[i].ID == paneID {
			current = &snap[i]
			break
		}
	}
	if current == nil {
		return nil
	}
	var panes []*Pane
	seen := make(map[string]bool)
	for i := range snap {
		x := &snap[i]
		if x.Window != current.Window || x.Dead || x.Service || x.Session == "_gg-park" || seen[x.ID] {
			continue
		}
		seen[x.ID] = true
		panes = append(panes, x)
	}
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].Y != panes[j].Y {
			return panes[i].Y < panes[j].Y
		}
		if panes[i].X != panes[j].X {
			return panes[i].X < panes[j].X
		}
		return panes[i].ID < panes[j].ID
	})
	if !current.Sidebar {
		p.tmux("set-window-option", "-t", current.Window, "@gg_return", current.ID)
	}
	for i, pane := range panes {
		if pane.ID != current.ID {
			continue
		}
		if !current.Sidebar && i == len(panes)-1 && p.snapshot().task(mustInt(current.Task)).sidebar() == "" {
			return p.listPopup(mustInt(current.Task), client...)
		}
		if len(panes) > 1 {
			_, err := p.tmux("select-pane", "-t", panes[(i+1)%len(panes)].ID)
			return err
		}
		break
	}
	return nil
}

func (p *Project) addTerminal(id int, orientation string) error {
	if orientation != "below" && orientation != "right" {
		return errors.New("invalid split direction")
	}
	return p.locked(func(s *State) error {
		t, err := taskByID(s, id)
		if err != nil {
			return err
		}
		if len(p.snapshot().task(id).live()) == 0 {
			if err = p.ensureServer(); err != nil {
				return err
			}
			return p.ensureTask(t)
		}
		target := p.returnPane(id)
		flag := "-v"
		if orientation == "right" {
			flag = "-h"
		}
		args := append([]string{"split-window", flag, "-d", "-t", target, "-c", t.Path, "-P", "-F", "#{pane_id}"}, p.terminalEnv(t)...)
		pane, err := p.tmux(append(args, quote(shell()))...)
		if err != nil {
			return err
		}
		if err = p.labelPane(pane, oneLine(t.label())); err != nil {
			return err
		}
		p.tmux("set-window-option", "-t", pane, "@gg_return", pane)
		return nil
	})
}

func (p *Project) attach() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New(p.text("dashboard requires an interactive terminal", "dashboard требует интерактивный терминал"))
	}
	if err := checkDependency("tmux", "3.4"); err != nil {
		return err
	}
	source, client, err := enclosingDashboard()
	if err != nil {
		return err
	}
	if _, err = p.currentState("."); err != nil {
		return err
	}
	token, err := newGeneration()
	if err != nil {
		return err
	}
	task, err := p.createTerminal(".", "", true, token)
	if err != nil {
		return err
	}
	pid := os.Getpid()
	if source != nil {
		pid = clientPID(client)
	}
	if err = bindRun(pid, runOrigin{Workspace: p.WorkspaceID, Task: task.ID, Token: token, Path: task.Path}); err != nil {
		_ = p.Delete(task.ID, false)
		return err
	}
	if source != nil {
		return source.handoff(p, task.ID, client)
	}
	return p.connect(task.ID)
}

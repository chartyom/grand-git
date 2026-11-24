package gg

import (
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A job reserves the same section and ID as its eventual session. Keeping it
// outside the task list prevents terminal and bulk actions from using it.
func (m sidebar) job(target *listTarget) *worktreeJob {
	if target == nil || target.Folder {
		return nil
	}
	if g := m.group(target.Group); g != nil {
		for i := range g.Jobs {
			if g.Jobs[i].ID == target.ID {
				return &g.Jobs[i]
			}
		}
	}
	return nil
}

func (g sessionGroup) hasPendingWorktree() bool {
	return slices.ContainsFunc(g.Jobs, func(job worktreeJob) bool { return job.Phase != "failed" })
}

func (m sidebar) phaseLabel(phase string) string {
	switch phase {
	case "copying":
		return m.cfg.text("Copying", "Копирование")
	case "starting":
		return m.cfg.text("Starting", "Запуск")
	case "failed":
		return m.cfg.text("Failed", "Ошибка")
	default:
		return m.cfg.text("Preparing", "Подготовка")
	}
}

var worktreeSpinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m sidebar) worktreeJobLine(job *worktreeJob, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "› "
	}
	icon := worktreeSpinner[m.spinnerFrame]
	style := dim
	if job.Phase == "failed" {
		icon, style = "!", danger
	}
	prefix += icon + " ⎇  "
	label := m.phaseLabel(job.Phase)
	nameWidth := max(1, width-ansi.StringWidth(prefix)-ansi.StringWidth(label)-1)
	name := ansi.Truncate(oneLine(job.Name), nameWidth, "…")
	line := prefix + name + strings.Repeat(" ", max(1, nameWidth-ansi.StringWidth(name)+1)) + label
	if selected {
		style = selectedStyle
	}
	return style.Render(ansi.Truncate(line, width, "…"))
}

func (m sidebar) worktreeProgress() string {
	count, phase := 0, ""
	for _, g := range m.groups {
		for _, job := range g.Jobs {
			if job.Phase != "failed" {
				count++
				phase = job.Phase
			}
		}
	}
	if count == 0 {
		return ""
	}
	status := m.phaseLabel(phase)
	if count > 1 {
		status = m.cfg.text("Worktrees", "Worktree") + " " + strconv.Itoa(count)
	}
	return worktreeSpinner[m.spinnerFrame] + " " + status
}

type worktreeQueuedMsg struct {
	job *worktreeJob
	err error
}

func (m continuousDashboard) queueWorktree() tea.Cmd {
	target, cwd, values, retry := *m.formProject, m.formCWD, m.form.values(), m.form.retryID
	return func() tea.Msg {
		job, err := target.queueWorktree(cwd, values.name, values.branch, values.template, retry)
		return worktreeQueuedMsg{job, err}
	}
}

func (m continuousDashboard) worktreeQueued(msg worktreeQueuedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.form.done, m.form.message = false, msg.err.Error()
		return m, nil
	}
	job := msg.job
	target := listTarget{Group: job.SectionID, ID: job.ID}
	if g := m.side.group(target.Group); g != nil {
		g.Jobs = slices.DeleteFunc(g.Jobs, func(old worktreeJob) bool { return old.ID == m.form.retryID })
		if _, err := m.side.findTask(target.Group, target.ID); err != nil && m.side.job(&target) == nil {
			g.Jobs = append(g.Jobs, *job)
		}
	}
	m.form, m.openWhenReady = nil, &target
	m.side.clearSearch()
	m.side.onlyActive, m.side.message = false, ""
	m.side.lastKey = time.Now() // Discard refreshes that predate this reservation.
	m.side.selectTarget(target)
	m.listFocus, m.side.active = true, true
	return m, requestSidebarRefresh
}

func (m continuousDashboard) finishCreation(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	target := m.openWhenReady
	if target == nil {
		return m, cmd
	}
	if !m.listFocus || m.form != nil || m.folderForm != nil || m.side.modal != "" || m.side.searching || m.side.confirming ||
		m.side.cursorFolder || m.side.cursorGroup != target.Group || m.side.cursorID != target.ID {
		m.openWhenReady = nil
		return m, cmd
	}
	if job := m.side.job(target); job != nil {
		if job.Phase == "failed" {
			m.openWhenReady = nil
		}
		return m, cmd
	}
	m.openWhenReady = nil
	g := m.side.group(target.Group)
	if _, err := m.side.findTask(target.Group, target.ID); err != nil || g == nil || len(g.Snap.task(target.ID).live()) == 0 {
		return m, cmd
	}
	next, opened := m.open(popupAction{Kind: "open", Workspace: g.workspaceID(), ID: target.ID})
	return next, tea.Batch(cmd, opened)
}

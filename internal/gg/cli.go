package gg

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func Execute() error {
	err := newCLI().Execute()
	if err == nil {
		return nil
	}
	p, _ := Discover(".")
	cfg, _ := p.settings()
	return errors.New(localizeError(err.Error(), cfg.Language))
}

type SessionView struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Workspace   WorkspaceView `json:"workspace"`
	Branch      string        `json:"branch"`
	Base        string        `json:"base"`
	Path        string        `json:"path"`
	CreatedAt   time.Time     `json:"created_at"`
	Protected   bool          `json:"protected"`
	CopySource  string        `json:"copy_source,omitempty"`
	CopiedFiles int           `json:"copied_files,omitempty"`
	Active      bool          `json:"active"`
	Temporary   bool          `json:"temporary"`
	Terminals   int           `json:"terminals"`
	AgentStatus string        `json:"agent_status"`
	Template    string        `json:"template"`
}

func (p *Project) listBranchError(t *Task, err error) error {
	if _, statErr := os.Stat(filepath.Join(t.Path, ".git")); !errors.Is(statErr, os.ErrNotExist) {
		return err
	}
	// Delete removes the checkout under the registry lock, then commits its
	// removal. A list inside a checkout hook must never wait on that lock.
	return p.withRegistryLock(unix.LOCK_SH|unix.LOCK_NB, func(latest *State) error {
		if _, missing := taskByID(latest, t.ID); missing != nil {
			return nil
		}
		return err
	})
}

func (p *Project) sessionViews(s *State, snap Snapshot) ([]SessionView, error) {
	out := []SessionView{}
	var failures []error
	for _, t := range s.Tasks {
		branch, err := t.currentBranch()
		if err != nil {
			if err = p.listBranchError(t, err); err != nil {
				failures = append(failures, fmt.Errorf("%s (%s): %w", t.Path, p.publicID(t.ID), err))
			}
			continue
		}
		live := len(snap.task(t.ID).live())
		out = append(out, SessionView{ID: p.publicID(t.ID), Name: t.label(), Kind: t.kind(), Workspace: p.workspaceView(), Branch: branch, Base: t.Base, Path: t.Path, CreatedAt: t.CreatedAt, Protected: t.Protected, CopySource: t.CopySource, CopiedFiles: t.CopiedFiles, Active: live > 0, Temporary: (t.Protected || t.Kind == "temporary") && snap.task(t.ID).temporary(), Terminals: live, AgentStatus: p.status(t, snap), Template: t.templateName()})
	}
	return out, errors.Join(failures...)
}

func newCLI() *cobra.Command {
	var project *Project
	var discoveryErr error
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "_prepare-worktree" || args[0] == "_checkout-worktree") {
		// Keep the worker's inherited lease and acknowledgement pipe private
		// while the command resolves its workspace with Git.
		unix.CloseOnExec(3)
		unix.CloseOnExec(4)
	}
	// A dashboard starts with global settings. Service commands resolve their
	// target via --workspace or --root, independently of the caller's cwd.
	dashboardLaunch := len(args) == 1 && args[0] == "dashboard" && os.Getenv("GG_TEST_LEGACY_DASHBOARD") != "1"
	serviceCall := len(args) > 0 && strings.HasPrefix(args[0], "_") && !slices.Contains(args, "--help") && !slices.Contains(args, "-h")
	if !dashboardLaunch && !serviceCall {
		project, discoveryErr = DiscoverWorkspace(".")
	}
	cfg, cfgErr := project.settings()
	tr := cfg.text
	discover := func() (*Project, error) {
		if discoveryErr != nil {
			return nil, discoveryErr
		}
		if cfgErr != nil {
			return nil, cfgErr
		}
		return project, project.register()
	}
	root := &cobra.Command{
		Use: "gg", Short: tr("Parallel Git sessions and persistent terminals", "Параллельные Git-сессии и постоянные терминалы"),
		Long:         tr("Grand Git — isolated worktrees and persistent terminals for parallel tasks.\n\nCreate sessions in Git projects or ordinary directories; list and dashboard show all known groups.\nstart/stop control activity. delete removes a worktree and preserves Git branches.\nconfig manages language and templates. init installs local Codex status hooks.\nWithout a command, gg shows this help.", "Grand Git — отдельные worktree и постоянные терминалы для параллельных задач.\n\ncreate создаёт сессию в Git или обычной папке; list и dashboard показывают все группы.\nstart/stop управляют активностью. delete удаляет worktree и сохраняет Git-ветки.\nconfig задаёт язык и шаблоны. init устанавливает локальные хуки Codex.\nБез подкоманды gg показывает справку."),
		Example:      "  gg create clone1 --base main\n  gg dashboard\n  gg stop clone1\n  gg start clone1\n  gg stop --all\n  gg delete clone1\n  gg config set language ru\n  gg create --help",
		SilenceUsage: true, SilenceErrors: true,
		Version: fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, BuildDate),
	}
	root.InitDefaultVersionFlag()
	if flag := root.Flags().Lookup("version"); flag != nil {
		flag.Usage = tr("Show build version", "Показать версию сборки")
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.Long += tr("\n\ntemplate list/show inspects startup commands. create --template selects them.\nset/reset update YAML settings globally by default; --local saves in shared .gg.", "\n\ntemplate list/show показывает команды запуска, create --template выбирает шаблон.\nset/reset меняет YAML-настройки глобально; --local сохраняет их в общей .gg.")
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf(tr("unknown command %q", "неизвестная команда %q"), args[0])
		}
		return cmd.Help()
	}
	if cfg.Language == "ru" {
		r := strings.NewReplacer("Usage:", "Использование:", "Available Commands:", "Доступные команды:", "Global Flags:", "Общие флаги:", "Flags:", "Флаги:", "Examples:", "Примеры:", "Use \"", "Используйте \"", "for more information about a command.", "для справки о команде.")
		root.SetUsageTemplate(r.Replace(root.UsageTemplate()))
		root.SetHelpTemplate(r.Replace(root.HelpTemplate()))
	}
	initCmd := &cobra.Command{Use: "init", Short: tr("Install project-local Codex hooks", "Установить проектные хуки Codex"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := discover()
		if err != nil {
			return err
		}
		if p.Directory {
			return errors.New("init requires a Git repository")
		}
		if err = p.Init(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), tr("Local hooks installed. In Codex, open /hooks, inspect and trust them.", "Проектные хуки установлены. В Codex откройте /hooks, проверьте их и подтвердите доверие."))
		return nil
	}}
	create := &cobra.Command{Use: "create [name]", Short: tr("Create an active session in the background", "Создать активную сессию в фоне"), Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := discover()
		if err != nil {
			return err
		}
		values := createValues{}
		values.branch, _ = cmd.Flags().GetString("branch")
		if cmd.Flags().Changed("branch") {
			if p.Directory {
				return errors.New(tr("--branch requires a Git repository", "--branch требует Git-репозитория"))
			}
			if strings.TrimSpace(values.branch) == "" {
				return errors.New(tr("specify a branch name after --branch", "укажите имя ветки после --branch"))
			}
			values.name = values.branch
		}
		if len(args) > 0 {
			values.name = args[0]
		}
		base, _ := cmd.Flags().GetString("base")
		skip, _ := cmd.Flags().GetBool("no-copy")
		if p.Directory && (cmd.Flags().Changed("base") || cmd.Flags().Changed("no-copy")) {
			return errors.New("--base and --no-copy require a Git repository")
		}
		values.template, _ = cmd.Flags().GetString("template")
		choose, _ := cmd.Flags().GetBool("choose-template")
		if cmd.Flags().Changed("template") {
			if err := validateTemplateName(values.template); err != nil {
				return err
			}
		}
		interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		if choose && !interactive {
			return errors.New(tr("--choose-template requires an interactive terminal", "--choose-template требует интерактивный терминал"))
		}
		if interactive && ((!p.Directory && values.name == "") || choose) {
			values, err = promptCreate(p, cfg, values, choose)
			if errors.Is(err, errCancelled) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		if values.name == "" && !p.Directory {
			return errors.New(tr("specify a worktree name: gg create <name>", "укажите имя worktree: gg create <name>"))
		}
		t, err := p.Create(".", values.name, values.branch, base, skip, values.template)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), tr("Session created: %s\nPath: %s\nOpen: gg dashboard\n", "Сессия создана: %s\nПапка: %s\nОткрыть: gg dashboard\n"), t.label(), t.Path)
		fmt.Fprintf(cmd.OutOrStdout(), tr("Template: %s (startup runs in the terminal)\n", "Шаблон: %s (команды выполняются в терминале)\n"), t.templateName())
		if t.CopySource != "" {
			fmt.Fprintf(cmd.OutOrStdout(), tr("Ignored files: %d; source: %s\n", "Игнорируемые файлы: %d; источник: %s\n"), t.CopiedFiles, t.CopySource)
		}
		return nil
	}}
	create.Long = tr("Create a session in the background and return to your terminal.\n\nInside Git, create a named worktree with detached HEAD at the calling HEAD or --base.\nUse --branch BRANCH to create a branch or attach an existing free branch.\nAn existing branch cannot be combined with --base. Omitting the worktree name\nwith --branch uses the branch name. Use git switch inside the worktree.\nOutside Git, create [name] opens a terminal in the current directory; name defaults\nto the directory name and no branch prompt appears. Duplicate terminal names are allowed.\n\nGit worktrees copy ignored local files (.env, .agents, node_modules) by default.\n--no-copy skips copying. A local worktree of --base is the preferred copy source;\notherwise use the calling worktree. --branch, --base and --no-copy require Git.", "Создать сессию в фоне и вернуть управление терминалу.\n\nВ Git создаётся именованный worktree без ветки на текущем HEAD или --base.\nОпция --branch ВЕТКА создаёт ветку или подключает существующую свободную.\nДля существующей ветки --base недопустима. Если имя worktree опущено,\nиспользуется имя из --branch. Для смены ветки в worktree выполните git switch.\nВне Git create [name] открывает терминал в текущей папке. Имя по умолчанию — имя\nпапки; вопросов о ветке нет. Одинаковые имена обычных терминалов разрешены.\n\nДля worktree по умолчанию копируются игнорируемые файлы (.env, .agents, node_modules).\n--no-copy отключает копирование. Источник — worktree явной --base, если он есть,\nиначе worktree вызова. --branch, --base и --no-copy доступны только в Git.")
	create.Long += tr("\n\nThe session belongs to the dashboard section for the exact calling directory;\nthe section is added when needed. Public IDs and workspace grouping stay unchanged.", "\n\nСессия относится к разделу dashboard для точной папки вызова;\nпри необходимости раздел добавляется. Публичные ID и группировка workspace сохраняются.")
	create.Example = "  gg create clone1\n  gg create clone2 --base release/1.0\n  gg create clone3 --branch feat/login\n  gg create --branch fix/release --no-copy"
	create.Long += tr("\nFor example, feat/data uses one worktree folder named feat~data.", "\nНапример, для feat/data создаётся одна папка worktree с именем feat~data.")
	create.Flags().String("branch", "", tr("Git branch to create or attach (default: detached HEAD)", "Ветка для создания или подключения (по умолчанию worktree без ветки)"))
	create.Flags().String("base", "", tr("Base branch or commit (default: current HEAD)", "Базовая ветка или коммит (по умолчанию текущий HEAD)"))
	create.Flags().Bool("no-copy", false, tr("Skip Git-ignored environment files", "Не копировать игнорируемые Git файлы окружения"))
	create.Flags().String("template", "", tr("Startup template (default: default_template setting, then shell)", "Шаблон запуска (по умолчанию default_template, затем shell)"))
	create.Flags().Bool("choose-template", false, tr("Choose a template interactively", "Выбрать шаблон интерактивно"))
	create.MarkFlagsMutuallyExclusive("template", "choose-template")
	create.Long += tr("\n\n--template selects named startup commands; --choose-template opens the optional picker.\nA template runs on creation and cold start, not on switching. Commands begin after\nregistration; completion/errors leave an interactive shell. Creation does not wait\nfor command success. The default template is shell. Ignored .gg is not copied.", "\n\n--template выбирает команды запуска; --choose-template открывает список по запросу.\nШаблон выполняется при создании и запуске остановленной сессии, не при переключении.\nКоманды начинаются после регистрации; после завершения/ошибки остаётся оболочка.\nСоздание не ожидает успеха команд. По умолчанию шаблон shell. Игнорируемая .gg не копируется.")
	create.Example += "\n  gg create feat/agent --template codex\n  gg create feat/server --choose-template"
	list := &cobra.Command{Use: "list", Short: tr("List all known sessions", "Показать все известные сессии"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := discover()
		if err != nil {
			return err
		}
		if _, err = p.currentState("."); err != nil {
			return err
		}
		groups, err := collectGroups(false)
		if err != nil {
			return err
		}
		rows := []SessionView{}
		failures := []error{}
		for _, g := range groups {
			if g.Err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", g.View.Path, g.Err))
				continue
			}
			groupRows, err := g.Project.sessionViews(g.State, g.Snap)
			rows = append(rows, groupRows...)
			if err != nil {
				failures = append(failures, err)
			}
		}
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			if err = json.NewEncoder(cmd.OutOrStdout()).Encode(rows); err != nil {
				return err
			}
		} else {
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, tr("GROUP\tID\tNAME\tBRANCH\tTYPE\tACTIVE\tTERMINALS\tAGENT\tPATH", "ГРУППА\tID\tИМЯ\tВЕТКА\tТИП\tАКТИВНА\tТЕРМИНАЛЫ\tАГЕНТ\tПАПКА"))
			for _, r := range rows {
				active := tr("no", "нет")
				if r.Active {
					active = tr("yes", "да")
				}
				kind := cfg.text(r.Kind, map[string]string{"source": "исходная", "worktree": "задача", "terminal": "терминал", "temporary": "временная"}[r.Kind])
				branch := r.Branch
				if r.Kind == "terminal" || r.Kind == "temporary" {
					branch = "—"
				} else if branch == "" {
					branch = tr("detached", "без ветки")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", oneLine(r.Workspace.Name), r.ID, oneLine(r.Name), oneLine(branch), kind, active, r.Terminals, p.statusLabel(r.AgentStatus), oneLine(r.Path))
			}
			if err = w.Flush(); err != nil {
				return err
			}
		}
		return errors.Join(failures...)
	}}
	list.Long = tr("List active and stopped sessions from every known project and directory.\nIDs use group:session, for example gg delete 3:7 --yes. Worktree names address sessions\nin the current project across branch switches. An unavailable group is reported on stderr; healthy\nrecords are still printed and the exit status indicates the partial failure.", "Показать активные и остановленные сессии всех известных проектов и папок.\nID имеет вид группа:сессия, например gg delete 3:7 --yes. Имя worktree адресует сессию\nв текущем проекте и сохраняется при смене ветки. Недоступная группа сообщается в stderr; исправные записи выводятся,\nкод завершения указывает на неполный результат.")
	list.Long += tr("\nSource worktrees are not registered automatically; only running legacy sources remain.", "\nИсходные worktree не регистрируются автоматически; остаются только работающие старые сессии.")
	list.Flags().Bool("json", false, tr("Print all known sessions with global IDs as JSON", "Вывести все известные сессии с глобальными ID в JSON"))
	start := &cobra.Command{Use: "start <name|id>", Short: tr("Start a stopped session without opening dashboard", "Запустить остановленную сессию в фоне"), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, t, err := resolveSession(args[0])
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "session no longer exists") {
				return fmt.Errorf("%w; %s", err, tr("use gg dashboard, add the directory with + and press T on its section", "используйте gg dashboard, добавьте папку через + и нажмите T на её разделе"))
			}
			return err
		}
		name, _ := cmd.Flags().GetString("template")
		if cmd.Flags().Changed("template") {
			if err := validateTemplateName(name); err != nil {
				return err
			}
		}
		if err = p.Start(t.ID, name); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), tr("Active: %s\n", "Активна: %s\n"), t.label())
		return nil
	}}
	start.Flags().String("template", "", tr("Change the template of a stopped session before starting", "Сменить шаблон остановленной сессии перед запуском"))
	start.Long = tr("Accept a worktree name in the current Git project or a global group:session ID.\nStart using the session's current named template. Active sessions are unchanged.\n--template changes a stopped session's template; it is rejected for an active session.\nUse --template shell if a previously selected template is unavailable.", "Принимает имя worktree текущего Git-проекта или глобальный ID группа:сессия.\nЗапустить сессию с актуальной версией её шаблона. Активные сессии не меняются.\n--template меняет шаблон остановленной сессии; для активной сессии отклоняется.\nЕсли прежний шаблон недоступен, используйте --template shell.")
	operation := func(action string) *cobra.Command {
		short := tr("Stop terminals, preserving files", "Остановить терминалы, сохранив файлы")
		if action == "delete" {
			short = tr("Stop and remove worktrees, preserving Git branches", "Остановить и удалить worktree, сохранив Git-ветки")
		}
		c := &cobra.Command{Use: action + " <name|id> | --all", Short: short, Args: cobra.MaximumNArgs(1)}
		c.Long = tr("Accept a worktree name in the current Git project or a global group:session ID.\n--all targets only the current project or directory. Managed and ordinary sessions\nkeep their records when stopped; legacy source records are retired after stopping.\nSource/external worktree directories and Git branches are always preserved.", "Принимает имя worktree текущего Git-проекта или глобальный ID группа:сессия.\n--all действует только на текущий проект или папку. При остановке записи задач\nи обычных терминалов сохраняются; старые исходные записи убираются.\nПапки исходных/внешних worktree и Git-ветки всегда сохраняются.")
		if action == "delete" {
			c.Long += tr("\nDelete removes managed worktrees and records, or just plain/temporary terminal\nrecords in Git or ordinary directory groups, preserving their directory files. Modified/untracked files require --force; ignored files are\nremoved with the worktree. A detached commit outside the history of all local\nbranches, remote-tracking branches and tags also requires --force. Save it with\ngit switch -c <branch> before deletion. Noninteractive deletion requires --yes.", "\nУдаление убирает worktree задачи и запись; у обычных и временных терминалов\nв Git и других папках удаляется только запись, файлы папки сохраняются. Изменённые/неотслеживаемые файлы требуют --force; игнорируемые файлы\nудаляются с worktree. Если текущий коммит дерева без ветки не входит в историю\nлокальной ветки, ветки удалённого репозитория или тега, также требуется --force.\nСохраните коммит через git switch -c <ветка>. Для неинтерактивного удаления нужен --yes.")
		}
		c.Flags().Bool("all", false, tr("All sessions in the current project or directory", "Все сессии текущего проекта или папки"))
		c.Flags().Bool("yes", false, tr("Skip confirmation", "Пропустить подтверждение"))
		if action == "delete" {
			c.Flags().Bool("force", false, tr("Allow loss of uncommitted files or unreferenced detached commits", "Разрешить потерю незакоммиченных файлов или коммитов вне веток и тегов"))
		}
		c.RunE = func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			yes, _ := cmd.Flags().GetBool("yes")
			force := false
			if action == "delete" {
				force, _ = cmd.Flags().GetBool("force")
			}
			if (all && len(args) > 0) || (!all && len(args) == 0) {
				return errors.New(tr("specify a worktree name, group:session ID, or --all", "укажите имя worktree, ID группа:сессия или --all"))
			}
			var p *Project
			var err error
			ids := []int{}
			label := tr("all current group sessions", "все сессии текущей группы")
			if all {
				p, err = discover()
				if err != nil {
					return err
				}
				s, e := p.currentState(".")
				if e != nil {
					return e
				}
				for _, t := range s.Tasks {
					ids = append(ids, t.ID)
				}
			} else {
				var t *Task
				p, t, err = resolveSession(args[0])
				if err != nil {
					return err
				}
				if action == "delete" && t.Protected {
					return errors.New(tr("original worktree is protected; it cannot be deleted", "исходный worktree защищён: удалять его нельзя"))
				}
				ids = append(ids, t.ID)
				label = t.label() + " (" + p.publicID(t.ID) + ")"
			}
			if (action == "delete" || all) && !yes {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return errors.New(tr("confirmation requires a terminal; pass --yes", "для неинтерактивного выполнения укажите --yes"))
				}
				verb := tr("Stop", "Остановить")
				if action == "delete" {
					verb = tr("Stop and delete", "Остановить и удалить")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s? [y/N] ", verb, label)
				answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if strings.ToLower(strings.TrimSpace(answer)) != "y" {
					fmt.Fprintln(cmd.OutOrStdout(), tr("Cancelled", "Отменено"))
					return nil
				}
			}
			own := false
			for _, id := range ids {
				if p.deletingOwnTerminal(id) {
					own = true
					break
				}
			}
			if own {
				// Preflight deletion before the originating terminal can disappear.
				if action == "delete" {
					if err = p.preflightDelete(ids, force, all); err != nil {
						return err
					}
				}
				fmt.Fprintln(cmd.OutOrStdout(), tr("Operation continues outside this terminal; see dashboard for the result.", "Операция продолжится вне этого терминала; результат появится в dashboard."))
				return p.operationInBackground(action, ids, force, all)
			}
			if err = p.operate(action, ids, force, all); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), tr("Completed: %s %s. Files of ordinary directories, Git branches and protected worktrees preserved.\n", "Выполнено: %s %s. Файлы обычных папок, Git-ветки и исходные worktree сохранены.\n"), action, label)
			return nil
		}
		return c
	}
	dashboard := &cobra.Command{Use: "dashboard", Short: tr("Open the session list and interactive terminals", "Открыть список сессий и терминалы"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GG_TEST_LEGACY_DASHBOARD") == "1" {
			p, err := discover()
			if err != nil {
				return err
			}
			return p.attach()
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		p := &Project{Root: cwd, Directory: true}
		return p.attachContinuous()
	}}
	dashboard.Long = tr("Show saved folder sections and sessions in one dashboard process.\nLaunch creates no session, opens no terminal and does not add the calling\ndirectory. Press + to add an existing directory using a path or folder browser.\nSections may refer to different directories in the same Git repository.\nOn a section, T creates a persistent shell in its exact directory, N creates a\nworktree in Git or a directory session, and X removes an empty section.\nSessions share one list with worktree/terminal marks. Enter or a single click\nopens a session; stopped sessions start. Ctrl+W cycles the list and panes.\nQ or disconnection closes the dashboard and preserves persistent terminals.", "Показать сохранённые разделы-папки и сессии в одном процессе dashboard.\nЗапуск не создаёт сессий, не открывает терминал и не добавляет папку вызова.\nНажмите +, чтобы добавить существующую папку через поле пути или обзор папок.\nРазделы могут указывать на разные папки одного Git-репозитория.\nНа разделе T создаёт постоянный shell точно в его папке, N создаёт worktree\nв Git или обычную сессию, X убирает пустой раздел.\nСессии показаны общим списком с обозначением worktree/терминала. Enter или\nодин клик открывает сессию; остановленная сессия запускается. Ctrl+W переключает\nсписок и панели. Q и отключение закрывают dashboard, сохраняя постоянные терминалы.")
	dashboard.Long += tr("\n\nRight-click, ? or Shift+F10 opens target actions. Hover previews details.\nS/D on a section confirms Stop all/Delete all for that section's fixed targets;\nGit branches, protected worktrees and ordinary directory files remain.\nSearch (/) is available above ten activity-filtered session rows. Down/Enter\napplies search; Esc clears it. Forms support mouse and keyboard. The folder\nbrowser accepts relative paths and ~/; Ctrl+S adds the selected directory.", "\n\nПравая кнопка, ? или Shift+F10 открывает действия; наведение показывает детали.\nS/D на разделе подтверждает остановку/удаление его фиксированного набора сессий;\nGit-ветки, защищённые worktree и файлы обычных папок сохраняются.\nПоиск (/) доступен при числе сессий больше десяти после фильтра активности.\nDown/Enter применяет поиск; Esc очищает. Формы поддерживают мышь и клавиатуру.\nОбзор папок принимает относительные пути и ~/; Ctrl+S добавляет выбранную папку.")

	dashboard.Long += tr("\nGit worktree creation shows an animated row while files copy. You can use\nother terminals or quit while it runs. Stay on the row to open it when ready.\nOn failure, press Enter on the row to retry with the saved form.\n", "\nСоздание Git-worktree показывает строку с анимацией копирования. Пока оно идёт,\nможно работать в другом терминале или выйти. Останьтесь на строке, чтобы открыть\nготовую сессию. При ошибке Enter вернёт заполненную форму для повтора.\n")
	dashboard.Long += tr("\nOn a Git section, R enables relative worktree paths for the whole project.\nConfirm that the host and all other Git clients use Git 2.48 or newer.", "\nНа Git-разделе R включает относительные пути worktree для всего проекта.\nПодтвердите, что на хосте и у всех остальных клиентов установлен Git 2.48 или новее.")

	root.AddCommand(initCmd, create, list, start, operation("stop"), operation("delete"), dashboard, configCommand(project, cfg), settingCommand(project, cfg, false), settingCommand(project, cfg, true), templateCommand(project, cfg))
	addInternal(root)
	root.InitDefaultHelpCmd()
	var translateHelp func(*cobra.Command)
	translateHelp = func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		if flag := c.Flags().Lookup("help"); flag != nil {
			flag.Usage = tr("Help for ", "Справка о ") + c.CommandPath()
		}
		if c.Name() == "help" {
			c.Short = tr("Help about any command", "Справка о командах")
			if cfg.Language == "ru" {
				c.Long = "Показать справку для выбранной команды."
			}
		}
		for _, sub := range c.Commands() {
			translateHelp(sub)
		}
	}
	translateHelp(root)
	return root
}

func configCommand(p *Project, cfg Settings) *cobra.Command {
	tr := cfg.text
	c := &cobra.Command{Use: "config", Short: tr("View effective settings and their sources", "Показать настройки и их источники"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, err := p.settings()
		if err != nil {
			return err
		}
		user, userPath, err := p.settingsLayer(false)
		if err != nil {
			return err
		}
		local, localPath, err := p.settingsLayer(true)
		if err != nil {
			return err
		}
		for _, key := range settingKeys {
			source := tr("built-in default", "встроенное значение")
			if user.values()[key] != "" {
				source = userPath
			}
			if local.values()[key] != "" {
				source = localPath
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s (%s)\n", key, s.values()[key], source)
		}
		return nil
	}}
	c.AddCommand(settingCommand(p, cfg, false), settingCommand(p, cfg, true))
	return c
}

func settingCommand(p *Project, cfg Settings, reset bool) *cobra.Command {
	tr := cfg.text
	use, count := "set <key> <value>", 2
	if reset {
		use, count = "reset <key>", 1
	}
	c := &cobra.Command{Use: use, Args: cobra.ExactArgs(count), Short: tr("Update settings (global by default)", "Изменить настройки (по умолчанию глобально)")}
	c.Long = tr("Keys: language (en/ru), default_template.\nGlobal YAML is in the user grand-git config directory; --local writes the primary\nworktree's .gg/config.yaml, shared by all worktrees. --project is an alias for --local.\nLegacy TOML is read until YAML exists; the first write migrates the selected layer.", "Ключи: language (en/ru), default_template.\nГлобальный YAML находится в пользовательском каталоге grand-git; --local записывает\nобщий для всех worktree .gg/config.yaml основной папки. --project — аналог --local.\nСтарый TOML читается до появления YAML; первая запись переносит выбранный уровень.")
	c.Flags().Bool("local", false, tr("Save in the shared project .gg directory", "Сохранить в общей проектной папке .gg"))
	c.Flags().Bool("project", false, tr("Alias for --local", "Аналог --local"))
	c.RunE = func(cmd *cobra.Command, args []string) error {
		local, _ := cmd.Flags().GetBool("local")
		project, _ := cmd.Flags().GetBool("project")
		value := ""
		if !reset {
			value = args[1]
		}
		if err := p.setSetting(args[0], value, local || project, reset); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), tr("Settings saved", "Настройки сохранены"))
		return nil
	}
	return c
}

func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}

func addInternal(root *cobra.Command) {
	internal := func(name string, run func(*Project, *cobra.Command) error) *cobra.Command {
		var dir string
		var workspace int
		cmd := &cobra.Command{Use: name, Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = "."
			}
			var p *Project
			var err error
			if workspace > 0 {
				p, err = workspaceByID(workspace)
			} else {
				p, err = DiscoverWorkspace(dir)
				if err == nil {
					err = p.register()
				}
			}
			if err != nil {
				return err
			}
			return run(p, cmd)
		}}
		cmd.Flags().StringVar(&dir, "root", "", "Project root")
		cmd.Flags().IntVar(&workspace, "workspace", 0, "Internal workspace ID")
		root.AddCommand(cmd)
		return cmd
	}
	connect := internal("_connect", func(p *Project, cmd *cobra.Command) error { id, _ := cmd.Flags().GetInt("task"); return p.connect(id) })
	connect.Flags().Int("task", -1, "Target session")
	quitRun := internal("_quit-run", func(p *Project, cmd *cobra.Command) error {
		identity, _ := cmd.Flags().GetString("client")
		token, _ := cmd.Flags().GetString("run-token")
		if err := p.quitRun(identity, token); err != nil {
			p.notice(err.Error())
		}
		return nil
	})
	quitRun.Flags().String("client", "", "Bound client identity")
	quitRun.Flags().String("run-token", "", "Bound dashboard run")
	clientGone := internal("_client-gone", func(p *Project, cmd *cobra.Command) error {
		pid, _ := cmd.Flags().GetInt("pid")
		session, _ := cmd.Flags().GetString("session")
		if pid < 2 || !strings.HasPrefix(session, "run-") {
			return nil
		}
		return p.disconnectedRun(pid, strings.TrimPrefix(session, "run-"))
	})
	clientGone.Flags().Int("pid", 0, "Detached client PID")
	clientGone.Flags().String("session", "", "Detached dashboard session")
	temporaryEnded := internal("_temporary-ended", func(p *Project, cmd *cobra.Command) error {
		id, _ := cmd.Flags().GetInt("task")
		token, _ := cmd.Flags().GetString("token")
		return p.removeEndedTemporary(id, token)
	})
	temporaryEnded.Flags().Int("task", -1, "Temporary session ID")
	temporaryEnded.Flags().String("token", "", "Temporary generation")
	terminal := internal("_terminal", func(p *Project, cmd *cobra.Command) error {
		id, _ := cmd.Flags().GetInt("task")
		token, _ := cmd.Flags().GetString("token")
		return p.runTerminal(id, token)
	})
	terminal.Flags().Int("task", -1, "Internal task ID")
	terminal.Flags().String("token", "", "Committed launch token")
	input := internal("_input", func(p *Project, cmd *cobra.Command) error {
		pane, _ := cmd.Flags().GetString("pane")
		client, _ := cmd.Flags().GetString("client")
		key, _ := cmd.Flags().GetString("key")
		reader, _ := cmd.Flags().GetInt("reader")
		return p.forwardQuitKey(pane, client, key, reader)
	})
	input.Flags().String("pane", "", "Input pane")
	input.Flags().String("client", "", "Originating client")
	input.Flags().String("key", "q", "Literal key")
	input.Flags().Int("reader", 0, "Sidebar process identity")
	quit := internal("_quit", func(p *Project, cmd *cobra.Command) error {
		client, _ := cmd.Flags().GetString("client")
		data, _ := cmd.Flags().GetString("targets")
		var targets []quitTarget
		if err := json.Unmarshal([]byte(data), &targets); err != nil {
			return err
		}
		if err := p.quit(client, targets); err != nil {
			p.notice(err.Error())
		}
		return nil
	})
	quit.Flags().String("client", "", "Bound client identity")
	quit.Flags().String("targets", "[]", "Bound temporary generations")
	attached := internal("_attached", func(p *Project, cmd *cobra.Command) error {
		pid, _ := cmd.Flags().GetInt("pid")
		client, _ := cmd.Flags().GetString("client")
		if client != "" {
			value, err := p.tmux("display-message", "-p", "-c", client, "#{client_pid}")
			if err == nil {
				pid, _ = strconv.Atoi(value)
			}
		}
		if pid > 1 {
			_ = os.Remove(p.attachLeasePath(pid))
			if session, err := p.tmux("display-message", "-p", "-c", client, "#{session_name}"); err == nil && strings.HasPrefix(session, "run-") {
				_, _ = p.tmux("set-option", "-t", session, "@gg_client_pid", strconv.Itoa(pid))
			}
		}
		return p.resize()
	})
	attached.Flags().Int("pid", 0, "Attached client PID (legacy worker)")
	attached.Flags().String("client", "", "Hook client name")
	internal("_resize", func(p *Project, cmd *cobra.Command) error { return p.resize() })
	side := internal("_sidebar", func(p *Project, cmd *cobra.Command) error {
		task, _ := cmd.Flags().GetInt("task")
		return runSidebar(p, task)
	})
	side.Flags().Int("task", -1, "Internal task ID")
	popup := internal("_list-popup", func(p *Project, cmd *cobra.Command) error {
		task, _ := cmd.Flags().GetInt("task")
		result, _ := cmd.Flags().GetString("result")
		m := newSidebar(p, task)
		m.popup = true
		m.active = true
		m.resultPath = result
		_, err := tea.NewProgram(m).Run()
		return err
	})
	popup.Flags().Int("task", -1, "Internal task ID")
	popup.Flags().String("result", "", "Result file")
	navigate := internal("_navigate", func(p *Project, cmd *cobra.Command) error {
		pane, _ := cmd.Flags().GetString("pane")
		client, _ := cmd.Flags().GetString("client")
		return p.navigate(pane, client)
	})
	navigate.Flags().String("pane", "", "Pane")
	navigate.Flags().String("client", "", "Originating client")
	form := internal("_create-form", func(p *Project, cmd *cobra.Command) error {
		cfg, err := p.settings()
		if err != nil {
			return err
		}
		values, err := promptCreate(p, cfg, createValues{}, false)
		if errors.Is(err, errCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd == "" {
			cwd = p.Root
		}
		t, err := p.Create(cwd, values.name, values.branch, "", false, values.template)
		if err != nil {
			p.notice(err.Error())
			return nil
		}
		result, _ := cmd.Flags().GetString("result")
		data, err := json.Marshal(popupAction{Kind: "open", Workspace: p.WorkspaceID, ID: t.ID})
		if err != nil {
			return err
		}
		return os.WriteFile(result, data, 0600)
	})
	form.Flags().String("cwd", "", "Calling worktree")
	form.Flags().String("result", "", "Result file")
	creation := internal("_prepare-worktree", func(p *Project, cmd *cobra.Command) error {
		id, _ := cmd.Flags().GetInt("job")
		recovery, _ := cmd.Flags().GetBool("recover")
		return p.runWorktreeWorker(id, recovery)
	})
	creation.Flags().Int("job", 0, "Reserved worktree ID")
	creation.Flags().Bool("recover", false, "Recover an interrupted creation")
	checkout := internal("_checkout-worktree", func(p *Project, cmd *cobra.Command) error {
		id, _ := cmd.Flags().GetInt("job")
		return p.withWorkerLease(id, func(j *worktreeJob, _ *os.File) error {
			return p.claimCheckout(j)
		})
	})
	checkout.Flags().Int("job", 0, "Reserved worktree ID")
	worker := internal("_operate", func(p *Project, cmd *cobra.Command) error {
		ids, _ := cmd.Flags().GetIntSlice("id")
		action, _ := cmd.Flags().GetString("action")
		force, _ := cmd.Flags().GetBool("force")
		all, _ := cmd.Flags().GetBool("all")
		surviving, _ := cmd.Flags().GetBool("surviving")
		if action != "stop" && action != "delete" {
			return errors.New("invalid operation")
		}
		if err := p.operate(action, ids, force, all, surviving); err != nil {
			p.notice(err.Error())
		} else {
			p.setNotice(p.text("Completed: ", "Выполнено: ")+action, true)
		}
		return nil
	})
	worker.Flags().IntSlice("id", nil, "Bound internal IDs")
	worker.Flags().String("action", "", "Action")
	worker.Flags().Bool("force", false, "Allow dirty removal")
	worker.Flags().Bool("all", false, "Preserve protected records")
	worker.Flags().Bool("surviving", false, "Operate on surviving confirmed bulk targets")
	// Compatibility for workers already queued by the preceding binary.
	old := internal("_delete", func(p *Project, cmd *cobra.Command) error {
		id, _ := cmd.Flags().GetInt("id")
		force, _ := cmd.Flags().GetBool("force")
		return p.Delete(id, force)
	})
	old.Flags().Int("id", 0, "Internal task ID")
	old.Flags().Bool("force", false, "Allow dirty removal")
	service := func(cmd *cobra.Command, args []string) error {
		for {
			time.Sleep(time.Hour)
		}
	}
	root.AddCommand(&cobra.Command{Use: "_service", Hidden: true, RunE: service}, &cobra.Command{Use: "_welcome", Hidden: true, RunE: service})
	hook := &cobra.Command{Use: "_hook", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		var payload json.RawMessage
		if json.NewDecoder(cmd.InOrStdin()).Decode(&payload) != nil {
			return nil
		}
		var event struct{ Cwd string }
		if json.Unmarshal(payload, &event) != nil || event.Cwd == "" {
			return nil
		}
		p, err := Discover(event.Cwd)
		if err != nil {
			return nil
		}
		name, _ := cmd.Flags().GetString("event")
		_ = p.hook(name, strings.NewReader(string(payload)))
		return nil
	}}
	hook.Flags().String("event", "", "Event")
	root.AddCommand(hook)
}

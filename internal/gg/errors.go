package gg

import "strings"

// Domain errors remain independent of CLI/TUI locale, including compatibility
// with workers started by the old binary. Git/tmux diagnostic details stay raw.
func localizeError(s, language string) string {
	pairs := [][2]string{
		{"section no longer exists", "раздел больше не существует"},
		{"section is not empty", "раздел содержит сессии"},
		{"section belongs to another workspace", "раздел принадлежит другому проекту"},
		{"section directory is unavailable or replaced", "папка раздела недоступна или подменена"},
		{"invalid section catalog version or counter; file preserved", "некорректная версия или счётчик каталога разделов; файл сохранён"},
		{"invalid section catalog record; file preserved", "некорректная запись каталога разделов; файл сохранён"},
		{"Sessions no longer exist", "Сессии больше не существуют"},
		{"--base and --no-copy require a Git repository", "--base и --no-copy требуют Git-репозитория"},
		{"init requires a Git repository", "init требует Git-репозитория"},
		{"invalid workspace catalog record; file preserved", "некорректная запись каталога групп; файл сохранён"},
		{"invalid workspace catalog counter", "некорректный счётчик каталога групп"},
		{"invalid workspace catalog", "повреждён каталог групп"},
		{"invalid session ID; expected group:session", "некорректный ID сессии; ожидается группа:сессия"},
		{"use the session ID from gg list: group:session", "используйте ID из gg list: группа:сессия"},
		{"workspace directory was replaced", "папка группы подменена"},
		{"workspace repository was replaced", "репозиторий группы подменён"},
		{"invalid terminal session", "некорректная терминальная сессия"},
		{"session directory is unavailable", "папка сессии недоступна"},
		{"dashboard client is no longer attached", "клиент dashboard больше не подключён"},
		{"cannot identify dashboard client", "не удалось определить клиента dashboard"},
		{"invalid dashboard client context", "некорректный контекст клиента dashboard"},
		{"Completed: ", "Выполнено: "},
		{"git not found; requires ", "не найден git; требуется "},
		{"tmux not found; requires ", "не найден tmux; требуется "},
		{"Install or upgrade: ", "Установите или обновите: "},
		{"Install: ", "Установите: "},
		{" required (found ", " требуется (найдено "},
		{"run this command inside a Git repository", "запустите команду внутри Git-репозитория"},
		{"bare repositories are not supported", "bare-репозитории не поддерживаются"},
		{"corrupt session registry", "повреждён реестр сессий"},
		{"invalid registry counter", "некорректный счётчик ID в реестре"},
		{"invalid session record; registry left unchanged", "некорректная запись задачи в реестре; файл сохранён без изменений"},
		{"--base cannot be used with an existing branch", "--base нельзя использовать с существующей веткой"},
		{"cannot resolve base commit", "не удалось определить базовый коммит"},
		{"cannot copy environment", "не удалось скопировать окружение"},
		{"copying ", "копирование "},
		{" is a symlink; local installation cancelled", " является ссылкой; локальная установка отменена"},
		{"hooks.json must contain a JSON object, not null", "hooks.json должен содержать JSON-объект, не null"},
		{"hooks must be a JSON object", "hooks должен быть JSON-объектом"},
		{"invalid hooks structure: ", "неверная структура hooks."},
		{"invalid hooks group: ", "неверная группа hooks."},
		{"invalid hooks handlers: ", "неверные обработчики hooks."},
		{"features must be a TOML table", "features должен быть TOML-таблицей"},
		{"invalid session path", "недопустимый путь сессии"},
		{"session parent is not a real directory", "родительский каталог сессии подменён"},
		{"unknown setting", "неизвестная настройка"},
		{"invalid value", "недопустимое значение"},
		{"--project requires a Git repository", "--project требует Git-репозитория"},
		{"--local requires a Git repository", "--local требует Git-репозитория"},
		{"invalid template name", "недопустимое имя шаблона"},
		{"use letters, numbers, _ or -", "используйте буквы, цифры, _ или -"},
		{"shell is reserved and cannot be overridden", "shell зарезервирован и не может быть переопределён"},
		{"unknown template field", "неизвестное поле шаблона"},
		{"commands must be a sequence of strings", "commands должен быть списком строк"},
		{"each command must be a nonempty string without NUL", "каждая команда должна быть непустой строкой без NUL"},
		{"startup commands require sh, bash or zsh; current shell is", "команды запуска требуют sh, bash или zsh; текущая оболочка"},
		{"choose --template shell", "выберите --template shell"},
		{"use gg template list or", "используйте gg template list или"},
		{"expected one YAML document", "ожидается один YAML-документ"},
		{"expected a YAML mapping", "ожидается YAML-объект"},
		{"expected string keys", "ожидаются строковые ключи"},
		{"duplicate key", "повторяющийся ключ"},
		{"must be a string", "должно быть строкой"},
		{"session ", "сессия "}, {" not found", " не найдена"},
		{"unknown flag", "неизвестный флаг"}, {"unknown command", "неизвестная команда"},
	}
	for _, pair := range pairs {
		if language == "ru" {
			s = strings.ReplaceAll(s, pair[0], pair[1])
		} else {
			s = strings.ReplaceAll(s, pair[1], pair[0])
		}
	}
	return s
}

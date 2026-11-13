package gg

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

func templateCommand(p *Project, cfg Settings) *cobra.Command {
	tr := cfg.text
	c := &cobra.Command{Use: "template", Short: tr("Inspect built-in, global and local startup templates", "Посмотреть встроенные, глобальные и локальные шаблоны запуска")}
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: tr("List templates and their sources", "Показать шаблоны и источники"), RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := p.templates()
		if err != nil {
			return err
		}
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, tr("NAME\tSOURCE", "ИМЯ\tИСТОЧНИК"))
		for _, t := range entries {
			fmt.Fprintf(w, "%s\t%s\n", t.Name, t.Source)
		}
		return w.Flush()
	}}
	list.Flags().Bool("json", false, tr("Print template names and sources as JSON", "Вывести имена шаблонов и источники в JSON"))
	show := &cobra.Command{Use: "show <name>", Args: cobra.ExactArgs(1), Short: tr("Print a template as editable YAML", "Вывести шаблон в YAML для редактирования"), RunE: func(cmd *cobra.Command, args []string) error {
		t, err := p.resolveTemplate(args[0])
		if err != nil {
			return err
		}
		b, err := yaml.Marshal(t)
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(b)
		return err
	}}
	c.AddCommand(list, show)
	return c
}

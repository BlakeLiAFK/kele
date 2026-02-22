package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/BlakeLiAFK/kele/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "管理 Kele 配置",
		Long:  "读取和修改 Kele 配置（存储在 SQLite 数据库中）",
	}

	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigListCmd())

	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "设置配置项",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if err := config.SetValue(key, value); err != nil {
				return fmt.Errorf("设置失败: %w", err)
			}
			fmt.Printf("%s = %s\n", key, value)
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "获取配置项",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			val, err := config.GetValue(args[0])
			if err != nil {
				return fmt.Errorf("获取失败: %w", err)
			}
			fmt.Println(val)
			return nil
		},
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有配置项及当前生效值",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			all := config.AllSettings(cfg)

			// DB 中已设置的 key
			dbValues, _ := config.ListValues()

			keys := make([]string, 0, len(all))
			for k := range all {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				v := all[k]
				if v == "" {
					v = "(未设置)"
				}
				source := ""
				if _, ok := dbValues[k]; ok {
					source = " [db]"
				}
				fmt.Printf("%-28s %s%s\n", k, v, source)
			}
			fmt.Printf("\n存储: %s\n", config.ConfigStorePath())
			return nil
		},
	}
}

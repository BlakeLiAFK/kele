package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/BlakeLiAFK/kele/internal/config"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Kele v%s\n", config.Version)
			fmt.Printf("Go: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

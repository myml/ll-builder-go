package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	versionFlag bool
	filePath    string
)

var rootCmd = &cobra.Command{
	Use:   "ll-builder",
	Short: "Linyaps application build tool",
	Long: `ll-builder is a tool provided for application developers to build Linyaps applications.
It supports building applications in isolated containers and includes a complete push and publish workflow.`,
	Run: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			fmt.Println("linyaps build tool version 1.0.0")
			return
		}
		cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&versionFlag, "version", false, "Show version")
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

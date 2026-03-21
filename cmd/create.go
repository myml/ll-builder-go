package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
)

var createCmd = &cobra.Command{
	Use:   "create NAME",
	Short: "Create Linyaps build template project",
	Long:  "Create a new Linyaps build template project with a linglong.yaml configuration file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		b := builder.NewBuilder(nil, cwd, nil, nil)
		return b.Create(projectName)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List built Linyaps app",
	Long:  "List all built Linyaps applications in the local repository.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewLocalOSTreeRepo("", cfg)

		// List applications
		return builder.ListApp(ostreeRepo)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

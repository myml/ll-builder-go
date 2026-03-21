package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
)

var (
	noCleanObjects bool
)

var removeCmd = &cobra.Command{
	Use:   "remove [APP...]",
	Short: "Remove built Linyaps app",
	Long:  "Remove built Linyaps applications from the local repository.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewLocalOSTreeRepo("", cfg)

		// Remove applications
		return builder.RemoveApp(ostreeRepo, args, !noCleanObjects)
	},
}

func init() {
	removeCmd.Flags().BoolVar(&noCleanObjects, "no-clean-objects", false, "Don't clean object files before removing")

	rootCmd.AddCommand(removeCmd)
}
